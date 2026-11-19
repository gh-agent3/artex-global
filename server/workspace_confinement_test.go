package server

// os.Root confinement tests (ported with wpsec/ARTEX-Pro fix(security)
// 844dfffd): a symlink planted inside the workspace must not let workspace
// read/download reach files outside the root, even though plain
// filepath.Clean-based resolution accepts the on-disk path.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceSymlinkConfined(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("outside-root"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Plant a symlink inside the workspace pointing outside the root — agents
	// running probes write to this shared dir, so this is a realistic state.
	if err := os.Symlink(secret, filepath.Join(dir, "escape-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := &Server{m: &Manager{dir: dir}}

	// read/download follow symlinks on the plain path — os.Root must refuse them.
	for _, c := range []struct {
		name string
		path string
	}{
		{"read", "/api/workspace/read?path=escape-link"},
		{"download", "/api/workspace/download?path=escape-link"},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			var h http.HandlerFunc
			if c.name == "read" {
				h = s.wsRead
			} else {
				h = s.wsDownload
			}
			h(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
			if rec.Code != 404 && rec.Code != 500 {
				t.Fatalf("status = %d (body %q), want 404/500 — the symlink escape was served", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "outside-root") {
				t.Fatalf("response leaked the outside file content")
			}
		})
	}

	// delete removes the symlink entry itself (never the target) and must not
	// touch the outside file.
	rec := httptest.NewRecorder()
	s.wsDelete(rec, httptest.NewRequest(http.MethodDelete, "/api/workspace/delete?path=escape-link", nil))
	if rec.Code != 200 {
		t.Fatalf("delete status = %d (body %q)", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("outside file was affected through the workspace symlink: %v", err)
	}

	// A fresh symlink for the list check: listing shows the entry without
	// following it.
	if err := os.Symlink(secret, filepath.Join(dir, "escape-link2")); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	s.wsList(rec, httptest.NewRequest(http.MethodGet, "/api/workspace/list?path=", nil))
	if rec.Code != 200 {
		t.Fatalf("list status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "escape-link2") {
		t.Fatalf("list does not show the symlink entry: %s", rec.Body.String())
	}
}
