package db

// Verification queue tests: enqueue creates a bound conversation with a frozen
// snapshot, double-enqueue is refused, the verdict mirrors onto findings.validation.

import (
	"context"
	"strings"
	"testing"
)

func TestFindingVerificationLifecycle(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("database unavailable (%v) — skipping", err)
	}
	defer d.Close()
	ctx := context.Background()

	fid, err := d.AddFinding(0, 0, "auth", "verify-fixture", "high", "claim", "evidence", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM findings WHERE id=$1`, fid)

	v, conv, created, err := d.CreateFindingVerification(ctx, fid, "test")
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	if v.Status != "pending" || v.ConversationID == nil {
		t.Fatalf("row: %+v", v)
	}
	if conv == nil || conv.AgentKey != FindingVerifierAgentKey {
		t.Fatalf("conversation: %+v", conv)
	}
	snap, err := d.VerificationSnapshot(v.ID)
	if err != nil || !strings.Contains(string(snap), "verify-fixture") {
		t.Fatalf("snapshot: %s err=%v", snap, err)
	}

	// Second enqueue while one exists is a no-op.
	_, _, created, err = d.CreateFindingVerification(ctx, fid, "again")
	if err != nil || created {
		t.Fatalf("double enqueue: created=%v err=%v", created, err)
	}

	// Bound-conversation lookup works both ways.
	got, err := d.VerificationForConversation(ctx, *v.ConversationID)
	if err != nil || got == nil || got.ID != v.ID {
		t.Fatalf("by conversation: %+v err=%v", got, err)
	}

	hist, err := d.ListFindingVerifications(fid)
	if err != nil || len(hist) != 1 {
		t.Fatalf("history: %d err=%v", len(hist), err)
	}

	if err := d.FinishFindingVerification(v.ID, "verified", "evidence proves the claim"); err != nil {
		t.Fatal(err)
	}
	var validation string
	if err := d.QueryRow(`SELECT validation FROM findings WHERE id=$1`, fid).Scan(&validation); err != nil {
		t.Fatal(err)
	}
	if validation != "verified" {
		t.Fatalf("findings.validation = %q, want verified", validation)
	}
	fin, _ := d.VerificationForConversation(ctx, *v.ConversationID)
	if fin.Status != "verified" || fin.Reason == "" {
		t.Fatalf("finished row: %+v", fin)
	}
}

func TestFindingsPendingVerificationListsOnlyFresh(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("database unavailable (%v) — skipping", err)
	}
	defer d.Close()
	ctx := context.Background()

	fid, err := d.AddFinding(0, 0, "info", "pending-verify-fixture", "low", "s", "e", "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM findings WHERE id=$1`, fid)
	defer d.Exec(`DELETE FROM finding_verifications WHERE finding_id=$1`, fid)

	rows, err := d.FindingsPendingVerification(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.ID == fid {
			found = true
		}
	}
	if !found {
		t.Fatal("fresh finding missing from the pending scan")
	}

	// Once a verification exists (any verdict), it leaves the pending set.
	v, _, _, err := d.CreateFindingVerification(ctx, fid, "x")
	if err != nil {
		t.Fatal(err)
	}
	_ = d.FinishFindingVerification(v.ID, "inconclusive", "not enough evidence")
	rows, _ = d.FindingsPendingVerification(ctx, 100)
	for _, r := range rows {
		if r.ID == fid {
			t.Fatal("finding with a finished verification still pending")
		}
	}
}
