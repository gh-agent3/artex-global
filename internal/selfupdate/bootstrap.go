package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gh-agent3/artex-global/config"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// smokeEnv makes the child process spawned by the smoke test skip Bootstrap.
//
// Strictly speaking it is not required: the child's os.Executable() is artex.new, so
// every derived path carries the .new prefix and cannot touch the real upgrade files.
// Relying on that coincidence is too fragile though; an explicit short-circuit is
// obvious and also saves the child a pointless disk probe.
const smokeEnv = "ARTEX_SELFUPDATE_SMOKE"

// Action is the instruction Bootstrap gives main.
type Action int

const (
	// Continue: start the server as usual.
	Continue Action = iota
	// Restart: exit immediately with ExitRestart so the supervisor respawns us.
	Restart
)

// State describes the upgrade state of this start, so /api/update/check can tell the
// frontend truthfully whether the last upgrade succeeded or was rolled back.
type State struct {
	Pending     bool   // swapped in but not yet confirmed stable
	RolledBack  bool   // an automatic rollback just happened during this start
	FailedStage bool   // staged file failed verification/smoke test and was discarded
	Detail      string // one-line explanation for the user
}

// Bootstrap runs at the very top of main and must be called before listening on any
// port or opening the database.
//
// Three situations:
//
//  1. a staged artex.new exists -> verify + smoke test; on success swap and
//     request a restart, otherwise discard and keep running the old version
//  2. only the marker file is left -> the swap just happened; count one attempt
//     and roll back once there have been enough failures in a row
//  3. neither exists -> normal start
func Bootstrap() (Action, State) {
	if os.Getenv(smokeEnv) != "" {
		return Continue, State{}
	}
	p, err := ResolvePaths()
	if err != nil {
		log.Printf(config.TA("[update] bootstrap skipped: %v"), err)
		return Continue, State{}
	}

	if _, err := os.Stat(p.New); err == nil {
		return applyStaged(p)
	}

	m, ok := readMarker(p.Marker)
	if !ok {
		return Continue, State{}
	}
	return confirmOrRollback(p, m)
}

// applyStaged handles the "a staged file exists" situation: swap if verification
// passes, discard otherwise.
//
// This is the only place in the whole upgrade chain that overwrites the executable,
// and it is the last gate: the smoke test catches corrupt downloads, wrong
// architecture, missing shared libraries and the like. Let one unrunnable binary
// through and the supervisor will respawn it tirelessly while Go code never gets to
// run, so automatic rollback could not kick in either.
func applyStaged(p Paths) (Action, State) {
	m, _ := readMarker(p.Marker)

	if err := verifyStaged(p); err != nil {
		log.Printf(config.TA("[update] staged new version failed validation, discarded, continuing the current version: %v"), err)
		cleanStaged(p)
		_ = os.Remove(p.Marker)
		return Continue, State{FailedStage: true, Detail: "새 버전 검증에 실패해 폐기했습니다: " + err.Error()}
	}

	if err := swap(p); err != nil {
		log.Printf(config.TA("[update] swap failed, continuing the current version: %v"), err)
		cleanStaged(p)
		_ = os.Remove(p.Marker)
		return Continue, State{FailedStage: true, Detail: "버전 교체에 실패했습니다: " + err.Error()}
	}

	// Swap succeeded. Keep the marker so the next start (which runs the new version)
	// can confirm stability.
	m.Attempts = 0
	if m.StagedAt == 0 {
		m.StagedAt = time.Now().Unix()
	}
	if err := writeMarker(p.Marker, m); err != nil {
		log.Printf(config.TA("[update] failed to write the upgrade marker (automatic rollback is now unavailable): %v"), err)
	}
	log.Printf(config.TA("[update] swapped to %s, exiting to restart (exit %d)"), orUnknown(m.To), ExitRestart)
	return Restart, State{Pending: true}
}

// confirmOrRollback handles "the start after a swap": it counts attempts and restores
// the previous version once the limit is exceeded.
//
// The counter only increments once Go code runs, so it covers failures where the
// binary executes but crashes during initialisation (incompatible config, port in
// use, a failed DB migration). "Cannot exec at all" is caught by the pre-swap smoke
// test; together the two cover the whole range.
func confirmOrRollback(p Paths, m marker) (Action, State) {
	m.Attempts++
	if m.Attempts > maxAttempts {
		if err := rollback(p); err != nil {
			// If even the rollback failed, do not restart again or we end up in an
			// endless loop. Clear the marker and let the process start as-is; if it
			// cannot start, at least the user sees why in the log.
			log.Printf(config.TA("[update] new version failed to start %d times in a row and rollback failed: %v"), maxAttempts, err)
			_ = os.Remove(p.Marker)
			return Continue, State{Detail: "새 버전 시작에 실패하고 롤백도 실패했습니다: " + err.Error()}
		}
		log.Printf(config.TA("[update] new version failed to start %d times in a row, rolled back to %s, exiting to restart (exit %d)"),
			maxAttempts, orUnknown(m.From), ExitRestart)
		_ = os.Remove(p.Marker)
		return Restart, State{RolledBack: true, Detail: fmt.Sprintf("새 버전 시작에 실패해 이전 버전으로 롤백했습니다(%s)", orUnknown(m.From))}
	}
	if err := writeMarker(p.Marker, m); err != nil {
		log.Printf(config.TA("[update] failed to update the upgrade marker: %v"), err)
	}
	log.Printf(config.TA("[update] new version starting (attempt %d/%d), the upgrade will be confirmed after stable operation"),
		m.Attempts, maxAttempts)
	return Continue, State{Pending: true}
}

// Settle confirms the new version is running stably and clears the upgrade marker.
//
// main calls it with a delay after the HTTP listener is up: only surviving that long
// counts, otherwise the marker stays and the next start keeps counting attempts until
// a rollback is triggered.
func Settle() {
	p, err := ResolvePaths()
	if err != nil {
		return
	}
	settle(p)
}

func settle(p Paths) {
	if _, ok := readMarker(p.Marker); !ok {
		return // not a post-upgrade start, nothing to do
	}
	if err := os.Remove(p.Marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf(config.TA("[update] failed to clear the upgrade marker: %v"), err)
		return
	}
	log.Printf(config.TA("[update] new version is stable, upgrade complete (previous version kept as %s)"), p.Old)
}

// SettleDelay is how long the new version must keep running to count as alive.
const SettleDelay = 30 * time.Second

// verifyStaged verifies the staged file: compare the SHA256 first, then actually
// launch it once.
func verifyStaged(p Paths) error {
	want, err := os.ReadFile(p.Sum)
	if err != nil {
		return fmt.Errorf("체크섬 읽기 실패: %w", err)
	}
	got, err := fileSHA256(p.New)
	if err != nil {
		return fmt.Errorf("체크섬 계산 실패: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(string(want)), got) {
		return errors.New("SHA256 이 일치하지 않습니다(다운로드가 손상됐거나 변조됨)")
	}
	return smokeTest(p.New)
}

// smokeTest launches the new binary with -h to confirm it really executes on this
// system. That rules out a whole class of problems: truncated downloads, wrong
// architecture (exec format error), missing dependencies.
func smokeTest(bin string) error {
	if err := os.Chmod(bin, 0o755); err != nil {
		return fmt.Errorf("실행 권한 부여 실패: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-h")
	cmd.Env = append(os.Environ(), smokeEnv+"=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return errors.New("스모크 테스트 시간 초과(새 바이너리가 응답하지 않음)")
	}
	if err != nil {
		snippet := strings.TrimSpace(string(out))
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		return fmt.Errorf("스모크 테스트 실패: %v: %s", err, snippet)
	}
	return nil
}

// swap replaces the current binary with the staged new version.
//
// Both Unix and Windows allow renaming a running executable (Windows forbids deleting
// and overwriting it, renaming is not in that list), so no per-platform split is
// needed here and we do not have to stop ourselves first.
func swap(p Paths) error {
	// Windows rename does not overwrite an existing target, so a .old left by a
	// previous upgrade must be removed first.
	if err := os.Remove(p.Old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("이전 백업 %s 정리 실패: %w", p.Old, err)
	}
	if err := os.Rename(p.Current, p.Old); err != nil {
		return fmt.Errorf("현재 버전 백업 실패: %w", err)
	}
	if err := os.Rename(p.New, p.Current); err != nil {
		// The swap failed but the current version has already been moved away; it must
		// be put back as-is or the next start has no executable.
		if rerr := os.Rename(p.Old, p.Current); rerr != nil {
			return fmt.Errorf("새 버전 설치 실패(%v), 현재 버전 복구도 실패: %w", err, rerr)
		}
		return fmt.Errorf("새 버전 설치 실패: %w", err)
	}
	_ = os.Remove(p.Sum)
	return nil
}

// rollback restores the previous version that swap backed up.
func rollback(p Paths) error {
	if _, err := os.Stat(p.Old); err != nil {
		return fmt.Errorf("롤백할 백업이 없습니다(%s): %w", p.Old, err)
	}
	// Move the version that cannot start to .failed for investigation instead of
	// deleting it.
	failed := p.Current + ".failed"
	_ = os.Remove(failed)
	if err := os.Rename(p.Current, failed); err != nil {
		return fmt.Errorf("실패한 버전 이동 실패: %w", err)
	}
	if err := os.Rename(p.Old, p.Current); err != nil {
		return fmt.Errorf("이전 버전 복구 실패: %w", err)
	}
	return nil
}

// Rollback implements /api/update/rollback: go back to the previous version on
// demand. It only performs the swap; restarting is again left to the supervisor (the
// caller exits with ExitRestart right after).
func Rollback() error {
	p, err := ResolvePaths()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p.Old); err != nil {
		return errors.New("롤백할 이전 버전이 없습니다(" + p.Old + " 없음)")
	}
	cleanStaged(p)
	if err := smokeTest(p.Old); err != nil {
		return fmt.Errorf("이전 버전을 실행할 수 없어 롤백을 거부합니다: %w", err)
	}
	// Swap current and backup so a rollback can itself be rolled back.
	tmp := p.Current + ".swap"
	_ = os.Remove(tmp)
	if err := os.Rename(p.Current, tmp); err != nil {
		return fmt.Errorf("현재 버전 이동 실패: %w", err)
	}
	if err := os.Rename(p.Old, p.Current); err != nil {
		_ = os.Rename(tmp, p.Current)
		return fmt.Errorf("이전 버전 설치 실패: %w", err)
	}
	if err := os.Rename(tmp, p.Old); err != nil {
		log.Printf(config.TA("[update] failed to tidy backups after rollback (operation unaffected): %v"), err)
	}
	_ = os.Remove(p.Marker)
	return nil
}

// HasBackup reports whether a previous version is available for rollback, so the
// frontend can decide whether to show the rollback button.
func HasBackup() bool {
	p, err := ResolvePaths()
	if err != nil {
		return false
	}
	_, err = os.Stat(p.Old)
	return err == nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "알 수 없는 버전"
	}
	return s
}
