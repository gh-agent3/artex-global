package db

// Core-path validation for the experimental SQLite backend (backlog item 6).
// Uses a real SQLite file via the modernc driver; exercises the flows a
// single-user local deployment needs: schema apply, settings roundtrip,
// profile save/load, task creation, findings, activity, credential crypto and
// the verification queue.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func openTestSQLite(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artex-test.db")
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"
	d, err := Open(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSQLiteCore(t *testing.T) {
	d := openTestSQLite(t)
	ctx := context.Background()

	// settings roundtrip
	if err := d.SetSetting("traffic_capture", "true"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	v, ok, err := d.GetSetting("traffic_capture")
	if err != nil || !ok || v != "true" {
		t.Fatalf("GetSetting = %q ok=%v err=%v", v, ok, err)
	}
	if err := d.SetBool("llm_pool_enabled", false); err != nil {
		t.Fatalf("SetBool: %v", err)
	}
	if d.GetBool("llm_pool_enabled", true) {
		t.Fatal("GetBool default ignored")
	}

	// profiles: save, list, active
	pid, err := d.SaveProfile(&LLMProfile{Name: "sqlite-core", Format: "openai", Model: "m", APIKey: "sk-sqlite-core"})
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	defer d.Exec(`DELETE FROM llm_profiles WHERE id=$1`, pid)
	profs, err := d.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	found := false
	for _, p := range profs {
		if p.ID == pid && p.APIKeyHint != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("saved profile not listed")
	}
	byID, err := d.ProfileByID(pid)
	if err != nil || byID == nil || byID.APIKey != "sk-sqlite-core" {
		t.Fatalf("ProfileByID: %+v err=%v", byID, err)
	}

	// task creation path (origin fact + goal node + spawns link)
	task, err := d.CreateTask("sqlite core task", "prove sqlite works", nil, 0, 0)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	defer d.DeleteTask(task.ID)
	if task.ExplorationID == 0 {
		t.Fatal("task has no exploration")
	}

	// findings
	fid, err := d.AddFinding(task.ID, 0, "auth", "sqlite-finding", "high", "claim", "evidence", "worker", nil)
	if err != nil {
		t.Fatalf("AddFinding: %v", err)
	}
	f, err := d.GetFinding(fid)
	if err != nil || f == nil || f.Name != "sqlite-finding" {
		t.Fatalf("GetFinding: %+v err=%v", f, err)
	}

	// activity append + read
	actID, err := d.Exploration(task.ExplorationID).AppendActivity(Activity{Worker: "w", Kind: "text", Summary: "sqlite activity"})
	if err != nil || actID == 0 {
		t.Fatalf("AppendActivity: %d err=%v", actID, err)
	}

	// verification queue lifecycle on the sqlite finding
	vf, conv, created, err := d.CreateFindingVerification(ctx, fid, "auto")
	if err != nil || !created {
		t.Fatalf("CreateFindingVerification: created=%v err=%v", created, err)
	}
	_ = vf
	if conv == nil {
		t.Fatal("no conversation bound")
	}
	snap, err := d.VerificationSnapshot(vf.ID)
	if err != nil || !strings.Contains(string(snap), "sqlite-finding") {
		t.Fatalf("snapshot: %s err=%v", snap, err)
	}
	if err := d.FinishFindingVerification(vf.ID, "verified", "evidence fits"); err != nil {
		t.Fatalf("FinishFindingVerification: %v", err)
	}
	var validation string
	if err := d.QueryRow(`SELECT validation FROM findings WHERE id=$1`, fid).Scan(&validation); err != nil || validation != "verified" {
		t.Fatalf("validation = %q err=%v", validation, err)
	}

	// credential crypto on top of sqlite
	SetCredentialKey([]byte("sqlite-test-master-key"))
	defer SetCredentialKey(nil)
	pid2, err := d.SaveProfile(&LLMProfile{Name: "sqlite-enc", Format: "openai", Model: "m", APIKey: "sk-secret-sqlite"})
	if err != nil {
		t.Fatalf("SaveProfile encrypted: %v", err)
	}
	defer d.Exec(`DELETE FROM llm_profiles WHERE id=$1`, pid2)
	var raw string
	if err := d.QueryRow(`SELECT api_key FROM llm_profiles WHERE id=$1`, pid2).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == "sk-secret-sqlite" || !strings.HasPrefix(raw, "enc:v1:") {
		t.Fatalf("api_key not encrypted: %q", raw)
	}
	enc, err := d.ProfileByID(pid2)
	if err != nil || enc.APIKey != "sk-secret-sqlite" {
		t.Fatalf("decrypt roundtrip: %+v err=%v", enc, err)
	}
}
