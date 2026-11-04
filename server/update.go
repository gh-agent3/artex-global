package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/Autumn-27/artex/internal/selfupdate"
)

// The HTTP surface of the one-click page update. The actual download/verify/swap logic lives in the
// selfupdate package; this file only holds the auth boundary, the upgrade mutex, progress
// broadcast, and telling main "it is time to exit".
//
// The process does not restart itself: once the new version is staged it exits with
// selfupdate.ExitRestart and the supervisor script (start.sh / start.bat, or the Dockerfile
// ENTRYPOINT) brings it back up.

// 사용자에게 노출되는 업데이트 메시지(SSE 진행 메시지·검사 사유·에러 응답).
// 프런트엔드(system/settings 의 update-card)가 progress.message·reason·writeErr
// 본문을 그대로 렌더하므로 한국어로 둔다. 로그·중국어 주석은 Z2(로그 최하위)라 범위 밖.
const (
	updateMsgPreparing          = "준비 중…"
	updateMsgFailed             = "업데이트 실패"
	updateMsgStaged             = "새 버전이 준비되었습니다. 재시작 중…"
	updateErrNotReleaseFmt      = "현재 버전 %q 은(는) 정식 릴리스 버전이 아니므로 원클릭 업데이트를 사용할 수 없습니다"
	updateErrAlreadyLatestFmt   = "이미 최신 버전입니다 (%s)"
	updateErrInProgress         = "이미 업데이트가 진행 중입니다"
	updateErrRollbackInProgress = "업데이트가 진행 중이어서 롤백할 수 없습니다"
)

// restartCh closes when the upgrade is staged or the rollback has completed; main exits with ExitRestart on it.
var (
	restartOnce sync.Once
	restartCh   = make(chan struct{})
)

// RestartRequested returns a channel that closes when "exit and let the supervisor bring me back up" is requested.
func RestartRequested() <-chan struct{} { return restartCh }

func requestRestart() { restartOnce.Do(func() { close(restartCh) }) }

// bootState is the conclusion of selfupdate.Bootstrap for this boot (upgrade succeeded / just rolled
// back / the staged bundle was discarded), injected by main so /api/update/check can honestly tell
// the frontend how the previous upgrade ended.
var (
	bootStateMu sync.Mutex
	bootState   selfupdate.State
)

// SetBootUpdateState is called once by main at startup.
func SetBootUpdateState(st selfupdate.State) {
	bootStateMu.Lock()
	defer bootStateMu.Unlock()
	bootState = st
}

func bootUpdateState() selfupdate.State {
	bootStateMu.Lock()
	defer bootStateMu.Unlock()
	return bootState
}

// releaseCache caches the latest-release lookup against GitHub.
//
// The top-bar "update available" hint queries on every full page load, and an unauthenticated GitHub
// API call is limited to 60 per IP per hour - without a cache, a few open tabs or a couple of refreshes
// would burn the quota, and the real update would then have nothing to query. An explicit "check for
// updates" click may bypass the cache with force.
type releaseCache struct {
	mu  sync.Mutex
	rel *selfupdate.Release
	err error
	at  time.Time
	// fetch is the data function, an injection point for tests only; nil means the real GitHub query.
	fetch func(context.Context, *http.Client) (*selfupdate.Release, error)
}

const (
	releaseTTL = 30 * time.Minute
	// Even a failure is cached briefly, otherwise every page load would wait out a timeout while GitHub
	// is unreachable; but the TTL is short so it recovers as soon as the network does.
	releaseErrTTL = 2 * time.Minute
	// Timeout for the lookup. NewClient's 30-minute timeout is for downloading a whole release; a
	// version check must not wait that long.
	releaseTimeout = 20 * time.Second
)

var relCache = &releaseCache{}

// get returns the latest Release, serving from the cache when it can.
//
// The lock is held for the whole fetch: concurrent requests queue up for the same result instead of
// each hitting GitHub on its own (a freshly loaded page with several tabs is exactly the moment rate
// limiting is most likely).
func (c *releaseCache) get(ctx context.Context, client *http.Client, force bool) (*selfupdate.Release, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !force {
		ttl := releaseTTL
		if c.err != nil {
			ttl = releaseErrTTL
		}
		if !c.at.IsZero() && time.Since(c.at) < ttl {
			return c.rel, c.err
		}
	}

	fetch := c.fetch
	if fetch == nil {
		fetch = selfupdate.FetchLatest
	}
	ctx, cancel := context.WithTimeout(ctx, releaseTimeout)
	defer cancel()
	rel, err := fetch(ctx, client)
	// A cancelled request (the user closed the tab) does not mean GitHub is broken, so do not cache it;
	// otherwise the next visitor gets a mysterious "cancelled" error.
	if err != nil && ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
		return c.rel, err
	}
	c.rel, c.err, c.at = rel, err, time.Now()
	return rel, err
}

// updateProgress is one progress message pushed to the frontend.
type updateProgress struct {
	Phase   selfupdate.Phase `json:"phase"`
	Percent int              `json:"percent"` // only meaningful in the download phase; -1 otherwise
	Message string           `json:"message"`
	Version string           `json:"version,omitempty"`
	Error   string           `json:"error,omitempty"`
}

// updateHub holds the progress of one upgrade and broadcasts it to SSE subscribers.
//
// running doubles as the mutex: a second POST /api/update/apply during an upgrade gets 409, so two
// goroutines never write the same artex.new at once.
type updateHub struct {
	mu      sync.Mutex
	running bool
	cur     updateProgress
	subs    map[chan updateProgress]struct{}
}

var updHub = &updateHub{
	cur:  updateProgress{Phase: selfupdate.PhaseIdle, Percent: -1},
	subs: map[chan updateProgress]struct{}{},
}

// begin claims the upgrade right and returns false if one is already running.
func (h *updateHub) begin(version string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		return false
	}
	h.running = true
	h.cur = updateProgress{Phase: selfupdate.PhaseDownload, Percent: 0, Message: updateMsgPreparing, Version: version}
	h.fanout(h.cur)
	return true
}

// finish ends an upgrade. err nil means the staging succeeded and we are waiting for the restart.
func (h *updateHub) finish(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = false
	if err != nil {
		h.cur = updateProgress{Phase: selfupdate.PhaseFailed, Percent: -1, Message: updateMsgFailed, Error: err.Error(), Version: h.cur.Version}
	} else {
		h.cur = updateProgress{Phase: selfupdate.PhaseStaged, Percent: 100, Message: updateMsgStaged, Version: h.cur.Version}
	}
	h.fanout(h.cur)
}

func (h *updateHub) publish(ph selfupdate.Phase, pct int, msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cur = updateProgress{Phase: ph, Percent: pct, Message: msg, Version: h.cur.Version}
	h.fanout(h.cur)
}

// fanout must be called with h.mu held. Subscriber channels are buffered; a full one drops the event -
// progress is transient and a stuck SSE connection must never block the upgrade itself.
func (h *updateHub) fanout(p updateProgress) {
	for ch := range h.subs {
		select {
		case ch <- p:
		default:
		}
	}
}

func (h *updateHub) snapshot() (updateProgress, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cur, h.running
}

func (h *updateHub) subscribe() (<-chan updateProgress, func()) {
	ch := make(chan updateProgress, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
}

// updateCheck queries GitHub for the latest release and compares it to the running version.
//
// The frontend could query api.github.com directly (GitHub CORS is *), but **this endpoint is the
// reference**: the download is done by the backend, and only a server that can reach GitHub can update.
// It is common for the browser to be connected but the server not (an internal network, or a proxy
// configured only in the browser) - then updating would fail anyway, so the check reports it honestly here.
func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	current := BuildVersion
	mode := "binary"
	if selfupdate.InDocker() {
		mode = "docker"
	}
	boot := bootUpdateState()
	out := map[string]any{
		"current":     current,
		"mode":        mode,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"has_backup":  selfupdate.HasBackup(),
		"repo":        selfupdate.Repo,
		"boot_notice": boot.Detail,
		"rolled_back": boot.RolledBack,
	}

	// The top-bar hint uses the cache (default); an explicit "check for updates" click passes force=1 to bypass it.
	force := r.URL.Query().Get("force") != ""
	client := selfupdate.NewClient(s.m.GlobalProxy())
	rel, err := relCache.get(r.Context(), client, force)
	if err != nil {
		out["error"] = err.Error()
		writeJSON(w, 200, out)
		return
	}

	latest := rel.TagName
	out["latest"] = latest
	out["notes"] = rel.Body
	out["html_url"] = rel.HTMLURL
	if !rel.PublishedAt.IsZero() {
		out["published_at"] = rel.PublishedAt.Format(time.RFC3339)
	}

	asset := selfupdate.AssetName(latest, runtime.GOOS, runtime.GOARCH)
	out["asset"] = asset
	if a, ok := rel.FindAsset(asset); ok {
		out["asset_available"] = true
		out["size"] = a.Size
	} else {
		out["asset_available"] = false
	}

	cmp, comparable := selfupdate.CompareVersions(current, latest)
	out["comparable"] = comparable
	out["has_update"] = comparable && cmp < 0
	if !comparable {
		// A development build (dev / a git-describe suffix) has no comparable version number. Allowing an
		// update would replace the binary being debugged with the release build, so no update is offered.
		out["reason"] = fmt.Sprintf(updateErrNotReleaseFmt, current)
	}
	writeJSON(w, 200, out)
}

// updateApply downloads and stages the new version, then lets the process exit so the supervisor restarts it.
//
// It answers 202 immediately while the real work runs in a background goroutine: a full download may
// take minutes and would be cut by the reverse proxy timeout if it hung on the request. Progress is
// streamed on /api/update/stream.
func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	current := BuildVersion

	// Through the cache: what gets installed is exactly the version the user saw and confirmed on the page.
	client := selfupdate.NewClient(s.m.GlobalProxy())
	rel, err := relCache.get(r.Context(), client, false)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	cmp, comparable := selfupdate.CompareVersions(current, rel.TagName)
	if !comparable {
		writeErr(w, 400, fmt.Sprintf(updateErrNotReleaseFmt, current))
		return
	}
	if cmp >= 0 {
		writeErr(w, 400, fmt.Sprintf(updateErrAlreadyLatestFmt, current))
		return
	}
	if !updHub.begin(rel.TagName) {
		writeErr(w, 409, updateErrInProgress)
		return
	}

	go func() {
		// Deliberately s.ctx, not the request ctx: the request is over as soon as the response is sent, and a
		// download hanging on it would be cancelled at once.
		err := selfupdate.Stage(s.ctx, client, rel, current, func(ph selfupdate.Phase, pct int, msg string) {
			updHub.publish(ph, pct, msg)
		})
		updHub.finish(err)
		if err != nil {
			log.Printf(logA("[update] update failed: %v"), err)
			return
		}
		log.Printf(logA("[update] %s -> %s staged, exiting shortly to complete the swap"), current, rel.TagName)
		// Leave a moment to push the last progress event to the frontend, then exit.
		time.Sleep(1500 * time.Millisecond)
		requestRestart()
	}()

	writeJSON(w, 202, map[string]any{"ok": true, "target": rel.TagName})
}

// updateRollback actively rolls back to the previous version (artex.old, backed up before the swap).
func (s *Server) updateRollback(w http.ResponseWriter, r *http.Request) {
	if _, running := updHub.snapshot(); running {
		writeErr(w, 409, updateErrRollbackInProgress)
		return
	}
	if err := selfupdate.Rollback(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	log.Print(logA("[update] manually rolled back to the previous version, exiting shortly to complete the switch"))
	writeJSON(w, 202, map[string]any{"ok": true})
	go func() {
		time.Sleep(500 * time.Millisecond)
		requestRestart()
	}()
}

// updateStream streams the upgrade progress over SSE.
func (s *Server) updateStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := updHub.subscribe()
	defer unsub()

	send := func(p updateProgress) {
		b, _ := json.Marshal(p)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}
	// Send the current state first, so a refreshed page immediately sees the upgrade in progress.
	cur, _ := updHub.snapshot()
	send(cur)

	ctx := r.Context()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case p, ok := <-ch:
			if !ok {
				return
			}
			send(p)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
