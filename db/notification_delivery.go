package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// This file covers claiming deliveries and their state transitions.
//
// Claiming uses a lease rather than a long transaction: a row is set to sending and
// next_attempt_at is pushed into the future as the lease expiry, and the network send
// happens after the transaction commits. That way no database lock is held during the
// send — a network request can take seconds (client timeout 15s), and holding a row
// lock that long would drag down every other write on the same database.
//
// The cost is that a process crashing mid-send leaves the row in sending. That is
// **self-healing**: once the lease expires next_attempt_at falls into the past and the
// next claim picks the same row up again (see state IN ('pending','sending') in the
// claim condition). The attempt counter is already incremented at claim time, so a
// crash cannot cause endless retries — after MaxNotifyAttempts chances the row drops
// into failed and waits for a human.

// MaxNotifyAttempts is the maximum number of attempts for one delivery (first one
// included). It lives here rather than in the delivery engine: it is the state
// machine's own policy, and the engine merely executes it.
const MaxNotifyAttempts = 3

// MaxDigestBatchSize is the maximum number of deliveries merged into one digest batch.
//
// The reason is resources: if a digest period uncovers tens of thousands of findings
// (entirely possible — one full scan can do it), an unbounded claim would read every
// row into memory, render one enormous message and have most of it cut off by the
// channel's length limit — wasting memory and **silently losing** the findings that
// were cut. With a bound, the surplus stays in the DB as the next batch and goes out
// on the following period instead of being lost.
//
// 500 is chosen because, once rendered, that is the order of magnitude that still has
// readable content within WeCom's 4096-byte limit; anything larger only moves the
// truncation point further along.
const MaxDigestBatchSize = 500

// NotificationDelivery is one delivery task, carrying the channel configuration and
// event snapshot needed to render it.
type NotificationDelivery struct {
	ID            int64           `json:"id"`
	EventID       int64           `json:"event_id"`
	ChannelID     int64           `json:"channel_id"`
	State         string          `json:"state"`
	Attempts      int             `json:"attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	LastError     string          `json:"last_error"`
	BatchID       *int64          `json:"batch_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	SentAt        *time.Time      `json:"sent_at,omitempty"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
	// Render context loaded by the join; not part of the JSON (the server layer builds
	// the DTO).
	Channel *NotificationChannel `json:"-"`
	// FindingID/EventKind come from the event so the history list can link straight to
	// the finding.
	FindingID int64  `json:"finding_id,string"`
	EventKind string `json:"event_kind"`
	// ChannelName/ChannelKind are redundant fields for the list view, saving the UI a
	// second query.
	ChannelName string `json:"channel_name"`
	ChannelKind string `json:"channel_kind"`
}

const notificationDeliveryCols = `d.id, d.event_id, d.channel_id, d.state, d.attempts, d.next_attempt_at,
       d.last_error, d.batch_id, d.created_at, d.sent_at`

// joinedDeliveryQuery is the single read shape for a delivery row: delivery + event
// snapshot + channel configuration. Rendering a message needs all three, and querying
// them separately would mean three round trips.
const joinedDeliveryQuery = `SELECT ` + notificationDeliveryCols + `,
       e.snapshot, e.kind, e.finding_id,
       c.id, c.name, c.kind, c.enabled, c.config, c.mode, c.filter, c.rate_per_min
FROM notification_deliveries d
JOIN notification_events e ON e.id = d.event_id
JOIN notification_channels c ON c.id = d.channel_id`

func scanNotificationDelivery(sc interface{ Scan(...any) error }) (*NotificationDelivery, error) {
	var (
		dl        NotificationDelivery
		lastErr   sql.NullString
		batchID   sql.NullInt64
		sentAt    sql.NullTime
		snapshot  []byte
		eventKind string
		channel   NotificationChannel
		chEnabled bool
	)
	if err := sc.Scan(&dl.ID, &dl.EventID, &dl.ChannelID, &dl.State, &dl.Attempts, &dl.NextAttemptAt,
		&lastErr, &batchID, &dl.CreatedAt, &sentAt,
		&snapshot, &eventKind, &dl.FindingID,
		&channel.ID, &channel.Name, &channel.Kind, &chEnabled, &channel.Config, &channel.Mode, &channel.Filter, &channel.RatePerMin); err != nil {
		return nil, err
	}
	dl.LastError = lastErr.String
	if batchID.Valid {
		dl.BatchID = &batchID.Int64
	}
	if sentAt.Valid {
		dl.SentAt = &sentAt.Time
	}
	dl.Snapshot = json.RawMessage(snapshot)
	dl.EventKind = eventKind
	dl.ChannelName = channel.Name
	dl.ChannelKind = channel.Kind
	channel.Enabled = &chEnabled
	dl.Channel = &channel
	return &dl, nil
}

// claimQuery describes one claim: select candidates with sel and lock them, then set
// them to sending and extend the lease. The lease placeholder inside sel is written by
// the caller as $n and passed in its args.
type claimQuery struct {
	sql  string
	args []any
}

// ClaimRealtimeDeliveries claims one batch of due realtime deliveries for a channel,
// at most limit of them.
//
// Claiming is deliberately per **single channel** rather than "claim globally, then pick
// what to send": the rate limiter is maintained per channel in the delivery engine, and
// only by first knowing how many more messages this channel may send this round and
// then claiming exactly that many rows does the limit avoid burning retry attempts. If
// rows were claimed and then dropped, the ones blocked by the limiter would already
// have consumed an attempt, and the budget of 3 would be spent on pure waiting until
// the row lands in failed.
//
// The condition includes "sending with an expired lease" — that is the crash-recovery
// hook. The lease must be clearly longer than the worst-case duration of one send (the
// channel HTTP client timeout is 15s), otherwise the same row could be delivered by two
// dispatchers at once. Disabled channels are excluded as well: disabling already marked
// existing deliveries as skipped, and this is a second guard against a race between
// disabling and claiming.
func (d *DB) ClaimRealtimeDeliveries(ctx context.Context, channelID int64, limit int, lease time.Duration) ([]*NotificationDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	return d.claimDeliveries(ctx, lease, claimQuery{
		sql: `SELECT dd.id FROM notification_deliveries dd
JOIN notification_channels c ON c.id = dd.channel_id
WHERE dd.channel_id = $1 AND dd.state IN ($2,$3) AND dd.next_attempt_at <= now()
  AND c.enabled AND c.mode = $4
ORDER BY dd.next_attempt_at, dd.id
FOR UPDATE OF dd SKIP LOCKED
LIMIT $5`,
		args: []any{channelID, NotifyStatePending, NotifyStateSending, NotifyModeRealtime, limit},
	}, nil)
}

// DigestBatchDue reports whether a channel has accumulated a due batch: unsent
// deliveries exist and **the oldest one** has reached the digest period.
//
// The test is the age of the oldest delivery rather than wall-clock alignment, so a
// freshly created channel does not immediately emit a one-item "digest" just because
// it lined up with the hour, and a long backlog does not wait another round for
// nothing.
//
// It is separate from ClaimDigestBatch because the semantics differ: this function only
// answers "should we send", while claiming takes **every** unsent row of the channel
// (including those not old enough yet) — otherwise one period would be split into
// several messages and the digest would lose its point.
func (d *DB) DigestBatchDue(ctx context.Context, channelID int64, minAge time.Duration) (bool, error) {
	var due bool
	err := d.QueryRowContext(ctx, `SELECT EXISTS (
  SELECT 1 FROM notification_deliveries d
  JOIN notification_channels c ON c.id = d.channel_id
  WHERE d.channel_id = $1 AND d.state IN ($2,$3) AND c.enabled
  GROUP BY d.channel_id
  HAVING min(d.created_at) <= now() - make_interval(secs => $4)
)`, channelID, NotifyStatePending, NotifyStateSending, int64(minAge.Seconds())).Scan(&due)
	return due, err
}

// ClaimDigestBatch claims a channel's currently due unsent deliveries as one digest
// batch, at most MaxDigestBatchSize of them.
//
// Every delivery in the batch shares a batch_id, taken from the smallest id in the set
// (stable, readable, no extra sequence needed). On retry COALESCE keeps the original
// batch id, so "these N were sent together" still holds after several retries.
//
// The first N rows are taken in ascending id order rather than at random: the oldest
// delivery goes out first, so a backlog never starves with "new findings sent first,
// old ones queued forever".
func (d *DB) ClaimDigestBatch(ctx context.Context, channelID int64, limit int, lease time.Duration) ([]*NotificationDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	// limit is a **memory bound**; the caller passes MaxDigestBatchSize, and it is
	// clamped again here so a larger value cannot slip in.
	//
	// A rate-limit budget is deliberately not accepted as the batch size: the limiter
	// counts messages — one batch sends one message and spends one token, deducted by
	// takeTokens in the server layer — which is a different dimension from "how many
	// findings fit in one batch". This used to pass the per-round request budget in as
	// the batch size so that rate_per_min applied to digests, and a channel at
	// rate=20/min ended up putting one finding per batch, degrading the digest into a
	// realtime push with digest wording. To change rate limiting, change want in
	// takeTokens, not this.
	if limit > MaxDigestBatchSize {
		limit = MaxDigestBatchSize
	}
	out, err := d.claimDeliveries(ctx, lease, claimQuery{
		sql: `SELECT dd.id FROM notification_deliveries dd
JOIN notification_channels c ON c.id = dd.channel_id
WHERE dd.channel_id = $1 AND dd.state IN ($2,$3) AND dd.next_attempt_at <= now() AND c.enabled
ORDER BY dd.id
FOR UPDATE OF dd SKIP LOCKED
LIMIT $4`,
		args: []any{channelID, NotifyStatePending, NotifyStateSending, limit},
	}, func(tx *sql.Tx, ids []int64) error {
		batchID := ids[0]
		for _, id := range ids {
			if id < batchID {
				batchID = id
			}
		}
		ph, idArgs := placeholders(2, ids)
		_, err := tx.ExecContext(ctx, `UPDATE notification_deliveries SET batch_id = COALESCE(batch_id, $1)
WHERE id IN (`+ph+`)`, append([]any{batchID}, idArgs...)...)
		return err
	})
	return out, err
}

// claimDeliveries performs "select + set sending and extend the lease + read the full
// rows", all in one transaction. postClaim is an optional extra step (the digest batch
// uses it to write batch_id).
func (d *DB) claimDeliveries(ctx context.Context, lease time.Duration, cq claimQuery, postClaim func(*sql.Tx, []int64) error) ([]*NotificationDelivery, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit

	ids, err := selectForClaim(ctx, tx, cq.sql, cq.args...)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, tx.Commit()
	}
	// Set sending and push next_attempt_at into the future: that future instant is the
	// lease expiry, so "lease not expired" and "retry not due yet" share one condition
	// and no extra column is needed.
	ph, idArgs := placeholders(3, ids)
	if _, err := tx.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, attempts=attempts+1, next_attempt_at=now()+make_interval(secs => $2)
WHERE id IN (`+ph+`)`,
		append([]any{NotifyStateSending, lease.Seconds()}, idArgs...)...); err != nil {
		return nil, err
	}
	if postClaim != nil {
		if err := postClaim(tx, ids); err != nil {
			return nil, err
		}
	}
	out, err := loadDeliveriesTx(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

func selectForClaim(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func loadDeliveriesTx(ctx context.Context, tx *sql.Tx, ids []int64) ([]*NotificationDelivery, error) {
	ph, args := placeholders(1, ids)
	rows, err := tx.QueryContext(ctx, joinedDeliveryQuery+` WHERE d.id IN (`+ph+`) ORDER BY d.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*NotificationDelivery{}
	for rows.Next() {
		dl, err := scanNotificationDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dl)
	}
	return out, rows.Err()
}

// MarkDeliveriesSent marks a batch of deliveries as delivered.
func (d *DB) MarkDeliveriesSent(ctx context.Context, ids []int64) error {
	ph, args := placeholders(2, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, sent_at=now(), last_error='' WHERE id IN (`+ph+`)`, append([]any{NotifyStateSent}, args...)...)
	return err
}

// RescheduleDeliveries returns a batch of deliveries to pending and pushes the retry
// time back.
//
// Going back to pending instead of adding a new intermediate state keeps "how many
// chances are left" expressed in one place (MaxNotifyAttempts) and stops the state
// machine from growing a branch per retry policy.
func (d *DB) RescheduleDeliveries(ctx context.Context, ids []int64, delay time.Duration, errMsg string) error {
	ph, args := placeholders(4, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, next_attempt_at=now()+make_interval(secs => $2), last_error=$3
WHERE id IN (`+ph+`)`,
		append([]any{NotifyStatePending, delay.Seconds(), truncateNotifyError(errMsg)}, args...)...)
	return err
}

// DeferDeliveries returns a batch of deliveries to pending, immediately claimable
// again, and **undoes the attempt counted at claim time**.
//
// There is exactly one use: when a digest message is split to fit a channel's length
// limit, the items that did not fit must wait for the next batch. That is not a
// failure, so it must not spend retry budget — attempts was already optimistically
// incremented at claim time and has to be decremented here. Otherwise a backlog of 500
// would be cut into 25 segments of 20 and the tail items would be judged failed by
// MaxNotifyAttempts at the third segment even though nothing ever went wrong for them.
//
// GREATEST(...,0) covers the case where a manual resend zeroed attempts and then this
// path runs, keeping the counter from going negative.
func (d *DB) DeferDeliveries(ctx context.Context, ids []int64, reason string) error {
	ph, args := placeholders(3, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, attempts=GREATEST(attempts-1, 0), next_attempt_at=now(), last_error=$2
WHERE id IN (`+ph+`)`,
		append([]any{NotifyStatePending, truncateNotifyError(reason)}, args...)...)
	return err
}

// FailDeliveries marks a batch of deliveries as permanently failed, waiting for a
// human to resend them from the delivery history.
func (d *DB) FailDeliveries(ctx context.Context, ids []int64, errMsg string) error {
	// Placeholders start at $3: $1 is state, $2 is last_error.
	ph, args := placeholders(3, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries SET state=$1, last_error=$2 WHERE id IN (`+ph+`)`,
		append([]any{NotifyStateFailed, truncateNotifyError(errMsg)}, args...)...)
	return err
}

// RetryNotificationDelivery manually resends one delivery: back to pending, attempt
// counter reset, due immediately. Resetting the counter is deliberate — a human
// pressing "resend" means the cause of the earlier failures was dealt with, so holding
// the old count against it makes no sense.
func (d *DB) RetryNotificationDelivery(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$2, attempts=0, next_attempt_at=now(), last_error=''
WHERE id=$1 AND state IN ($3,$4)`, id, NotifyStatePending, NotifyStateFailed, NotifyStateSkipped)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("delivery %d does not exist or is not in a state that allows resending", id)
	}
	return nil
}

// NotificationDeliveryFilter holds the query conditions for the delivery history.
type NotificationDeliveryFilter struct {
	ChannelID int64
	State     string
	EventKind string
}

func (f NotificationDeliveryFilter) where() (string, []any) {
	var conds []string
	var args []any
	if f.ChannelID > 0 {
		args = append(args, f.ChannelID)
		conds = append(conds, fmt.Sprintf("d.channel_id=$%d", len(args)))
	}
	if f.State != "" {
		args = append(args, f.State)
		conds = append(conds, fmt.Sprintf("d.state=$%d", len(args)))
	}
	if f.EventKind != "" {
		args = append(args, f.EventKind)
		conds = append(conds, fmt.Sprintf("e.kind=$%d", len(args)))
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// ListNotificationDeliveries pages through the delivery history, newest first.
func (d *DB) ListNotificationDeliveries(ctx context.Context, f NotificationDeliveryFilter, page, pageSize int) ([]*NotificationDelivery, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	where, args := f.where()

	var total int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM notification_deliveries d
JOIN notification_events e ON e.id = d.event_id`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := fmt.Sprintf("%s%s ORDER BY d.id DESC LIMIT $%d OFFSET $%d",
		joinedDeliveryQuery, where, len(args)+1, len(args)+2)
	rows, err := d.QueryContext(ctx, q, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*NotificationDelivery{}
	for rows.Next() {
		dl, err := scanNotificationDelivery(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, dl)
	}
	return out, total, rows.Err()
}

// truncateNotifyError cuts an error message to a length the column accepts. A channel
// response body can be long (especially a generic webhook pointed at a self-hosted
// service) and would otherwise bloat the payload of the history list.
func truncateNotifyError(msg string) string {
	const max = 500
	if len(msg) <= max {
		return msg
	}
	// Back off to a character boundary so the UI never shows half a UTF-8 character.
	cut := max
	for cut > 0 && !isUTF8Start(msg[cut]) {
		cut--
	}
	return msg[:cut] + "…"
}

func isUTF8Start(b byte) bool { return b&0xC0 != 0x80 }

// placeholders builds the $n placeholder list starting at start, with matching args,
// for use in IN (...). For example start=3, ids=[7,8] -> "$3,$4", [7,8].
func placeholders(start int, ids []int64) (string, []any) {
	ph := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		ph = append(ph, fmt.Sprintf("$%d", start+i))
		args = append(args, id)
	}
	return strings.Join(ph, ","), args
}
