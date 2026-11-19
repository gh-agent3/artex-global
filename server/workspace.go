package server

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Workspace file manager — browse / view / edit / download / upload / delete the
// shared work dir (s.m.dir), where all agents write their artifacts. Every path is
// confined to the work dir root: traversal via ".." is neutralised by wsResolve,
// and all real filesystem work goes through an os.Root (Go 1.24+), so a symlink
// planted inside the work dir (agents run untrusted probes there) cannot escape
// the root either (os.Root hardening ported from wpsec/ARTEX-Pro fix(security)
// 844dfffd). All routes sit behind requireAuth (see Handler()).

const (
	maxWorkspaceRead   = 2 << 20   // 2 MiB: files bigger than this aren't inlined for view/edit (download instead)
	maxWorkspaceUpload = 512 << 20 // 512 MiB per upload request
)

// 사용자에게 노출되는 오류 문구(한국어). 경로·페이로드·err.Error() 원문은 그대로 둔다.
// workspace → "작업 공간"(용어집 정본).
const (
	errWsIllegalPath      = "잘못된 경로입니다"
	errWsPathNotFound     = "경로가 존재하지 않습니다"
	errWsNotDir           = "디렉터리가 아닙니다"
	errWsFileNotFound     = "파일이 존재하지 않습니다"
	errWsIsDir            = "디렉터리이므로 파일로 읽을 수 없습니다"
	errWsTargetIsDir      = "대상 경로가 디렉터리입니다"
	errWsCannotDeleteRoot = "작업 공간 루트 디렉터리는 삭제할 수 없습니다"
	errWsUploadDirMissing = "대상 디렉터리가 존재하지 않습니다"
	errWsUploadParse      = "업로드 처리에 실패했거나 크기 제한을 초과했습니다: " // 뒤에 err.Error() 를 이어 붙인다
	errWsNoUploadFile     = "업로드할 파일이 없습니다(폼 필드 file)"
	errWsRootUnavailable  = "작업 공간에 접근할 수 없습니다"
)

// wsResolve maps a user-supplied relative path to a cleaned path relative to
// the work dir ("" or "." for the root itself). It returns ok=false if the path
// would escape the root. filepath.Clean on a rooted copy collapses any "..", and
// the os.Root ops enforce the same boundary when following symlinks.
func (s *Server) wsResolve(rel string) (string, bool) {
	base := filepath.Clean(s.m.dir)
	rel = strings.TrimPrefix(strings.TrimSpace(rel), "/")
	clean := filepath.Clean("/" + rel) // e.g. "/a/../../etc" → "/etc" (still rooted at "/")
	abs := filepath.Clean(filepath.Join(base, clean))
	if abs != base && !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		return "", false
	}
	within, err := filepath.Rel(base, abs)
	if err != nil {
		return "", false
	}
	return within, true
}

// wsRel renders a rooted filesystem path as a workspace-relative path.
func (s *Server) wsRel(rel string) string {
	if rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

func openWorkspaceRoot(w http.ResponseWriter, dir string) (*os.Root, bool) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		writeErr(w, 500, errWsRootUnavailable)
		return nil, false
	}
	return root, true
}

func closeWorkspaceRoot(root *os.Root) {
	if err := root.Close(); err != nil {
		log.Printf(logT("[workspace] 작업 공간 루트를 닫지 못했습니다: %v"), err)
	}
}

func closeWorkspaceFile(file *os.File) {
	if err := file.Close(); err != nil {
		log.Printf(logT("[workspace] 작업 공간 파일을 닫지 못했습니다: %v"), err)
	}
}

type wsEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"` // unix millis
}

// GET /api/workspace/list?path=<rel>
func (s *Server) wsList(w http.ResponseWriter, r *http.Request) {
	rel, ok := s.wsResolve(r.URL.Query().Get("path"))
	if !ok {
		writeErr(w, 400, errWsIllegalPath)
		return
	}
	root, ok := openWorkspaceRoot(w, s.m.dir)
	if !ok {
		return
	}
	defer closeWorkspaceRoot(root)
	fi, err := root.Stat(rel)
	if err != nil {
		writeErr(w, 404, errWsPathNotFound)
		return
	}
	if !fi.IsDir() {
		writeErr(w, 400, errWsNotDir)
		return
	}
	d, err := root.Open(rel)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer closeWorkspaceFile(d)
	ents, err := d.ReadDir(-1)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]wsEntry, 0, len(ents))
	for _, e := range ents {
		entryPath := filepath.Join(rel, e.Name())
		info, err := root.Lstat(entryPath) // Lstat: symlinks are listed as entries, never followed
		if err != nil {
			continue
		}
		out = append(out, wsEntry{
			Name:  e.Name(),
			Path:  s.wsRel(entryPath),
			Dir:   info.IsDir(),
			Size:  info.Size(),
			MTime: info.ModTime().UnixMilli(),
		})
	}
	// 디렉터리를 먼저 두고, 각각 이름순으로 정렬한다.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, 200, map[string]any{"path": s.wsRel(rel), "entries": out})
}

// GET /api/workspace/read?path=<rel> — inline text for view/edit. Binary or oversize
// files return {binary:true}/{too_large:true} with no content (use download instead).
func (s *Server) wsRead(w http.ResponseWriter, r *http.Request) {
	rel, ok := s.wsResolve(r.URL.Query().Get("path"))
	if !ok {
		writeErr(w, 400, errWsIllegalPath)
		return
	}
	root, ok := openWorkspaceRoot(w, s.m.dir)
	if !ok {
		return
	}
	defer closeWorkspaceRoot(root)
	fi, err := root.Stat(rel)
	if err != nil {
		writeErr(w, 404, errWsFileNotFound)
		return
	}
	if fi.IsDir() {
		writeErr(w, 400, errWsIsDir)
		return
	}
	if fi.Size() > maxWorkspaceRead {
		writeJSON(w, 200, map[string]any{"path": s.wsRel(rel), "size": fi.Size(), "too_large": true, "binary": true})
		return
	}
	data, err := root.ReadFile(rel)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		writeJSON(w, 200, map[string]any{"path": s.wsRel(rel), "size": fi.Size(), "binary": true})
		return
	}
	writeJSON(w, 200, map[string]any{"path": s.wsRel(rel), "size": fi.Size(), "binary": false, "content": string(data)})
}

// POST /api/workspace/write  {path, content} — create/overwrite a text file.
func (s *Server) wsWrite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rel, ok := s.wsResolve(req.Path)
	if !ok || rel == "." {
		writeErr(w, 400, errWsIllegalPath)
		return
	}
	root, ok := openWorkspaceRoot(w, s.m.dir)
	if !ok {
		return
	}
	defer closeWorkspaceRoot(root)
	if fi, err := root.Stat(rel); err == nil && fi.IsDir() {
		writeErr(w, 400, errWsTargetIsDir)
		return
	}
	if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := root.WriteFile(rel, []byte(req.Content), 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "path": s.wsRel(rel)})
}

// POST /api/workspace/mkdir  {path}
func (s *Server) wsMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	rel, ok := s.wsResolve(req.Path)
	if !ok || rel == "." {
		writeErr(w, 400, errWsIllegalPath)
		return
	}
	root, ok := openWorkspaceRoot(w, s.m.dir)
	if !ok {
		return
	}
	defer closeWorkspaceRoot(root)
	if err := root.MkdirAll(rel, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "path": s.wsRel(rel)})
}

// DELETE /api/workspace/delete?path=<rel> — removes a file or a directory tree
// (confined to the work dir; the root itself can't be deleted).
func (s *Server) wsDelete(w http.ResponseWriter, r *http.Request) {
	rel, ok := s.wsResolve(r.URL.Query().Get("path"))
	if !ok {
		writeErr(w, 400, errWsIllegalPath)
		return
	}
	if rel == "." {
		writeErr(w, 400, errWsCannotDeleteRoot)
		return
	}
	root, ok := openWorkspaceRoot(w, s.m.dir)
	if !ok {
		return
	}
	defer closeWorkspaceRoot(root)
	if _, err := root.Lstat(rel); err != nil {
		writeErr(w, 404, errWsPathNotFound)
		return
	}
	if err := root.RemoveAll(rel); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/workspace/download?path=<rel> — stream a file as an attachment.
func (s *Server) wsDownload(w http.ResponseWriter, r *http.Request) {
	rel, ok := s.wsResolve(r.URL.Query().Get("path"))
	if !ok {
		writeErr(w, 400, errWsIllegalPath)
		return
	}
	root, ok := openWorkspaceRoot(w, s.m.dir)
	if !ok {
		return
	}
	defer closeWorkspaceRoot(root)
	f, err := root.Open(rel)
	if err != nil {
		writeErr(w, 404, errWsFileNotFound)
		return
	}
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		closeWorkspaceFile(f)
		writeErr(w, 404, errWsFileNotFound)
		return
	}
	defer closeWorkspaceFile(f)
	name := filepath.Base(rel)
	// RFC 5987 filename* keeps non-ASCII names intact; plain filename is the fallback.
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(name)+"\"; filename*=UTF-8''"+url.PathEscape(name))
	http.ServeContent(w, r, name, fi.ModTime(), f)
}

// POST /api/workspace/upload?path=<dir> — multipart form field "file" (one or more).
func (s *Server) wsUpload(w http.ResponseWriter, r *http.Request) {
	dirRel, ok := s.wsResolve(r.URL.Query().Get("path"))
	if !ok {
		writeErr(w, 400, errWsIllegalPath)
		return
	}
	root, ok := openWorkspaceRoot(w, s.m.dir)
	if !ok {
		return
	}
	defer closeWorkspaceRoot(root)
	if fi, err := root.Stat(dirRel); err != nil || !fi.IsDir() {
		writeErr(w, 400, errWsUploadDirMissing)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkspaceUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, 400, errWsUploadParse+err.Error())
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeErr(w, 400, errWsNoUploadFile)
		return
	}
	saved := 0
	for _, hdr := range files {
		name := filepath.Base(hdr.Filename) // strip any path component
		if name == "" || name == "." || name == ".." {
			continue
		}
		if err := saveUpload(root, hdr, filepath.Join(dirRel, name)); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		saved++
	}
	writeJSON(w, 200, map[string]any{"uploaded": saved})
}

// saveUpload streams one multipart file into the workspace root; os.Root rejects
// any destination that would escape the root through a symlink.
func saveUpload(root *os.Root, hdr *multipart.FileHeader, rel string) error {
	src, err := hdr.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := root.OpenFile(rel, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer closeWorkspaceFile(out)
	_, err = io.Copy(out, src)
	return err
}

// sanitizeFilename strips characters unsafe for a Content-Disposition filename token.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\\", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	return name
}
