package server

import "testing"

func TestVerifyPolicySeverityGate(t *testing.T) {
	// critical/high always in scope; medium is a deterministic 50% sample;
	// low/info never.
	for _, sev := range []string{"critical", "high"} {
		if !verifyPolicyApplies(sev, 1) || !verifyPolicyApplies(sev, 2) {
			t.Fatalf("%s must always verify", sev)
		}
	}
	if verifyPolicyApplies("low", 1) || verifyPolicyApplies("info", 99) || verifyPolicyApplies("", 7) {
		t.Fatal("low/info/empty must never verify")
	}
	// medium: deterministic and ~50% over a range of ids.
	n := 0
	for id := int64(1); id <= 200; id++ {
		a, b := verifyPolicyApplies("medium", id), verifyPolicyApplies("medium", id)
		if a != b {
			t.Fatalf("medium decision not deterministic for id %d", id)
		}
		if a {
			n++
		}
	}
	if n < 60 || n > 140 {
		t.Fatalf("medium sample of 200 ids = %d, want ~100", n)
	}
}
