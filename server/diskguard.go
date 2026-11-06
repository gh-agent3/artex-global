package server

// Diskguard: disk-overflow protection with automatic rotation. Past incident:
// run artifacts and logs grew to tens of GB before anyone noticed. Two layers:
//
//   - compose-level: docker json-file logging is capped in docker-compose.yml
//     (the classic multi-GB /var/lib/docker/containers/*-json.log case);
//   - this janitor: an age-based rotation pass over the data directory that
//     ALWAYS runs (not only in an emergency), plus an emergency pass when the
//     filesystem crosses the high-water mark.
//
// Rotation targets (conservative, auditable):
//   - traffic recording blobs older than trafficKeepDays;
//   - files under data/tasks/<terminal task>/ older than taskArtifactsKeepDays
//     (audited into task_provenance_cleanups, actor "diskguard").
// Emergency pass: same walk with a 24h minimum age, regardless of policy, and
// a warning with the largest files when still above the high water afterwards.

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

const (
	diskguardInterval         = 10 * time.Minute
	diskguardHighWaterDefault = 85 // percent used
	diskguardTrafficKeepDays  = 7
	diskguardTaskKeepDays     = 14
	diskguardEmergencyMinAge  = 24 * time.Hour
	diskguardMaxLargestReport = 5
)

// diskguardStats is the last-run summary exposed via the status API.
type diskguardStats struct {
	LastRun        time.Time `json:"last_run"`
	LastFreedBytes int64     `json:"last_freed_bytes"`
	LastFreedFiles int64     `json:"last_freed_files"`
	LastEmergency  bool      `json:"last_emergency"`
	LastNote       string    `json:"last_note"`
}

// diskguardLoop runs the rotation pass periodically.
func (s *Server) diskguardLoop(ctx context.Context) {
	tick := time.NewTicker(diskguardInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.diskguardRun()
		}
	}
}

// diskUsagePct returns used-percent of the filesystem holding path.
func diskUsagePct(path string) (int, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	if st.Blocks == 0 {
		return 0, nil
	}
	used := st.Blocks - st.Bfree
	return int(used * 100 / st.Blocks), nil
}

// rotateTree deletes files under root whose mtime is older than cutoff.
// Directories are removed when they become empty. Returns reclaimed bytes/files.
// Errors on individual files are skipped (best-effort by design).
func rotateTree(root string, cutoff time.Time) (int64, int64, error) {
	var freedBytes, freedFiles int64
	if _, err := os.Stat(root); err != nil {
		return 0, 0, nil // nothing to rotate
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if os.Remove(path) == nil {
				freedBytes += info.Size()
				freedFiles++
			}
		}
		return nil
	})
	// Prune now-empty directories, deepest first.
	removeEmptyDirs(root)
	return freedBytes, freedFiles, err
}

func removeEmptyDirs(root string) {
	dirs := []string{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		_ = os.Remove(d) // non-empty dirs fail silently — fine
	}
}

// largestFiles reports the biggest files under root (for the emergency note).
func largestFiles(root string, n int) []string {
	type entry struct {
		path string
		size int64
	}
	var all []entry
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			all = append(all, entry{path, info.Size()})
		}
		return nil
	})
	sort.Slice(all, func(i, j int) bool { return all[i].size > all[j].size })
	out := []string{}
	for i, e := range all {
		if i >= n {
			break
		}
		out = append(out, fmt.Sprintf("%s (%dMB)", e.path, e.size>>20))
	}
	return out
}

func (s *Server) diskguardRun() {
	if s.m == nil || s.m.pg == nil {
		return
	}
	if !s.m.pg.GetBool("diskguard_enabled", true) {
		return
	}
	dataDir := s.m.dir
	pct, err := diskUsagePct(dataDir)
	if err != nil {
		log.Printf("[diskguard] statfs: %v", err)
		return
	}
	emergency := pct >= diskguardHighWaterDefault

	trafficCutoff := time.Now().AddDate(0, 0, -diskguardTrafficKeepDays)
	taskCutoff := time.Now().AddDate(0, 0, -diskguardTaskKeepDays)
	if emergency {
		// Below the policy ages regardless; floor at 24h so an in-flight run
		// never loses files younger than a day.
		floor := time.Now().Add(-diskguardEmergencyMinAge)
		if trafficCutoff.Before(floor) {
			trafficCutoff = floor
		}
		if taskCutoff.Before(floor) {
			taskCutoff = floor
		}
	}

	var freedBytes, freedFiles int64

	// 1. traffic recording blobs
	b, f, _ := rotateTree(dataDir+"/traffic/_blobs", trafficCutoff)
	freedBytes += b
	freedFiles += f

	// 2. terminal-task artifacts (audited)
	ids, err := s.m.pg.TerminalTasksOlderThan(diskguardTaskKeepDays, 50)
	if err != nil {
		log.Printf("[diskguard] terminal tasks: %v", err)
	}
	for _, id := range ids {
		root := fmt.Sprintf("%s/tasks/%d", dataDir, id)
		b, f, err := rotateTree(root, taskCutoff)
		if err != nil || (b == 0 && f == 0) {
			continue
		}
		freedBytes += b
		freedFiles += f
		if err := s.m.pg.SaveProvenanceCleanup(id, b, f, "diskguard",
			fmt.Sprintf("auto-rotation: usage %d%% keep-days %d (emergency=%v)", pct, diskguardTaskKeepDays, emergency)); err != nil {
			log.Printf("[diskguard] audit task %d: %v", id, err)
		}
	}

	note := fmt.Sprintf("usage=%d%% emergency=%v", pct, emergency)
	if emergency && freedBytes == 0 {
		note += "; largest: " + fmt.Sprintf("%v", largestFiles(dataDir, diskguardMaxLargestReport))
		log.Printf("[diskguard] HIGH WATER %d%% and rotation reclaimed nothing — %s", pct, note)
	} else if freedBytes > 0 {
		log.Printf("[diskguard] rotated %d files, reclaimed %.1fMB (%s)", freedFiles, float64(freedBytes)/(1<<20), note)
	}
	s.diskguardMu.Lock()
	s.diskguard = diskguardStats{LastRun: time.Now(), LastFreedBytes: freedBytes, LastFreedFiles: freedFiles, LastEmergency: emergency, LastNote: note}
	s.diskguardMu.Unlock()
}

func (s *Server) diskguardStatus(w http.ResponseWriter, r *http.Request) {
	pct, err := diskUsagePct(s.m.dir)
	if err != nil {
		pct = -1
	}
	s.diskguardMu.Lock()
	st := s.diskguard
	s.diskguardMu.Unlock()
	writeJSON(w, 200, map[string]any{
		"enabled":           s.m.pg.GetBool("diskguard_enabled", true),
		"usage_pct":         pct,
		"high_water_pct":    diskguardHighWaterDefault,
		"traffic_keep_days": diskguardTrafficKeepDays,
		"task_keep_days":    diskguardTaskKeepDays,
		"last_run":          st.LastRun,
		"last_freed_bytes":  st.LastFreedBytes,
		"last_freed_files":  st.LastFreedFiles,
		"last_emergency":    st.LastEmergency,
		"last_note":         st.LastNote,
	})
}
