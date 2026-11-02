package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/db"
)

// provenanceIntent is one top-level entry of a task's artifact directory.
type provenanceIntent struct {
	Name     string `json:"name"`
	Bytes    int64  `json:"bytes"`
	Files    int64  `json:"files"`
	MTime    string `json:"mtime,omitempty"`
	IntentID int64  `json:"intent_id,omitempty"`
	State    string `json:"state,omitempty"`
	Summary  string `json:"summary,omitempty"`
}

// provenanceView is what the Provenance tab renders: how much space the task's
// working directory takes, which intents own it, and the cleanup audit trail.
type provenanceView struct {
	Path        string                 `json:"path"`
	Exists      bool                   `json:"exists"`
	Bytes       int64                  `json:"bytes"`
	Files       int64                  `json:"files"`
	Intents     []provenanceIntent     `json:"intents"`
	OtherBytes  int64                  `json:"other_bytes"`
	OtherFiles  int64                  `json:"other_files"`
	Cleanups    []db.ProvenanceCleanup `json:"cleanups"`
	Cleanable   int64                  `json:"cleanable_bytes"`
	CleanableNo int64                  `json:"cleanable_files"`
}

// intentDirName reports whether a top-level entry is a per-intent work dir (i1234).
// Those are worker scratch: payloads, replica trees, logs. Everything else in the
// task directory (report.md, uploads, notes) is left alone by a cleanup.
func intentDirName(name string) bool {
	if len(name) < 2 || name[0] != 'i' {
		return false
	}
	rest := name[1:]
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	// Worker dirs are i<id> optionally followed by a short tag: i1234, i1234b, i1483live.
	for ; i < len(rest); i++ {
		c := rest[i]
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// intentIDFromDir returns the graph node id encoded in an intent dir name.
func intentIDFromDir(name string) int64 {
	id, _ := strconv.ParseInt(strings.TrimRight(name[1:], "abcdefghijklmnopqrstuvwxyz"), 10, 64)
	return id
}

func dirUsage(path string) (bytes, files int64, newest time.Time) {
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // a vanished file must not abort the measurement
		}
		if d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			bytes += info.Size()
			files++
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		return nil
	})
	return bytes, files, newest
}

// taskProvenance reports the disk footprint of one task's working directory.
func (s *Server) taskProvenance(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	dir := filepath.Join(s.m.dir, "tasks", t.ID)
	view := provenanceView{Path: dir, Intents: []provenanceIntent{}, Cleanups: []db.ProvenanceCleanup{}}
	entries, err := os.ReadDir(dir)
	if err == nil {
		view.Exists = true
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		b, f, newest := dirUsage(filepath.Join(dir, name))
		if intentDirName(name) {
			view.Intents = append(view.Intents, provenanceIntent{
				Name: name, Bytes: b, Files: f,
				MTime:    newest.UTC().Format(time.RFC3339),
				IntentID: intentIDFromDir(name),
			})
			view.Cleanable += b
			view.CleanableNo += f
		} else {
			view.OtherBytes += b
			view.OtherFiles += f
		}
	}
	sort.Slice(view.Intents, func(i, j int) bool { return view.Intents[i].Bytes > view.Intents[j].Bytes })
	// Attach what the graph knows about each intent so a file can be traced back to
	// the work that produced it (state + the intent summary the planner wrote).
	if pg := s.pg(w); pg != nil && len(view.Intents) > 0 {
		ids := make([]int64, 0, len(view.Intents))
		for _, it := range view.Intents {
			if it.IntentID > 0 {
				ids = append(ids, it.IntentID)
			}
		}
		if len(ids) > 0 {
			rows, err := pg.Query(`
SELECT id, COALESCE(state,''), COALESCE(payload->>'summary','')
FROM exploration_nodes WHERE exploration_id=$1 AND kind='intent' AND id = ANY($2)`,
				t.ExpID, ids)
			if err == nil {
				byID := make(map[int64][2]string, len(ids))
				for rows.Next() {
					var id int64
					var state, summary string
					if rows.Scan(&id, &state, &summary) == nil {
						byID[id] = [2]string{state, summary}
					}
				}
				rows.Close()
				for i := range view.Intents {
					if v, ok := byID[view.Intents[i].IntentID]; ok {
						view.Intents[i].State = v[0]
						view.Intents[i].Summary = v[1]
					}
				}
			}
		}
	}
	// The total also covers top-level files (report.md and the like).
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, ierr := e.Info(); ierr == nil {
			view.OtherBytes += info.Size()
			view.OtherFiles++
		}
	}
	view.Bytes = view.Cleanable + view.OtherBytes
	view.Files = view.CleanableNo + view.OtherFiles
	if pg := s.pg(w); pg != nil {
		taskID, _ := strconv.ParseInt(t.ID, 10, 64)
		if list, err := pg.ListProvenanceCleanups(taskID); err == nil && list != nil {
			view.Cleanups = list
		}
	}
	writeJSON(w, http.StatusOK, view)
}

// taskProvenanceCleanup deletes the per-intent work dirs of one task and records the
// cleanup. The task's own files (report.md, uploads) are never touched, and the audit
// row keeps "removed on purpose, N bytes" visible afterwards.
func (s *Server) taskProvenanceCleanup(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	var req struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	dir := filepath.Join(s.m.dir, "tasks", t.ID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeErr(w, http.StatusOK, "nothing to clean")
		return
	}
	var freedBytes, freedFiles int64
	var removed []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || !intentDirName(name) {
			continue
		}
		full := filepath.Join(dir, name)
		b, f, _ := dirUsage(full)
		if err := os.RemoveAll(full); err != nil {
			continue
		}
		freedBytes += b
		freedFiles += f
		removed = append(removed, name)
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	if pg := s.pg(w); pg != nil {
		if err := pg.SaveProvenanceCleanup(taskID, freedBytes, freedFiles, "operator", req.Note); err != nil {
			log.Printf("[provenance] task %s cleanup audit failed: %v", t.ID, err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"removed":     len(removed),
		"freed_bytes": freedBytes,
		"freed_files": freedFiles,
	})
}

// provenanceFile is one file inside a task artifact directory.
type provenanceFile struct {
	Path  string `json:"path"` // relative to the task dir
	Bytes int64  `json:"bytes"`
	MTime string `json:"mtime"`
}

// safeTaskPath resolves a client-supplied relative path inside the task directory and
// refuses anything that escapes it.
func (s *Server) safeTaskPath(taskID, rel string) (string, bool) {
	root := filepath.Join(s.m.dir, "tasks", taskID)
	full := filepath.Join(root, filepath.Clean("/"+rel))
	rootAbs, err1 := filepath.Abs(root)
	fullAbs, err2 := filepath.Abs(full)
	if err1 != nil || err2 != nil {
		return "", false
	}
	if fullAbs != rootAbs && !strings.HasPrefix(fullAbs, rootAbs+string(os.PathSeparator)) {
		return "", false
	}
	return fullAbs, true
}

// taskProvenanceFiles lists the files of one artifact directory (newest/largest first)
// so the UI can show a real file browser instead of only directory totals.
func (s *Server) taskProvenanceFiles(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("dir"))
	if rel == "" {
		writeErr(w, http.StatusBadRequest, "dir is required")
		return
	}
	dir, ok := s.safeTaskPath(t.ID, rel)
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	root := filepath.Join(s.m.dir, "tasks", t.ID)
	out := make([]provenanceFile, 0, 128)
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if len(out) >= 500 {
			return fs.SkipAll
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		relPath, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		out = append(out, provenanceFile{
			Path:  filepath.ToSlash(relPath),
			Bytes: info.Size(),
			MTime: info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	writeJSON(w, http.StatusOK, map[string]any{"dir": rel, "files": out})
}

// taskProvenanceFile streams one artifact file as a download.
func (s *Server) taskProvenanceFile(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	full, ok := s.safeTaskPath(t.ID, r.URL.Query().Get("path"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		writeErr(w, http.StatusNotFound, "file not found")
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(full)+"\"")
	http.ServeFile(w, r, full)
}
