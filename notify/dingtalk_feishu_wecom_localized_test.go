package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// asciiStatusChangeItem builds a status-change item without any Han characters (pure ASCII).
// If a label regresses to Chinese, Han characters appear in the rendered output, so the
// "zero Han in the whole output" assertion only catches label regressions while the data has
// zero Han.
func asciiStatusChangeItem() Item {
	return Item{
		FindingID:  7,
		Name:       "login-flaw",
		VulnClass:  "SQLi",
		Severity:   "high",
		Summary:    "SQL injection via q param",
		Assets:     []string{"a.example.com"},
		DetailURL:  "https://platform.example/finding/7",
		FromStatus: "pending",
		ToStatus:   "fixed",
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("카드 직렬화 실패: %v", err)
	}
	return string(b)
}

// TestFeishuItemLinesLocalized checks that the field labels of a single Feishu card body
// (status change, type, assets, summary) are Korean and character-identical to markdown.go
// (F4②). feishu.go keeps its own copy of these labels (it does not share the markdown
// 3-channel function), so this pin must exist separately from the shared-function regression
// test to catch a regression where only the Feishu body reverts to Chinese.
func TestFeishuItemLinesLocalized(t *testing.T) {
	got := feishuItemLines(asciiStatusChangeItem())
	assertKorean(t, "feishuItemLines", got)
	for _, want := range []string{
		"**상태 변경**: 처리 대기 → 수정됨",
		"**유형**: SQLi",
		"**자산**: a.example.com",
		"**개요**: SQL injection via q param",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the single Feishu body must contain %q:\n%s", want, got)
		}
	}
}

// TestFeishuCardButtonsLocalized checks that the Feishu card button labels (view details,
// view everything on the platform) are Korean and that the whole card has no Chinese Han
// characters (with ASCII data).
func TestFeishuCardButtonsLocalized(t *testing.T) {
	// Single: the "상세 보기" button (when DetailURL is set).
	single, _ := feishuCard(Message{Items: []Item{asciiStatusChangeItem()}})
	singleJSON := mustJSON(t, single)
	if hasHan(singleJSON) {
		t.Errorf("the single Feishu card still contains Chinese Han characters:\n%s", singleJSON)
	}
	if !strings.Contains(singleJSON, "상세 보기") {
		t.Errorf("the single Feishu card must have a '상세 보기' button:\n%s", singleJSON)
	}

	// Digest: the "플랫폼에서 전체 보기" button (when HomeURL is set).
	batch, _ := feishuCard(Message{Batch: true, HomeURL: "https://platform.example", Items: hanFreeItems(2)})
	batchJSON := mustJSON(t, batch)
	if hasHan(batchJSON) {
		t.Errorf("the digest Feishu card still contains Chinese Han characters:\n%s", batchJSON)
	}
	if !strings.Contains(batchJSON, "플랫폼에서 전체 보기") {
		t.Errorf("the digest Feishu card must have a '플랫폼에서 전체 보기' button:\n%s", batchJSON)
	}
}

// TestDingTalkActionCardButtonLocalized checks through the real Send path (fake receiver) that
// the DingTalk ActionCard "상세 보기" button (singleTitle) is Korean.
func TestDingTalkActionCardButtonLocalized(t *testing.T) {
	var singleTitle string
	srv := capturePost(t, `{"errcode":0,"errmsg":"ok"}`, func(_ *testing.T, body map[string]any, _ *http.Request) {
		card, _ := body["actionCard"].(map[string]any)
		singleTitle, _ = card["singleTitle"].(string)
	})
	m := Message{Items: []Item{asciiStatusChangeItem()}}
	if _, err := (dingTalkChannel{}).Send(context.Background(), map[string]any{"webhook": srv.URL}, m); err != nil {
		t.Fatalf("delivery failed: %v", err)
	}
	if singleTitle != "상세 보기" {
		t.Errorf("the DingTalk ActionCard singleTitle must be '상세 보기', got %q", singleTitle)
	}
}

// TestChinaPlatformValidateLocalized checks that the config validation errors of DingTalk,
// Feishu and WeCom are Korean and keep the "Webhook" wording (the notification settings save
// and test API expose them to the user).
func TestChinaPlatformValidateLocalized(t *testing.T) {
	for _, kind := range []string{KindDingTalk, KindFeishu, KindWeCom} {
		ch, ok := Get(kind)
		if !ok {
			t.Fatalf("%s 채널이 등록되어 있지 않습니다", kind)
		}

		// Empty config: "Webhook 주소가 없습니다"
		missing := ch.Validate(map[string]any{})
		if missing == nil {
			t.Fatalf("%s: an empty config must fail validation", kind)
		}
		assertKorean(t, kind+" missing", missing.Error())
		if !strings.Contains(missing.Error(), "Webhook") || !strings.Contains(missing.Error(), "없습니다") {
			t.Errorf("%s: the missing-field error must be 'Webhook 주소가 없습니다', got %q", kind, missing)
		}

		// Bad address (ftp): "Webhook 주소가 올바르지 않습니다: …"
		bad := ch.Validate(map[string]any{"webhook": "ftp://x"})
		if bad == nil {
			t.Fatalf("%s: an ftp address must fail validation", kind)
		}
		if hasHan(bad.Error()) {
			t.Errorf("%s: the bad-address error still contains Chinese Han characters: %q", kind, bad)
		}
		if !strings.Contains(bad.Error(), "Webhook 주소가 올바르지 않습니다") {
			t.Errorf("%s: the bad-address error must start with 'Webhook 주소가 올바르지 않습니다', got %q", kind, bad)
		}
	}
}

// TestChinaPlatformSendErrorsLocalized checks that DingTalk, Feishu and WeCom delivery failure
// reasons land in the delivery history (last_error) in Korean. Platform names match the UI
// channel labels (DingTalk/Feishu/WeCom). Server errmsg values are ASCII, so any Han character
// in the error means the scaffolding wording regressed to Chinese.
func TestChinaPlatformSendErrorsLocalized(t *testing.T) {
	// Business error code path (HTTP 200 with errcode/code != 0 in the body).
	bizCases := []struct {
		kind     string
		resp     string
		wantSubs []string
	}{
		{KindDingTalk, `{"errcode":310000,"errmsg":"keyword not matched"}`, []string{"DingTalk에서 오류가 발생했습니다", "310000"}},
		{KindFeishu, `{"code":19021,"msg":"sign error"}`, []string{"Feishu에서 오류가 발생했습니다", "19021"}},
		{KindWeCom, `{"errcode":45009,"errmsg":"freq out of limit"}`, []string{"WeCom 요청이 제한되었습니다", "45009"}},
		{KindWeCom, `{"errcode":93000,"errmsg":"invalid webhook"}`, []string{"WeCom에서 오류가 발생했습니다", "93000"}},
	}
	for _, tc := range bizCases {
		srv := capturePost(t, tc.resp, nil)
		ch, ok := Get(tc.kind)
		if !ok {
			t.Fatalf("channel %s is not registered", tc.kind)
		}
		_, err := ch.Send(context.Background(), map[string]any{"webhook": srv.URL}, singleMsg())
		if err == nil {
			t.Fatalf("%s: the response %s must be an error", tc.kind, tc.resp)
		}
		if hasHan(err.Error()) {
			t.Errorf("%s: the delivery error still contains Chinese Han characters: %q", tc.kind, err)
		}
		for _, sub := range tc.wantSubs {
			if !strings.Contains(err.Error(), sub) {
				t.Errorf("%s: the delivery error must contain %q, got %q", tc.kind, sub, err)
			}
		}
	}

	// Parse failure path when the response is not JSON: "<platform> 응답을 해석하지 못했습니다".
	parseCases := []struct {
		kind     string
		platform string
	}{
		{KindDingTalk, "DingTalk"},
		{KindFeishu, "Feishu"},
		{KindWeCom, "WeCom"},
	}
	for _, pc := range parseCases {
		srv := capturePost(t, `not-json`, nil)
		ch, ok := Get(pc.kind)
		if !ok {
			t.Fatalf("channel %s is not registered", pc.kind)
		}
		_, err := ch.Send(context.Background(), map[string]any{"webhook": srv.URL}, singleMsg())
		if err == nil {
			t.Fatalf("%s: a non-JSON response must be an error", pc.kind)
		}
		want := pc.platform + " 응답을 해석하지 못했습니다"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%s: the parse failure error must contain %q, got %q", pc.kind, want, err)
		}
	}
}
