package notify

// Snapshot is the contract for the notification_events.snapshot JSONB column. The
// writer is the finding-persistence transaction in the db layer; the readers are the
// delivery engine and the filter matching in the server layer. It lives in this
// package because it is the notification domain payload: db only serializes it and
// does not interpret the fields.
//
// Why the finding fields are stored redundantly instead of re-read at render time:
// findings get renamed, re-rated and re-statused later, but a push should reflect the
// conclusion **as of the event** -- re-reading would yield the dangerous lie of
// "later changed to low". It also lets fan-out and rendering skip JOINing
// findings/tasks/assets.
type Snapshot struct {
	// Event kind: finding_created / finding_status_changed
	Kind      string  `json:"kind"`
	FindingID int64   `json:"finding_id"`
	TaskID    int64   `json:"task_id"`
	VulnClass string  `json:"vulnclass"`
	Name      string  `json:"name"`
	Severity  string  `json:"severity"`
	Summary   string  `json:"summary"`
	AssetIDs  []int64 `json:"asset_ids"`
	// Non-empty only when kind=finding_status_changed.
	FromStatus string `json:"from_status,omitempty"`
	ToStatus   string `json:"to_status,omitempty"`
}

// Item is one finding to push, for channel rendering.
type Item struct {
	FindingID int64
	Name      string
	VulnClass string
	Severity  string
	Summary   string
	// Assets are the resolved display names (e.g. domain/IP). Filled by the server
	// layer -- this package never touches the database and cannot resolve them.
	Assets []string
	// DetailURL is the link back to the finding; empty means public_base_url is not
	// configured and the render omits it.
	DetailURL string
	// Only for status-change events; when both are set they render as
	// "pending -> fixed".
	FromStatus string
	ToStatus   string
}

// IsStatusChange reports whether the item is a status-change event.
func (i Item) IsStatusChange() bool { return i.FromStatus != "" || i.ToStatus != "" }

// Title returns the item's display title: the human-given name first, then the
// vulnclass, and a placeholder when both are empty -- never an empty title.
func (i Item) Title() string {
	if i.Name != "" {
		return i.Name
	}
	if i.VulnClass != "" {
		return i.VulnClass
	}
	return "(이름 없는 취약점)"
}

// Message is the complete content of one channel send.
type Message struct {
	// Length 1 for a single push; the whole batch for a digest push.
	// An empty slice is invalid: the caller must provide at least one item.
	Items []Item
	// Batch=true renders a digest (different title, with time window and counts).
	Batch bool
	// WindowMinutes is the digest period in minutes; only used when Batch=true, for
	// the "last N minutes" wording. Passed in explicitly instead of computing
	// time.Since at render time, which keeps rendering deterministic and testable.
	WindowMinutes int
	// HomeURL is the platform panel URL (global public_base_url); empty means no
	// panel entry is included.
	HomeURL string
}
