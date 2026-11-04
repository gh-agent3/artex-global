package notify

import (
	"strings"
	"unicode/utf8"
)

const ellipsis = "…"

// TruncateBytes truncates s to at most max bytes, keeping the result valid UTF-8 and
// never splitting a character.
//
// Why cut on character boundaries: the WeCom group robot's markdown has a hard limit of
// 4096 **bytes** (not characters), and one CJK character is 3 bytes. A raw byte slice
// would cut a character in half and produce invalid UTF-8 -- the platform either rejects
// the whole message or shows replacement boxes. So the budget position is walked back to
// the nearest rune start byte (utf8.RuneStart identifies continuation bytes 0b10xxxxxx).
//
// max<=0 means no limit. The ellipsis is appended after truncation, unless max is too
// small to hold it.
func TruncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	budget := max - len(ellipsis)
	suffix := ellipsis
	if budget < 0 {
		// max is shorter than the ellipsis: drop the ellipsis and cut plainly, so the
		// result does not exceed max after all.
		budget = max
		suffix = ""
	}
	cut := budget
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

// OneLine flattens multi-line text into one line: collapses all whitespace, then
// truncates by character count. Used for IM title lines -- summaries often contain
// newlines that would wreck the layout if inserted into a table or title.
// max<=0 means no length limit.
func OneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	return TruncateRunes(s, max)
}

// TruncateRunes truncates s to at most max characters (not bytes), appending an
// ellipsis when it overflows. max<=0 means no limit.
//
// The difference from TruncateBytes is the platform's unit: WeCom limits by bytes,
// Telegram by characters. Using the wrong unit does not error, it just cuts messages
// far shorter than expected (a CJK character is 3 bytes, so a 4096-byte cut leaves about
// 1365 characters), which is why both functions must exist and each channel picks one.
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + ellipsis
}

// TruncateHTML truncates an HTML fragment by character count and never leaves a
// half-written tag.
//
// Truncating HTML by characters can cut out fragments like `<a href="htt`; the platform
// parser either rejects the whole message or swallows the following body as an attribute
// value. So: truncate by characters first, then check whether the tail holds an unclosed
// `<` and back off to before it.
//
// Tags are not balanced (no closing `</b>` is added): Telegram's HTML parser closes
// unclosed tags itself, and doing it here would mean handling quotes in attributes,
// comments and self-closing tags -- complexity out of proportion to the benefit.
func TruncateHTML(s string, max int) string {
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	cut := TruncateRunes(s, max)
	// If the tail is a fragment starting with `<` (no `>` after the last `<`), back off
	// to before the `<`.
	if lt := strings.LastIndex(cut, "<"); lt >= 0 && !strings.Contains(cut[lt:], ">") {
		cut = cut[:lt]
	}
	// Same for a cut HTML entity (e.g. `&amp;` cut into `&amp`). A parser that only
	// understands entities may reject the **whole message** over a fragment -- and an
	// over-limit digest is common enough that losing the entire notification is not
	// worth it.
	if amp := strings.LastIndex(cut, "&"); amp >= 0 && !strings.Contains(cut[amp:], ";") {
		cut = cut[:amp]
	}
	return cut
}

// packItemCount computes how many items fit **whole** within the budget, for packing
// digest messages item by item.
//
// Why pack whole items instead of rendering everything and truncating: truncation makes
// the second half vanish while its delivery records are still marked delivered -- not
// visible in the message, not visible in the delivery history, and the finding is simply
// gone. Packing whole items leaves the ones that do not fit in the database for the next
// batch, and the kept the caller receives is how many this message really delivers.
//
// Parameters: maxSize<=0 means no limit; reserve is the budget for the message header and
// footer; size measures (each platform differs: WeCom/DingTalk count bytes, Telegram
// counts characters -- the wrong unit does not error, it just squeezes CJK messages far
// below the limit); render renders item idx into its actual text, since length varies by
// content and cannot be estimated.
//
// It returns at least 1 (while items remain). One extremely long item is still sent and
// left to the caller's final truncation, otherwise a single oversized finding would block
// the whole batch forever.
func packItemCount(items []Item, maxSize, reserve int, footer string, size func(string) int, render func(Item, int) string) int {
	if maxSize <= 0 {
		return len(items)
	}
	budget := maxSize - reserve - size(footer)
	if budget < 0 {
		budget = 0
	}
	used := 0
	for i, it := range items {
		used += size(render(it, i))
		if used > budget && i > 0 {
			return i
		}
	}
	return len(items)
}

// byteSize / runeSize are the two units packItemCount accepts; naming them keeps bare
// func(s string) int closures out of the call sites, where the unit would be hard to
// spot at a glance.
func byteSize(s string) int { return len(s) }
func runeSize(s string) int { return utf8.RuneCountInString(s) }

// assetLine renders the asset list as one display line; past limit entries it omits the
// rest and states the total. One finding can be anchored to dozens of assets and listing
// them all would blow up the message.
func assetLine(assets []string, limit int) string {
	if len(assets) == 0 {
		return ""
	}
	if limit <= 0 || len(assets) <= limit {
		return strings.Join(assets, ", ")
	}
	return strings.Join(assets[:limit], ", ") + " 등 " + itoa(len(assets)) + "개"
}

// itoa is a short alias for strconv.Itoa, used only when building display text, to avoid
// importing strconv everywhere.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
