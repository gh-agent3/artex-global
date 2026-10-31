package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// allowLocalTargets decides whether delivering messages to loopback / link-local
// addresses is allowed.
//
// Denied by default. Those ranges are not where IM bots or public mail servers live, and
// what they reach is very sensitive: the admin port of another service on the same host,
// and cloud metadata endpoints (169.254.169.254, which hands out instance credentials).
// The delivery address is configured by an admin, but an admin session hijacked through
// XSS/CSRF -- or a second person sharing the same JWT -- can change the config and read the
// response back: doJSON writes the first 200 bytes of a 4xx/5xx body into last_error, and
// the delivery-history endpoint echoes it, which is a half-blind read primitive.
//
// But a "local SMTP relay" (postfix on 127.0.0.1:25) is a common self-hosted mail setup, so
// a blanket block would get in the way. Hence an explicit escape hatch rather than a
// hardcoded allow: setting ARTEX_NOTIFY_ALLOW_LOCAL=1 allows it.
//
// Exported as AllowLocalTargetsEnv so tests can turn it on explicitly -- the tests in this
// package and in the server package use httptest fake receivers on 127.0.0.1 heavily, and
// without it they would all be stopped by the guard.
const AllowLocalTargetsEnv = "ARTEX_NOTIFY_ALLOW_LOCAL"

func allowLocalTargets() bool {
	v := strings.TrimSpace(os.Getenv(AllowLocalTargetsEnv))
	return v == "1" || strings.EqualFold(v, "true")
}

// isBlockedDialIP reports whether the target IP falls into a range that is not allowed
// by default.
//
// Only loopback, link-local (including cloud metadata 169.254.169.254), unspecified and
// multicast are refused. RFC1918 private ranges are **not** refused: a self-hosted
// Mattermost or SMTP relay on the intranet is a very common legitimate setup, and blocking
// them too would make the feature unusable in real environments. The tradeoff is
// deliberate -- the protection must stop the truly sensitive targets without killing
// normal deployments.
func isBlockedDialIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// IPv4-mapped IPv6 (::ffff:127.0.0.1) must be converted back to IPv4 before the check,
	// otherwise it bypasses it.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// blockInternalDial is the Control hook of the http.Transport dialer; it checks the
// target address **when the connection is established**.
//
// Why at dial time and not only when config is saved: this is the point that actually
// takes effect. It also covers the two ways a config check can be bypassed -- DNS
// rebinding (a public IP at check time, an internal one at connect time) and redirects
// (cross-host redirects are refused, but same-host ones can still point elsewhere).
func blockInternalDial(_, address string, _ syscall.RawConn) error {
	if allowLocalTargets() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("대상 주소를 해석할 수 없습니다: %q", host)
	}
	if isBlockedDialIP(ip) {
		return fmt.Errorf("로컬 또는 링크 로컬 주소 %s 로는 전송하지 않습니다 (로컬 서비스로 보내야 하면 %s=1 로 설정하세요)", ip, AllowLocalTargetsEnv)
	}
	return nil
}

// notifyTransport adds only a dial guard on top of the default Transport.
// Clone keeps all the default tuning (connection pool, HTTP/2, timeouts, proxy, etc.),
// so adding one check does not change any other behavior.
var notifyTransport = func() *http.Transport {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	clone := t.Clone()
	clone.DialContext = (&net.Dialer{Timeout: 10 * time.Second, Control: blockInternalDial}).DialContext
	return clone
}()

// httpClient is the client shared by all channel deliveries.
//
// It deliberately does **not** reuse the project's global egress proxy (GlobalProxy on the
// server side): that proxy carries pentest target traffic and is often an unstable tunnel,
// while notification availability must not be hostage to target-network jitter. IM pushes
// can go direct. The timeout is 15 seconds -- a peer slower than that is effectively broken.
//
// Cross-host redirects are refused: delivery addresses here are always "one fixed
// endpoint" and normally never redirect to another host, while these providers' credentials
// (DingTalk access_token, WeCom key, Telegram bot token) **are in the URL** -- following a
// cross-host redirect would hand them to the redirect target. Same-host redirects (e.g. a
// trailing slash added) are still allowed.
var httpClient = &http.Client{
	Timeout:   15 * time.Second,
	Transport: notifyTransport,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("리다이렉트가 너무 많습니다")
		}
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("다른 호스트로의 리다이렉트는 허용하지 않습니다 (%s → %s)", via[0].URL.Host, req.URL.Host)
		}
		return nil
	},
}

// respBodyLimit caps how much of the response body is read. A misbehaving peer may return
// huge content, while we only need the error code and a short error description for the
// delivery history.
const respBodyLimit = 8 << 10

// doJSON sends one request and returns the response body (length-capped).
//
// A nil payload sends an empty body (for GET or platforms that do not need a body).
// Key/value pairs in headers are appended as is, for generic webhook custom headers.
//
// Error classification is the core job here: network failures and 5xx/408/429 count as
// **retryable**, every other 4xx as **permanent** -- retrying a 403 just writes the same
// error to the log three times.
func doJSON(ctx context.Context, method, url string, headers map[string]string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			// A serialization failure is a local bug (wrong config field type) and
			// retrying will not help.
			return nil, Permanent(fmt.Errorf("요청 본문을 만들지 못했습니다: %w", err))
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		// Invalid URL -- usually a mistyped address, which is permanent.
		// err must not be passed through here either: url.Parse's error text contains the
		// full address.
		return nil, Permanent(fmt.Errorf("요청 주소가 올바르지 않습니다: %s", redactRequestTarget(url)))
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Connection refused, DNS failure, timeout -- mostly transient, left to backoff
		// retries.
		//
		// The error text must be redacted before it leaves this function. Reason:
		// http.Client.Do returns *url.Error, whose Error() is `Op "full URL": underlying
		// error`, and these providers' credentials **are in the URL** (DingTalk
		// access_token, WeCom key, Feishu hook id, Telegram /bot<token>/). Unredacted, the
		// credentials would flow into four places: notification_deliveries.last_error
		// (stored in plaintext), the delivery-history API response (**bypassing the
		// channel config masking**), server logs, and the 502 text the test-send endpoint
		// returns to the frontend.
		return nil, fmt.Errorf("요청에 실패했습니다: %s", redactTransportError(err))
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, respBodyLimit))
	if readErr != nil {
		return nil, fmt.Errorf("응답을 읽지 못했습니다: %w", readErr)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, nil
	}
	// 429 (rate limited) and 408 (timeout) are worth retrying; the other 4xx are config
	// or permission problems where a retry is pointless.
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout {
		return nil, fmt.Errorf("상대 서버가 요청을 제한하거나 응답이 지연되었습니다 (HTTP %d): %s", resp.StatusCode, snippet(raw))
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("상대 서버에 오류가 발생했습니다 (HTTP %d): %s", resp.StatusCode, snippet(raw))
	}
	return nil, Permanent(fmt.Errorf("상대 서버가 요청을 거부했습니다 (HTTP %d): %s", resp.StatusCode, snippet(raw)))
}

// snippet squeezes the response body into one short line for error messages. Responses can
// carry newlines and lots of whitespace; dropping them straight into last_error would wreck
// the delivery history layout.
func snippet(raw []byte) string {
	return OneLine(string(raw), 200)
}

// redactRequestTarget squeezes a delivery address into "scheme://host/…" for error
// messages.
//
// This is the package's only address-redaction rule and it is deliberately **crude**:
// everything but scheme and host is dropped. There is no "generic and safe" way to tell
// which URL segment is the credential:
//
//	DingTalk credential in query      /robot/send?access_token=xxx
//	WeCom    credential in query      /cgi-bin/webhook/send?key=xxx
//	Feishu   credential in **last path segment** /open-apis/bot/v2/hook/<hook_id>
//	Telegram credential in **middle path segment** /bot<token>/sendMessage
//
// Keeping "just the useful part" would mean patching per channel, and missing any one of
// them is a credential leak. Keeping the host is enough for diagnosis (unresolvable DNS,
// connection refused, bad certificate are all locatable), and the masked tail of the
// channel config identifies which bot it was.
//
// On a parse failure a fixed placeholder is returned -- the raw string is never echoed.
func redactRequestTarget(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(주소를 해석할 수 없음)"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

// redactTransportError strips the address from a transport error, keeping only the
// underlying cause.
//
// *url.Error is {Op, URL, Err} and its Error() prints the URL too. Taking the Err field
// explicitly bypasses its Error() -- more reliable than post-hoc string replacement, which
// would have to handle every URL-encoded/escaped variant and easily misses one.
func redactTransportError(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		host := ""
		if u, parseErr := url.Parse(uerr.URL); parseErr == nil {
			host = u.Host
		}
		if uerr.Err != nil {
			return fmt.Sprintf("%s %s: %s", uerr.Op, host, uerr.Err)
		}
		return fmt.Sprintf("%s %s: 알 수 없는 오류", uerr.Op, host)
	}
	// Not a *url.Error (e.g. an error from the redirect policy) can carry an address too,
	// so everything goes through the same redaction.
	return redactURLsInText(err.Error())
}

// redactURLsInText replaces http(s) addresses found in a text with their redacted form.
//
// It backstops errors without structured fields (redirect policy errors, third-party
// custom errors). Only http/https prefixes are recognized, split on whitespace and
// quotes -- addresses cannot contain either.
func redactURLsInText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		rest := s[i:]
		if strings.HasPrefix(rest, "http://") || strings.HasPrefix(rest, "https://") {
			end := len(rest)
			if j := strings.IndexAny(rest, " \t\n\"'"); j >= 0 {
				end = j
			}
			b.WriteString(redactRequestTarget(rest[:end]))
			i += end
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
