package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// weComMarkdownLimit is the hard limit of the WeCom group robot's markdown content (bytes,
// not characters). It is the tightest of all six channels and the main reason TruncateBytes
// exists.
const weComMarkdownLimit = 4096

// weComChannel implements the WeCom group robot.
//
// Platform traits:
//   - Auth is only the key in the URL, with no signing support -- so the webhook URL itself
//     is the whole credential.
//   - markdown content is limited to 4096 **bytes** and an over-long message is rejected
//     outright (not truncated). A CJK character is 3 bytes, so only about a thousand
//     characters fit and the client must truncate.
//   - Rate limited to 20 messages/minute, also handled by client-side limiting.
type weComChannel struct{}

func (weComChannel) Kind() string { return KindWeCom }

func (weComChannel) DefaultRatePerMin() int { return 20 }

// WeCom has a single credential (the key in the URL) and no signing support -- the whole
// address is the credential, so there are no other fields to mask.
func (weComChannel) SecretKeys() []string { return []string{"webhook"} }

// WeCom has only the webhook field, which is both destination and credential, so there is no
// "credential left behind after changing the address".
func (weComChannel) DestinationKeys() []string { return []string{"webhook"} }

func (weComChannel) Validate(cfg map[string]any) error {
	hook := cfgString(cfg, "webhook")
	if hook == "" {
		return errors.New("Webhook 주소가 없습니다")
	}
	if err := validateHTTPURL(hook); err != nil {
		return fmt.Errorf("Webhook 주소가 올바르지 않습니다: %w", err)
	}
	return nil
}

func (c weComChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	// A digest batch can be long (50 items x one line each + prefix) and easily exceeds 4096
	// bytes. Truncation happens here instead of relying on a platform error: rejection loses
	// the whole batch, while truncation still delivers the first items.
	content, kept := markdownBody(m, weComMarkdownLimit)
	payload := map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]any{"content": content},
	}
	raw, err := doJSON(ctx, "POST", cfgString(cfg, "webhook"), nil, payload)
	if err != nil {
		return 0, err
	}
	var res struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("WeCom 응답을 해석하지 못했습니다: %w (%s)", err, snippet(raw))
	}
	if res.ErrCode != 0 {
		// 45009 means the API call limit was exceeded -- the platform's rate-limit window
		// rolls, so a retry after backoff does work, hence explicitly retryable. Reaching
		// this means the channel's rate_per_min is configured too aggressively; the retry is
		// only a safety net and the real fix is lowering that channel's rate limit.
		if res.ErrCode == 45009 {
			return 0, fmt.Errorf("WeCom 요청이 제한되었습니다 (%d): %s", res.ErrCode, res.ErrMsg)
		}
		// 93000 means an invalid webhook key -- permanent, a retry will not fix it.
		return 0, Permanent(fmt.Errorf("WeCom에서 오류가 발생했습니다 (%d): %s", res.ErrCode, res.ErrMsg))
	}
	return kept, nil
}
