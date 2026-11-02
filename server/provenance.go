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

	"github.com/Autumn-27/artex/db"
)

// provenanceIntent is one top-level entry of a task's artifact directory.
type provenanceIntent struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
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
	for _, r := range name[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func dirUsage(path string) (bytes, files int64) {
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
		}
		return nil
	})
	return bytes, files
}

// taskProvenance reports the disk footprint of one task's working directory.
func (s *Server) taskProvenance(w http.ResponseWriter, r *http.Request) {
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	dir := filepath.Join(s.m.dir, "tasks", t.ID)
	view := provenanceView{Path: dir}
	entries, err := os.ReadDir(dir)
	if err == nil {
		view.Exists = true
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		b, f := dirUsage(filepath.Join(dir, name))
		if intentDirName(name) {
			view.Intents = append(view.Intents, provenanceIntent{Name: name, Bytes: b, Files: f})
			view.Cleanable += b
			view.CleanableNo += f
		} else {
			view.OtherBytes += b
			view.OtherFiles += f
		}
	}
	sort.Slice(view.Intents, func(i, j int) bool { return view.Intents[i].Bytes > view.Intents[j].Bytes })
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
		if list, err := pg.ListProvenanceCleanups(taskID); err == nil {
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
		b, f := dirUsage(full)
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
			log.Printf("[provenance] task %d cleanup audit failed: %v", t.ID, err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"removed":     len(removed),
		"freed_bytes": freedBytes,
		"freed_files": freedFiles,
	})
}
