package selfupdate

// Regression tests for the compiled-out online self-update (same call as
// failsafe-security/artex): every online entry point must refuse before
// touching the network, so the running binary can never replace itself
// with a remotely fetched artifact.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestOnlineSelfUpdateCompiledOut(t *testing.T) {
	if Enabled {
		t.Fatal("Enabled must stay false in this build: online self-update is a remote-binary-swap RCE channel")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// FetchLatest must refuse without any network request (a nil-ish client
	// with a short timeout proves no dial happens: it returns instantly).
	if _, err := FetchLatest(ctx, &http.Client{Timeout: time.Second}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("FetchLatest = %v, want ErrDisabled", err)
	}

	rel := &Release{TagName: "v0.0.1"}
	if err := Stage(ctx, &http.Client{Timeout: time.Second}, rel, "dev", nil); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Stage = %v, want ErrDisabled", err)
	}
}
