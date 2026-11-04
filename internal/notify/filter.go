package notify

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Filter is the contract for the notification_channels.filter JSONB column: the
// filter conditions of one channel instance. Every field is optional and absent means
// "no filtering" -- which is exactly the fallback semantics for malformed config, see
// ParseFilter.
type Filter struct {
	// MinSeverity is the minimum severity threshold (low/medium/high/critical);
	// empty means no threshold.
	MinSeverity string `json:"min_severity"`
	// Empty TaskIDs / AssetIDs mean unrestricted; non-empty requires an intersection
	// with the event.
	TaskIDs  []int64 `json:"task_ids"`
	AssetIDs []int64 `json:"asset_ids"`
	// An empty VulnClassInclude accepts everything; non-empty requires vulnclass to
	// match any of the keywords. VulnClassExclude drops the event on any keyword match
	// (exclusion wins over inclusion). Matching is case-insensitive substring -- safer
	// than regex, since a bad pattern cannot silently disable a channel.
	VulnClassInclude []string `json:"vulnclass_include"`
	VulnClassExclude []string `json:"vulnclass_exclude"`
	// OnStatusChange decides whether the channel receives finding status-change
	// events (only meaningful in realtime mode).
	OnStatusChange bool `json:"on_status_change"`
}

// ParseFilter parses a channel's filter config.
//
// **It never returns an error.** That is deliberate: malformed filter config degrades
// to the zero Filter (= no filtering = match everything), because for a
// vulnerability notification system **one extra push is far better than silently
// dropping a critical finding**. Turning a parse failure into "push nothing" would
// give the user a channel that looks configured and delivers nothing -- the worst
// possible failure mode.
func ParseFilter(raw []byte) Filter {
	var f Filter
	if len(raw) == 0 {
		return f
	}
	// On a parse failure f stays zero, i.e. no filtering.
	_ = json.Unmarshal(raw, &f)
	return f
}

// ValidMinSeverity reports whether s is a valid severity threshold (empty means no
// threshold).
func ValidMinSeverity(s string) bool {
	if s == "" {
		return true
	}
	_, ok := severityRank[s]
	return ok
}

// Validate checks the **restricted-value** fields of a filter config; called when a
// channel is saved.
//
// Why this must be caught at write time: Match evaluates an unknown threshold as
// `rank >= 0`, which is always true -- so one typo in min_severity ("hgih") makes the
// filter **silently ineffective**, i.e. "push everything". That matches this package's
// "rather over-push than under-push" bias (nothing is missed), but the user then
// believes they are doing graded delivery while every finding floods the group, with
// no hint that the config is wrong. Such silent degradation belongs at the entry
// point.
//
// Note Validate is only used on the **write** path. The read path keeps ParseFilter's
// lenient semantics, so a bad value already stored does not make the channel
// unreadable.
func (f Filter) Validate() error {
	if !ValidMinSeverity(f.MinSeverity) {
		return fmt.Errorf("올바르지 않은 최소 심각도입니다: %q. low / medium / high / critical 중에서 선택하거나, 비워 두면 제한하지 않습니다", f.MinSeverity)
	}
	return nil
}

// Match decides whether an event should be delivered to a channel carrying this
// filter.
//
// **It never returns an error**, for the same reason as ParseFilter: any internal
// surprise counts as a match. Order: event kind -> severity threshold -> task/asset
// scope -> vulnclass keywords.
func Match(f Filter, s Snapshot) bool {
	// Status-change events reach only channels that opted in. The default is off,
	// because most users mean "push" as "a new finding was discovered", not a running
	// commentary on every status transition.
	if s.Kind == EventFindingStatusChanged && !f.OnStatusChange {
		return false
	}
	if !AtLeast(s.Severity, f.MinSeverity) {
		return false
	}
	if len(f.TaskIDs) > 0 && !slices.Contains(f.TaskIDs, s.TaskID) {
		return false
	}
	if len(f.AssetIDs) > 0 && !intersectsInt(f.AssetIDs, s.AssetIDs) {
		return false
	}
	// Exclusion wins: any excluded keyword drops the event, even if it also matched
	// the include list.
	if len(f.VulnClassExclude) > 0 && containsAnyFold(s.VulnClass, f.VulnClassExclude) {
		return false
	}
	if len(f.VulnClassInclude) > 0 && !containsAnyFold(s.VulnClass, f.VulnClassInclude) {
		return false
	}
	return true
}

func intersectsInt(a, b []int64) bool {
	// A linear scan is fine for small sets; both sides are "a few dozen hand-picked
	// IDs", so building a map costs more than it saves.
	for _, v := range b {
		if slices.Contains(a, v) {
			return true
		}
	}
	return false
}

// containsAnyFold reports whether s contains any of the keywords
// (case-insensitive).
func containsAnyFold(s string, keywords []string) bool {
	lower := strings.ToLower(s)
	for _, kw := range keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw != "" && strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
