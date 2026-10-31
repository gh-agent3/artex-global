package notify

import (
	"fmt"
	"strings"
)

// This file holds the message rendering shared by the "Markdown family" channels
// (DingTalk, WeCom). Feishu uses card JSON, Telegram uses HTML and email uses HTML,
// each rendered in its own adapter.

// maxAssetsShown is the maximum number of assets listed in a message. One finding can
// be anchored to dozens of assets; listing them all would blow up the message with no
// informational value -- nobody reads domains past the fourth one in an IM.
const maxAssetsShown = 3

// maxSummaryRunes is how many characters a summary is squeezed into. An IM message is
// a nudge to go read the details, not the report itself; the full content is on the
// platform.
const maxSummaryRunes = 120

// markdownReservedBytes is the budget reserved for the message header (digest line,
// severity distribution, possible truncation notice) and footer (platform link). When
// packing whole messages this is subtracted from the budget so header and footer are
// never cut -- once they are gone the reader cannot even tell which batch this is or
// how many items are still missing.
const markdownReservedBytes = 320

// markdownEscape escapes markdown metacharacters.
//
// Why it is mandatory: finding titles, summaries, vuln classes and asset display names
// all come from **untrusted sources** -- titles and summaries come from model output
// (and the model reads the target's responses), while asset URLs are full scanned URLs
// (with a target-controlled query string). Without escaping, a finding titled
//
//	Login SQL injection\n[Urgent: verify your account](http://attacker.tld)
//
// renders as a **clickable external link** in the security engineer's DingTalk or
// Feishu; and `![](http://attacker.tld/beacon)` is fetched by the client at render
// time, which announces "this finding has been read" and leaks the reader's IP. Even
// without malice, injected bold or quote blocks can push a critical finding below the
// fold.
//
// The escape set covers title/link/emphasis/list/quote/strikethrough -- characters that
// change structure or produce clickable elements. `\` must be handled first, otherwise
// the backslashes added below get escaped again.
func markdownEscape(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"`", "\\`",
		"*", `\*`,
		"_", `\_`,
		"[", `\[`,
		"]", `\]`,
		"(", `\(`,
		")", `\)`,
		"!", `\!`,
		"#", `\#`,
		">", `\>`,
		"|", `\|`,
		"~", `\~`,
	)
	return replacer.Replace(s)
}

// markdownText flattens untrusted text to one line and escapes it, for markdown
// bodies. Flattening is the other half of escaping: a newline alone can forge a new
// list item or quote block, and escaping characters does not stop that.
func markdownText(s string, maxRunes int) string {
	return markdownEscape(OneLine(s, maxRunes))
}

// markdownTitle returns the message title (the IM title bar / card title); its content
// is the **raw, unescaped text**.
//
// Escaping is deliberately not done here: this title is shared by four rendering
// contexts -- markdown bodies, Telegram HTML, Feishu card plain_text, and the generic
// webhook's JSON plus email subject. Each context escapes differently (markdown escapes
// inside HTML leave visible backslashes, inside JSON they corrupt data), so escaping
// belongs to each output path: see writeItem / feishuItemLines / telegramEscape. A
// markdown escape once lived in this shared function and produced visible `\(1\)`
// backslashes in Telegram messages.
func markdownTitle(m Message) string {
	if m.Batch {
		return fmt.Sprintf("취약점 요약 · 총 %d건", len(m.Items))
	}
	if len(m.Items) == 0 {
		return "취약점 알림"
	}
	it := m.Items[0]
	return fmt.Sprintf("[%s] %s", SeverityLabel(it.Severity), OneLine(it.Title(), 0))
}

// markdownBody renders the message body and returns it with the number of items
// **actually written**.
//
// kept is how many items this delivery really carries; the caller marks only the first
// kept items as delivered. Items left outside the channel's length limit must wait for
// the next batch instead of being marked successful along with the rest. That is
// exactly where "silent loss" comes from: the message was truncated but the delivery
// record says everything was delivered, leaving no way to see the second half never
// went out.
//
// maxBytes<=0 means no limit.
func markdownBody(m Message, maxBytes int) (string, int) {
	if !m.Batch {
		if len(m.Items) == 0 {
			return "", 0
		}
		var b strings.Builder
		writeItem(&b, m.Items[0], "", true)
		// A single message is sent even when oversized (the final truncation catches
		// it): partial information about one finding beats sending nothing.
		return TruncateBytes(b.String(), maxBytes), 1
	}

	footer := ""
	if m.HomeURL != "" {
		footer = fmt.Sprintf("\n[플랫폼에서 전체 보기](%s)\n", m.HomeURL)
	}
	kept := packItemCount(m.Items, maxBytes, markdownReservedBytes, footer, byteSize, func(it Item, idx int) string {
		var b strings.Builder
		writeItem(&b, it, fmt.Sprintf("%d. ", idx+1), false)
		return b.String()
	})

	items := m.Items[:kept]
	var b strings.Builder
	b.WriteString(markdownBatchIntro(m, items, len(m.Items)))
	for i, it := range items {
		writeItem(&b, it, fmt.Sprintf("%d. ", i+1), false)
	}
	b.WriteString(footer)
	return TruncateBytes(b.String(), maxBytes), kept
}

// markdownBatchIntro renders the start of a digest: time window, count and severity
// distribution. With those, a reader can tell whether the batch needs immediate
// attention without opening the platform.
//
// items are the entries **actually packed**, total is the number this batch should
// have. When they differ the message must say how many items continue in the next
// message -- otherwise the reader assumes the number in the header is everything, while
// the items that never went out do not exist anywhere on screen.
func markdownBatchIntro(m Message, items []Item, total int) string {
	var b strings.Builder
	if m.WindowMinutes > 0 {
		fmt.Fprintf(&b, "**최근 %d분간 신규 취약점 %d건**", m.WindowMinutes, total)
	} else {
		fmt.Fprintf(&b, "**신규 취약점 %d건**", total)
	}
	if extra := total - len(items); extra > 0 {
		fmt.Fprintf(&b, "(이 메시지에는 앞 %d건만 표시하며, 나머지 %d건은 다음 메시지에서 이어서 보냅니다)", len(items), extra)
	}
	// Show the distribution by severity so critical items are obvious at a glance.
	// Only the entries **actually included** are counted, so "critical 3" matches the
	// items countable below.
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Severity]++
	}
	var parts []string
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		if n := counts[sev]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", SeverityLabel(sev), n))
		}
	}
	if len(parts) > 0 {
		b.WriteString("\n" + strings.Join(parts, " · "))
	}
	b.WriteString("\n\n")
	return b.String()
}

// writeItem renders a single finding entry.
//
// prefix is the index in a digest list; single=true renders the full version (with
// summary and back-link), while a digest list renders one summary line -- otherwise a
// 50-item digest becomes a long document.
//
// All external content (title/class/assets/summary) goes through markdownText:
// flatten to one line plus escaping. The back-link is built from the admin-configured
// public_base_url, is not untrusted content and must stay clickable, so it is emitted
// as is.
func writeItem(b *strings.Builder, it Item, prefix string, single bool) {
	line := fmt.Sprintf("%s**%s · %s**", prefix, SeverityLabel(it.Severity), markdownText(it.Title(), 0))
	if !single {
		// Digest mode: one line, with assets and summary squeezed in after it.
		var extras []string
		if a := assetLine(it.Assets, maxAssetsShown); a != "" {
			extras = append(extras, markdownText(a, 0))
		}
		if it.Summary != "" {
			extras = append(extras, markdownText(it.Summary, 60))
		}
		if len(extras) > 0 {
			line += " — " + strings.Join(extras, " · ")
		}
		b.WriteString(line + "\n")
		return
	}
	b.WriteString(line + "\n")
	if it.IsStatusChange() {
		fmt.Fprintf(b, "**상태 변경**: %s → %s\n",
			markdownText(StatusLabel(it.FromStatus), 0), markdownText(StatusLabel(it.ToStatus), 0))
	}
	if it.VulnClass != "" && it.VulnClass != it.Title() {
		fmt.Fprintf(b, "**유형**: %s\n", markdownText(it.VulnClass, 0))
	}
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		fmt.Fprintf(b, "**자산**: %s\n", markdownText(a, 0))
	}
	if it.Summary != "" {
		if s := markdownText(it.Summary, maxSummaryRunes); s != "" {
			fmt.Fprintf(b, "**개요**: %s\n", s)
		}
	}
	if it.DetailURL != "" {
		fmt.Fprintf(b, "[상세 보기](%s)\n", it.DetailURL)
	}
}
