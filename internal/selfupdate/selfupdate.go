// Package selfupdate implements the one-click update from the ARTEX web UI: it
// pulls the new binary from a GitHub Release, verifies it, stages it, and swaps
// it in atomically on the next start.
//
// Division of labour (see start.sh / start.bat):
//
//	start script = a dumb supervisor loop; it only decides whether to respawn
//	               the process after it exits, based on the exit code
//	this package = all the error-prone logic (download / SHA256 verify / smoke
//	               test / swap / rollback on failure)
//
// The swap lives in Go rather than in the scripts because SHA256 verification and
// the smoke test would have to be written twice for sh and bat (sha256sum /
// shasum / certutil), and that is exactly the step that must not go wrong:
// install a binary that cannot run and the supervisor will faithfully respawn it
// forever, leaving the user to rescue the machine by hand.
//
// A full upgrade spans three process starts:
//
//  1. The old server gets /api/update/apply -> download + verify -> stage
//     artex.new -> exit 75
//  2. The script respawns the old version -> Bootstrap finds artex.new ->
//     verify + smoke test -> swap -> exit 75
//  3. The script respawns, now running the new version -> Bootstrap records one
//     attempt -> the marker is cleared once startup succeeds
//
// Any failing step falls back to the old version: if step 2 fails verification the
// staged file is deleted and the old version keeps running; if step 3 never
// survives long enough to clear the marker three times in a row, artex.old is
// swapped back automatically.
package selfupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ExitRestart is the "respawn me, supervisor" exit code (EX_TEMPFAIL). The start
// script reruns immediately on it and does not count it as a crash backoff. 0 means
// the user stopped the process normally (the script leaves the loop); anything
// else counts as a crash.
const ExitRestart = 75

// maxAttempts is how many starts a swapped-in version gets. Every start increments
// the counter; surviving settleDelay clears the marker. Crashing maxAttempts times
// in a row means the new version cannot start at all, so it is rolled back.
const maxAttempts = 3

// Paths holds every file involved in one upgrade, all anchored to the directory
// of the executable.
// CWD is deliberately not used: when running as a service the working directory may
// be / or anything else, which would drop the staged file elsewhere and break the
// swap logic entirely.
type Paths struct {
	Dir     string // directory of the executable
	Current string // currently running binary   artex      / artex.exe
	New     string // staged new version          artex.new  / artex.new.exe
	Sum     string // sha256 of the new version (hex) artex.new.sha256 / artex.new.exe.sha256
	Old     string // previous version backed up before the swap artex.old / artex.old.exe
	Marker  string // upgrade state marker        artex.upgrade.json
}

// ResolvePaths derives all upgrade paths from the current executable.
//
// On Windows .new/.old must keep the .exe suffix, otherwise both the smoke test and
// the post-swap execution fail, so the suffix is stripped first and re-appended;
// that keeps the naming symmetric across both platforms.
func ResolvePaths() (Paths, error) {
	exe, err := os.Executable()
	if err != nil {
		return Paths{}, fmt.Errorf("실행 파일 위치 확인 실패: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	name := filepath.Base(exe)
	ext := filepath.Ext(name) // ".exe" on Windows, usually empty on Unix
	stem := strings.TrimSuffix(name, ext)

	join := func(suffix string) string { return filepath.Join(dir, stem+suffix+ext) }
	return Paths{
		Dir:     dir,
		Current: exe,
		New:     join(".new"),
		Sum:     join(".new") + ".sha256",
		Old:     join(".old"),
		Marker:  filepath.Join(dir, stem+".upgrade.json"),
	}, nil
}

// marker records the progress of one swap so a new version that cannot start
// triggers an automatic rollback.
type marker struct {
	From     string `json:"from"`     // version before the upgrade
	To       string `json:"to"`       // target version
	Attempts int    `json:"attempts"` // starts attempted after the swap
	StagedAt int64  `json:"staged_at"`
}

func readMarker(path string) (marker, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return marker{}, false
	}
	var m marker
	if json.Unmarshal(b, &m) != nil {
		return marker{}, false
	}
	return m, true
}

func writeMarker(path string, m marker) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// cleanStaged removes the staged files. It runs after a successful swap, a failed
// verification and a user cancel alike, so a leftover artex.new is never retried on
// the next start.
func cleanStaged(p Paths) {
	_ = os.Remove(p.New)
	_ = os.Remove(p.Sum)
}

// CompareVersions compares two version strings and returns -1/0/1 (a<b / a==b /
// a>b). ok=false means at least one side is not a comparable version string (for
// example a local dev build's "dev" or git describe's
// "0.3.7-2-gabc1234-dirty"); callers should then disable one-click update, otherwise
// a build under development would be "upgraded" to a release and lose uncommitted
// changes.
func CompareVersions(a, b string) (int, bool) {
	av, aok := parseVersion(a)
	bv, bok := parseVersion(b)
	if !aok || !bok {
		return 0, false
	}
	for i := range 3 {
		if av[i] != bv[i] {
			if av[i] < bv[i] {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

// parseVersion parses "v0.3.7" / "0.3.7" into a [3]int.
//
// Only a clean three-part version is accepted: for non-tag builds build.sh uses git
// describe, which yields suffixed versions such as "0.3.7-2-gabc1234". Those must be
// treated as non-comparable instead of being read as 0.3.7, otherwise a dev build is
// wrongly reported as up to date or overwritten by a release.
func parseVersion(s string) ([3]int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return [3]int{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// InDocker reports whether the process runs in a container. Under Docker the swap
// writes to the container's writable layer, so recreating the container with
// `docker compose up -d` reverts to the version baked into the image. That is
// expected (the user is pulling a new image at that point), but the frontend needs
// to be able to say so clearly.
func InDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	b, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "docker") || strings.Contains(s, "containerd")
}
