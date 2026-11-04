package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/internal/notify"
)

// Global settings keys (kept in the settings key/value table, no table of their own).
const (
	// settingNotifyEnabled is the master push switch. On by default: it exists to stop
	// everything with one switch during maintenance,
	// not as the feature's enable condition — the real condition is whether any channel is
	// configured.
	settingNotifyEnabled = "notify_enabled"
	// settingNotifyPublicBaseURL is the externally reachable address used to build finding
	// detail backlinks
	// (e.g. https://artex.example.com). When empty, messages carry no backlink button.
	// The project had no reusable external-address setting, so this one is new.
	settingNotifyPublicBaseURL = "notify_public_base_url"
	// settingNotifyDigestMinutes is the digest mode interval in minutes.
	settingNotifyDigestMinutes = "notify_digest_interval_min"
)

const (
	// notifyTick is the delivery engine's poll interval. 3 seconds is the engine's realtime
	// ceiling
	// and the main source of latency between a finding being stored and the message reaching IM.
	notifyTick = 3 * time.Second
	// notifyBackgroundDisabledEnv 가 설정되면 서버가 백그라운드 전달 루프
	// (newNotifier(s).Run)를 띄우지 않는다. 통합 테스트는 stepRealtime·stepDigest 를
	// 직접 호출해 분배 결과를 결정론적으로 검증하는데, 3 초 주기의 백그라운드 루프가
	// 같은 채널을 동시에 처리하면 집계가 타이밍에 따라 흔들린다. 이를 막기 위한
	// 테스트 전용 스위치이며, 변수가 없으면 평소대로 기동한다(프로덕션 동작 불변).
	notifyBackgroundDisabledEnv = "ARTEX_NOTIFY_BACKGROUND_DISABLED"
	// notifyLease is the lease taken when claiming a delivery. It must be clearly longer than
	// a single delivery's worst case
	// (the notify package's HTTP client timeout is 15s), otherwise the same row is delivered
	// by two
	// dispatchers at once.
	notifyLease = 3 * time.Minute
	// notifyFanOutPerTick caps how many events are dispatched per round, so that first
	// enabling a channel
	// does not expand the whole historical backlog into deliveries at once.
	notifyFanOutPerTick = 200
	// notifyDefaultDigestMinutes is the default digest interval.
	notifyDefaultDigestMinutes = 30
	// notifyUnlimitedBurstPerTick is the per-round delivery cap for a channel with no rate limit.
	// It exists so that "one unlimited channel plus a scan that turns up thousands of
	// findings"
	// cannot stretch a single round into a long block.
	notifyUnlimitedBurstPerTick = 50
	// notifyMaxSendsPerChannelPerTick is how many deliveries one channel may send per round.
	//
	// The cap is derived backwards from the **lease duration**: claiming stamps the row with a
	// lease (notifyLease = 3 minutes),
	// so if a round delivers enough rows serially that the worst case exceeds the lease, the
	// later rows expire mid-send.
	// Within one process it does not matter (Run is a single serial goroutine, ticks never
	// re-enter), but with **two
	// processes on one database** the other side re-claims lease-expired rows and sends them
	// again, and also
	// double-increments attempts, marking failures while the original process is still delivering.
	//
	// Value: 3-minute lease / 30-second send timeout = 6 fills the lease exactly with zero
	// margin and must not be used;
	// 5 leaves 30 seconds of margin on a 150-second worst case. This relation is pinned by
	// TestNotifyTickBudgetFitsWithinLease — changing notifyLease,
	// notifySendTimeout or this value makes that assertion fail.
	notifyMaxSendsPerChannelPerTick = 5
	// notifySendTimeout is the timeout of a single delivery. It also determines the constant
	// above:
	// their product must not exceed notifyLease, see TestNotifyTickBudgetFitsWithinLease.
	notifySendTimeout = 30 * time.Second
)

// notifyBackoff is the retry backoff sequence, indexed by attempts already made.
// 3 chances (including the first) match db.MaxNotifyAttempts; both must change together.
var notifyBackoff = []time.Duration{
	time.Second,
	5 * time.Second,
	30 * time.Second,
}

// 전달 실패·지연 사유. notification_deliveries.last_error 에 저장되어
// notify_api.go 의 전달 이력 응답을 거쳐 알림 페이지 「전달 기록」 표(delivery-list.tsx)에
// 그대로 표시된다. 따라서 사용자 노출 문구라 한국어로 둔다(진단 로그 log.Printf 는 원문 보존, Z2).
const (
	errDeliveryChannelKindUnregistered = "등록되지 않은 채널 유형입니다: %q"
	errDeliverySnapshotUnrenderable    = "이벤트 스냅샷을 해석하지 못해 이 취약점을 메시지로 만들지 못했습니다"
	errDeliveryChannelLengthCapped     = "이 메시지가 채널 길이 상한에 도달해 앞의 %d건만 전송했고, 나머지는 다음 배치로 미룹니다"
	errDeliveryNoDeliveredCount        = "채널이 전송 건수를 보고하지 않았습니다(delivered=%d)"
	errDeliveryRetryExhausted          = "%d회 재시도 후에도 실패했습니다: %s"
	errDeliverySnapshotEmpty           = "전달 항목 #%d 의 이벤트 스냅샷이 비어 있습니다"
	errDeliverySnapshotParse           = "전달 항목 #%d 의 이벤트 스냅샷을 해석하지 못했습니다: %w"
	errDeliveryBatchAllUnparseable     = "요약 배치의 전달 %d건을 모두 해석하지 못했습니다"
)

// Notifier is the vulnerability push delivery engine.
//
// It runs as its own goroutine alongside the Scheduler (see server.New). It deliberately
// does not reuse
// the Scheduler's tick: pushes need realtime latency (3 seconds), a different cadence from
// triggers,
// and neither's failure may affect the other — a stuck push must not block agent triggers.
type Notifier struct {
	s  *Server
	pg *db.DB

	// mu guards buckets. Channels are few and contention is low, so one mutex is enough;
	// a finer-grained structure is not worth it.
	mu      sync.Mutex
	buckets map[int64]*notifyBucket
}

// notifyBucket is the per-channel token bucket.
//
// A token bucket rather than a "count per minute, then reset" sliding window: the latter
// has terrible boundary effects —
// 20 sends at the end of a window plus 20 right after look like 40 in one second to the
// platform and get rate-limited;
// a bucket refills at a constant rate and naturally avoids such bursts.
type notifyBucket struct {
	tokens   float64
	lastFill time.Time
}

func newNotifier(s *Server) *Notifier {
	return &Notifier{s: s, pg: s.m.pg, buckets: map[int64]*notifyBucket{}}
}

// Run loops until ctx ends. Started once by server.New.
func (n *Notifier) Run(ctx context.Context) {
	if n.pg == nil {
		return
	}
	t := time.NewTicker(notifyTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n.step(ctx)
		}
	}
}

// step runs one round: dispatch new events first, then deliver the due tasks.
//
// Any step failure is only logged and never breaks the loop — a notification system fault
// must never escalate to a process-level problem.
// Each tick is independent, so the next round retries naturally.
func (n *Notifier) step(ctx context.Context) {
	if !n.enabled() {
		return
	}
	if _, _, err := n.pg.FanOutPendingEvents(ctx, notifyFanOutPerTick); err != nil {
		log.Printf(logA("[notify] failed to dispatch event: %v"), err)
		return
	}
	channels, err := n.pg.ListNotificationChannels(ctx)
	if err != nil {
		log.Printf(logA("[notify] failed to read channels: %v"), err)
		return
	}
	baseURL := n.publicBaseURL()
	for _, ch := range channels {
		if !ch.IsEnabled() {
			continue
		}
		// The token bucket counts **messages** (equivalent to HTTP requests), not findings.
		// In realtime mode they are the same (one finding, one message); a digest batch collapses
		// into one message, so it consumes a single token.
		//
		// Both modes ask the bucket first and claim by budget second — the order cannot be
		// reversed, or a delivery stopped by the rate limit
		// would already have burned a retry attempt.
		now := time.Now()
		if ch.Mode == db.NotifyModeDigest {
			tokens, claimLimit := digestTickPlan()
			if n.takeTokens(ch.ID, ch.RatePerMin, tokens, now) <= 0 {
				continue
			}
			n.stepDigest(ctx, ch, claimLimit, baseURL)
			continue
		}
		allow := n.takeTokens(ch.ID, ch.RatePerMin, notifyMaxSendsPerChannelPerTick, now)
		if allow <= 0 {
			continue
		}
		n.stepRealtime(ctx, ch, allow, baseURL)
	}
}

// digestTickPlan returns a digest channel's token cost and batch-size upper bound for this
// round.
//
// The two return values have **two different units**, which is exactly why this is its own
// function:
//
//   - tokens counts messages. A batch of findings becomes one message and one HTTP request,
//     so it is always 1.
//     rate_per_min therefore still applies to digest (at most that many digest messages per
//     minute).
//   - claimLimit is how many findings one batch may hold. It is bounded only by memory,
//     independent of the request budget.
//
// It once passed the per-round request budget
// (notifyMaxSendsPerChannelPerTick, derived from the lease) down as the batch size, just to
// make rate_per_min apply to digest.
// The result: a channel with rate_per_min=20 refilled only 1 token per 3-second tick, so
// every digest
// message held exactly 1 finding — digest degenerated into "realtime push with digest
// wording" and readers got a stream of
// "1 new finding in the last 30 minutes", while db.MaxDigestBatchSize stayed unreachable.
//
// That symptom is hard to catch in End-To-End tests (existing cases pass a large enough
// limit into
// stepDigest by hand, bypassing step's budget calculation), so the decision is pinned here by
// TestDigestTickPlanDecouplesBatchSizeFromSendBudget.
func digestTickPlan() (tokens, claimLimit int) {
	return 1, db.MaxDigestBatchSize
}

// stepRealtime claims and delivers one channel's realtime tasks, one finding per message.
func (n *Notifier) stepRealtime(ctx context.Context, ch *db.NotificationChannel, allow int, baseURL string) {
	deliveries, err := n.pg.ClaimRealtimeDeliveries(ctx, ch.ID, allow, notifyLease)
	if err != nil {
		log.Printf(logA("[notify] failed to claim real-time delivery channel=%d: %v"), ch.ID, err)
		return
	}
	if len(deliveries) == 0 {
		return
	}
	channel, cfg, ok := n.adapt(ch)
	if !ok {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), fmt.Sprintf(trMsg(errDeliveryChannelKindUnregistered), ch.Kind))
		return
	}
	for _, dl := range deliveries {
		msg, err := n.renderSingle(ctx, dl, baseURL)
		if err != nil {
			// A render failure is a local data problem; retrying will not fix it.
			_ = n.pg.FailDeliveries(ctx, []int64{dl.ID}, err.Error())
			continue
		}
		n.send(ctx, channel, cfg, msg, []*db.NotificationDelivery{dl})
	}
}

// stepDigest aggregates a channel's pending deliveries into one message once the batch is due.
func (n *Notifier) stepDigest(ctx context.Context, ch *db.NotificationChannel, allow int, baseURL string) {
	window := n.digestInterval()
	due, err := n.pg.DigestBatchDue(ctx, ch.ID, window)
	if err != nil {
		log.Printf(logA("[notify] failed to evaluate digest batch channel=%d: %v"), ch.ID, err)
		return
	}
	if !due {
		return
	}
	deliveries, err := n.pg.ClaimDigestBatch(ctx, ch.ID, allow, notifyLease)
	if err != nil {
		log.Printf(logA("[notify] failed to claim digest batch channel=%d: %v"), ch.ID, err)
		return
	}
	if len(deliveries) == 0 {
		return
	}
	channel, cfg, ok := n.adapt(ch)
	if !ok {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), fmt.Sprintf(trMsg(errDeliveryChannelKindUnregistered), ch.Kind))
		return
	}
	msg, included, err := n.renderBatch(ctx, deliveries, baseURL, int(window.Minutes()))
	if err != nil {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), err.Error())
		return
	}
	// Deliveries whose snapshot broke and never made it into the message must be failed
	// explicitly. Otherwise they stay
	// outside included, in neither the message nor the failed list — on a successful send their
	// status is
	// skipped by the batch marking and they sit in sending until the lease expires, claimed over
	// and over.
	if skipped := excludeDeliveries(deliveries, included); len(skipped) > 0 {
		reason := errDeliverySnapshotUnrenderable
		if fErr := n.pg.FailDeliveries(ctx, deliveryIDs(skipped), reason); fErr != nil {
			log.Printf(logA("[notify] failed to mark bad-snapshot deliveries as failed channel=%s ids=%v: %v"), ch.Kind, deliveryIDs(skipped), fErr)
		}
		log.Printf(logA("[notify] skipping %d deliveries whose snapshot cannot be parsed channel=%d"), len(skipped), ch.ID)
	}
	// Only the ones that made it into the message go to send: included[i] and msg.Items[i]
	// correspond strictly,
	// and send relies on that to land "the channel took the first K" on the right delivery rows.
	n.send(ctx, channel, cfg, msg, included)
}

// send delivers and moves state according to the result.
//
// A whole batch (dozens of rows in digest mode) shares one send result: either delivered or
// the batch retries as a whole.
// There is no per-row retry — a digest message is one unit, and resending part of it would
// corrupt the batch semantics.
//
// The one exception is **segmentation by the channel's length limit**: when the channel
// reports it only took the first K,
// row K+1 onward must wait for the next batch rather than being marked delivered with the
// rest. Otherwise the truncated
// findings end up in neither the message nor the failed list and vanish entirely.
func (n *Notifier) send(ctx context.Context, channel notify.Channel, cfg map[string]any, msg notify.Message, deliveries []*db.NotificationDelivery) {
	// Cap a single delivery so one stuck channel cannot hold up every remaining channel this
	// round.
	sendCtx, cancel := context.WithTimeout(ctx, notifySendTimeout)
	defer cancel()
	delivered, err := channel.Send(sendCtx, cfg, msg)
	if err == nil && delivered > 0 {
		if delivered > len(deliveries) {
			// The reported count cannot exceed the number of deliveries; if it does, the render layer
			// computed wrong —
			// treat everything as delivered and log it, which beats corrupting the records.
			log.Printf(logA("[notify] channel reported %d delivered of %d deliveries channel=%s, treating all as delivered"),
				delivered, len(deliveries), channel.Kind())
			delivered = len(deliveries)
		}
		sent, rest := deliveries[:delivered], deliveries[delivered:]
		if err := n.pg.MarkDeliveriesSent(ctx, deliveryIDs(sent)); err != nil {
			log.Printf(logA("[notify] failed to mark deliveries as delivered channel=%s ids=%v: %v"), channel.Kind(), deliveryIDs(sent), err)
		}
		if len(rest) > 0 {
			// This message hit the channel's length limit: the rest go straight back to the queue and
			// the next tick continues.
			// DeferDeliveries, not RescheduleDeliveries — this is not a failure
			// and must not burn retry budget (claiming already incremented it optimistically and that
			// is undone there).
			if err := n.pg.DeferDeliveries(ctx, deliveryIDs(rest),
				fmt.Sprintf(trMsg(errDeliveryChannelLengthCapped), delivered)); err != nil {
				log.Printf(logA("[notify] failed to requeue segmented continuation channel=%s ids=%v: %v"), channel.Kind(), deliveryIDs(rest), err)
			}
		}
		return
	}
	if err == nil {
		// The channel neither errored nor said how many it delivered. Treat it as a failure (with
		// backoff),
		// so the delivery is not claimed over and over while never being marked.
		err = fmt.Errorf(trMsg(errDeliveryNoDeliveredCount), delivered)
	}

	// Failure handling is decided **per row**, not by the batch's max attempt count.
	//
	// It used to be `if maxAttempts(deliveries) >= MaxNotifyAttempts` failing the whole batch,
	// but within a batch
	// attempts differ: an old delivery already retried twice (attempts=2) dragged fresh ones in
	// the same batch
	// (attempts=1) into failed — a new finding was lost permanently without a single retry,
	// the exact opposite of "don't let old rows sink new ones".
	permanent := notify.IsPermanent(err)
	var failIDs, exhaustedIDs []int64
	byDelay := map[time.Duration][]int64{}
	for _, dl := range deliveries {
		switch {
		case permanent:
			failIDs = append(failIDs, dl.ID)
		case dl.Attempts >= db.MaxNotifyAttempts:
			exhaustedIDs = append(exhaustedIDs, dl.ID)
		default:
			delay := notifyBackoff[min(dl.Attempts, len(notifyBackoff)-1)]
			byDelay[delay] = append(byDelay[delay], dl.ID)
		}
	}

	if len(failIDs) > 0 {
		if fErr := n.pg.FailDeliveries(ctx, failIDs, err.Error()); fErr != nil {
			log.Printf(logA("[notify] failed to mark delivery failure state channel=%s ids=%v: %v"), channel.Kind(), failIDs, fErr)
		}
	}
	if len(exhaustedIDs) > 0 {
		reason := fmt.Sprintf(trMsg(errDeliveryRetryExhausted), db.MaxNotifyAttempts, err)
		if fErr := n.pg.FailDeliveries(ctx, exhaustedIDs, reason); fErr != nil {
			log.Printf(logA("[notify] failed to mark delivery failure state channel=%s ids=%v: %v"), channel.Kind(), exhaustedIDs, fErr)
		}
	}
	// Regroup by delay: there are only 3 backoff steps, so the groups are naturally few and
	// there is no need to send one
	// UPDATE per row (which would turn a 500-row batch into 500 round trips).
	for delay, group := range byDelay {
		if rErr := n.pg.RescheduleDeliveries(ctx, group, delay, err.Error()); rErr != nil {
			log.Printf(logA("[notify] failed to reorder deliveries channel=%s ids=%v: %v"), channel.Kind(), group, rErr)
		}
	}
	if len(failIDs)+len(exhaustedIDs) > 0 {
		log.Printf(logA("[notify] delivery failed channel=%d kind=%s permanently_failed=%d retries_exhausted=%d pending_retry=%d: %s"),
			deliveries[0].ChannelID, channel.Kind(), len(failIDs), len(exhaustedIDs), len(byDelay), err)
	}
}

// excludeDeliveries returns the entries of all that are not in keep (compared by pointer
// identity).
// It finds the deliveries that never made it into a message — they must be handled
// explicitly, not left in limbo.
func excludeDeliveries(all, keep []*db.NotificationDelivery) []*db.NotificationDelivery {
	inKeep := make(map[*db.NotificationDelivery]bool, len(keep))
	for _, dl := range keep {
		inKeep[dl] = true
	}
	var out []*db.NotificationDelivery
	for _, dl := range all {
		if !inKeep[dl] {
			out = append(out, dl)
		}
	}
	return out
}

// adapt fetches the channel implementation and parses its config.
// ok=false means the type is not registered; the delivery must fail outright rather than
// retry forever.
func (n *Notifier) adapt(ch *db.NotificationChannel) (notify.Channel, map[string]any, bool) {
	channel, ok := notify.Get(ch.Kind)
	if !ok {
		return nil, nil, false
	}
	var cfg map[string]any
	if len(ch.Config) > 0 {
		// On a config parse failure pass an empty map: the channel's own Validate reports which
		// field is missing,
		// and that error guides the user's fix better than a JSON parse error.
		_ = json.Unmarshal(ch.Config, &cfg)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return channel, cfg, true
}

// renderSingle renders a single-finding message.
func (n *Notifier) renderSingle(ctx context.Context, dl *db.NotificationDelivery, baseURL string) (notify.Message, error) {
	snap, err := parseSnapshot(dl)
	if err != nil {
		return notify.Message{}, err
	}
	item, err := n.itemFor(ctx, snap, baseURL)
	if err != nil {
		return notify.Message{}, err
	}
	return notify.Message{Items: []notify.Item{item}, HomeURL: baseURL}, nil
}

// renderBatch renders a digest message. Snapshots are parsed one by one — a single broken
// one only drops that entry,
// never the whole batch.
//
// The returned included corresponds **strictly one-to-one** with msg.Items (i-th delivery ↔
// i-th entry).
// That is a hard requirement: the caller marks the first K deliveries delivered based on
// "the channel took the first K".
// If a broken snapshot were skipped without removing its delivery from included,
// the indexes would shift — a bad entry that should fail would be marked delivered and a
// good one treated as undelivered.
// The caller fails the broken ones explicitly, see stepDigest.
func (n *Notifier) renderBatch(ctx context.Context, deliveries []*db.NotificationDelivery, baseURL string, windowMinutes int) (notify.Message, []*db.NotificationDelivery, error) {
	items := make([]notify.Item, 0, len(deliveries))
	included := make([]*db.NotificationDelivery, 0, len(deliveries))
	for _, dl := range deliveries {
		snap, err := parseSnapshot(dl)
		if err != nil {
			// A broken snapshot goes into neither the message nor included — the caller handles it
			// (explicit failure, not smuggled in as delivered).
			log.Printf(logA("[notify] skipped unparseable snapshot in digest batch delivery=%d: %v"), dl.ID, err)
			continue
		}
		item, err := n.itemFor(ctx, snap, baseURL)
		if err != nil {
			return notify.Message{}, nil, err
		}
		items = append(items, item)
		included = append(included, dl)
	}
	if len(items) == 0 {
		return notify.Message{}, nil, fmt.Errorf(trMsg(errDeliveryBatchAllUnparseable), len(deliveries))
	}
	return notify.Message{
		Items:         items,
		Batch:         true,
		WindowMinutes: windowMinutes,
		HomeURL:       baseURL,
	}, included, nil
}

// itemFor renders an event snapshot into a push item, resolving the asset name and detail
// backlink along the way.
func (n *Notifier) itemFor(ctx context.Context, snap notify.Snapshot, baseURL string) (notify.Item, error) {
	assets, err := n.pg.NotificationAssetNames(ctx, snap.AssetIDs)
	if err != nil {
		// Failing to resolve the asset name must not block the push: a missing name is far cheaper
		// than a missing notification,
		// the message just has one row less.
		log.Printf(logA("[notify] failed to parse asset name finding=%d: %v"), snap.FindingID, err)
	}
	item := notify.Item{
		FindingID:  snap.FindingID,
		Name:       snap.Name,
		VulnClass:  snap.VulnClass,
		Severity:   snap.Severity,
		Summary:    snap.Summary,
		Assets:     assets,
		FromStatus: snap.FromStatus,
		ToStatus:   snap.ToStatus,
	}
	if baseURL != "" {
		// Detail page route: web/src/app/(main)/function/findings/detail/page.tsx;
		// it reads the finding id from the id query param.
		item.DetailURL = fmt.Sprintf("%s/function/findings/detail?id=%d", baseURL, snap.FindingID)
	}
	return item, nil
}

// takeTokens removes **up to want** tokens from a channel's bucket and returns how many it
// got.
//
// One token = one message (one HTTP request). Realtime callers pass the number of findings;
// a whole digest batch is one message, so pass 1.
//
// The bucket holds the channel's per-minute limit and refills at a constant rate.
// ratePerMin<=0 means unlimited:
// return a finite but large value so one round cannot be blocked by an unbounded backlog.
//
// The want cap is required: without it the bucket is drained entirely while the caller still
// has its own per-round cap,
// so the extra tokens are unusable and vanish before the next refill — accumulated burst
// capacity stays unreachable
// and even a round with nothing to send is charged.
func (n *Notifier) takeTokens(channelID int64, ratePerMin, want int, now time.Time) int {
	if want <= 0 {
		return 0
	}
	if ratePerMin <= 0 {
		return min(want, notifyUnlimitedBurstPerTick)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	b := n.buckets[channelID]
	if b == nil {
		b = &notifyBucket{tokens: float64(ratePerMin), lastFill: now}
		n.buckets[channelID] = b
	}
	// Refill by real elapsed time at ratePerMin/60 per second.
	if elapsed := now.Sub(b.lastFill).Seconds(); elapsed > 0 {
		b.tokens = minF(float64(ratePerMin), b.tokens+elapsed*float64(ratePerMin)/60)
		b.lastFill = now
	}
	// Add a tiny epsilon before truncating: tokens accumulate as floats, so two half-refills
	// can produce 0.9999999999 from 0.5 + 0.5, which int() truncates to 0 —
	// a mathematically full bucket that yields no token. 1e-9 is far below one token and never
	// forgives a real shortfall.
	take := min(int(b.tokens+1e-9), want)
	if take <= 0 {
		return 0
	}
	b.tokens -= float64(take)
	return take
}

// enabled reads the master switch.
func (n *Notifier) enabled() bool {
	return n.pg.GetBool(settingNotifyEnabled, true)
}

// publicBaseURL returns the external address used for backlinks, trailing slash removed.
func (n *Notifier) publicBaseURL() string {
	v, ok, err := n.pg.GetSetting(settingNotifyPublicBaseURL)
	if err != nil || !ok {
		return ""
	}
	return trimTrailingSlash(v)
}

// digestInterval returns the digest interval, falling back to the default when invalid or
// unset.
func (n *Notifier) digestInterval() time.Duration {
	v, ok, err := n.pg.GetSetting(settingNotifyDigestMinutes)
	if err != nil || !ok {
		return time.Duration(notifyDefaultDigestMinutes) * time.Minute
	}
	m := 0
	if _, err := fmt.Sscanf(v, "%d", &m); err != nil || m <= 0 {
		return time.Duration(notifyDefaultDigestMinutes) * time.Minute
	}
	return time.Duration(m) * time.Minute
}

// parseSnapshot parses the snapshot of the event a delivery belongs to.
func parseSnapshot(dl *db.NotificationDelivery) (notify.Snapshot, error) {
	var snap notify.Snapshot
	if len(dl.Snapshot) == 0 {
		return snap, fmt.Errorf(trMsg(errDeliverySnapshotEmpty), dl.ID)
	}
	if err := json.Unmarshal(dl.Snapshot, &snap); err != nil {
		return snap, fmt.Errorf(trMsg(errDeliverySnapshotParse), dl.ID, err)
	}
	if snap.Kind == "" {
		// The event type comes from the event row; the copy in the snapshot may be from an older
		// version.
		snap.Kind = dl.EventKind
	}
	return snap, nil
}

func deliveryIDs(deliveries []*db.NotificationDelivery) []int64 {
	out := make([]int64, 0, len(deliveries))
	for _, dl := range deliveries {
		out = append(out, dl.ID)
	}
	return out
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
