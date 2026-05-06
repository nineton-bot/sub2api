//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// makeOpenAITestContext 构造一个能让 GenerateSessionHash 返回非空 hash 的 gin.Context，
// 同时塞入带 GroupID 的 APIKey，让 getOpenAIGroupIDFromContext 能取到值。
//
// 之所以走 session_id header 而不是 body：避免触发底层 deriveOpenAIContentSessionSeed
// 的内容回退（行为更稳定，hash 与 sessionID 一一对应，便于断言）。
func makeOpenAITestContext(t *testing.T, sessionID string, groupID int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	if sessionID != "" {
		c.Request.Header.Set("session_id", sessionID)
	}
	if groupID > 0 {
		c.Set("api_key", &APIKey{ID: 1, GroupID: &groupID})
	}
	return c, w
}

// TestOpenAIHandleIncompleteStreamFailure_UnbindStickyOnTrueIncomplete 验证：
// OpenAI 路径上 "missing terminal event" 错误能正确解 sticky。
func TestOpenAIHandleIncompleteStreamFailure_UnbindStickyOnTrueIncomplete(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 98}
	c, _ := makeOpenAITestContext(t, "sess-incomplete-1", 42)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		[]byte(`{}`),
		true, // isStream
		fmt.Errorf("stream usage incomplete: missing terminal event"),
	)

	require.Len(t, cache.deleteCalls, 1, "true incomplete-stream should unbind sticky once")
	require.Equal(t, int64(42), cache.deleteCalls[0].groupID)
	require.True(t,
		strings.HasPrefix(cache.deleteCalls[0].sessionHash, "openai:"),
		"openai sticky key has 'openai:' prefix; got %q", cache.deleteCalls[0].sessionHash,
	)
}

// TestOpenAIHandleIncompleteStreamFailure_UnbindStickyOnUpstreamStreamEnded 验证：
// OpenAI buffered 路径上的 "upstream stream ended without terminal event" 错误
// 同样能解 sticky（这是和 Anthropic 路径不同的额外字符串）。
func TestOpenAIHandleIncompleteStreamFailure_UnbindStickyOnUpstreamStreamEnded(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 99}
	c, _ := makeOpenAITestContext(t, "sess-incomplete-2", 7)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		[]byte(`{}`),
		false, // isStream=false（buffered 路径，stream=false）
		fmt.Errorf("upstream stream ended without terminal event"),
	)

	require.Len(t, cache.deleteCalls, 1, "buffered upstream-truncation should also unbind sticky")
	require.Equal(t, int64(7), cache.deleteCalls[0].groupID)
}

// TestOpenAIHandleIncompleteStreamFailure_SkipOnClientDisconnect 验证：
// disconnect / timeout / context canceled 等客户端侧成因不应触发解 sticky。
func TestOpenAIHandleIncompleteStreamFailure_SkipOnClientDisconnect(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 98}
	c, _ := makeOpenAITestContext(t, "sess-skip-1", 42)

	skipCases := []error{
		fmt.Errorf("stream usage incomplete after disconnect: %w", context.Canceled),
		fmt.Errorf("stream usage incomplete: %w", context.Canceled),
		fmt.Errorf("stream usage incomplete after timeout"),
		// 关键：buffered 路径在 client cancel 时会包裹 ctx 错误，字符串虽匹配但
		// matcher 必须 reject，否则会误解 sticky（导致 prompt cache miss）+
		// 误触 599 policy（运维若开规则会拉黑健康账号）。
		fmt.Errorf("upstream stream ended without terminal event after client cancel: %w", context.Canceled),
		fmt.Errorf("upstream stream ended without terminal event after client cancel: %w", context.DeadlineExceeded),
	}
	for _, err := range skipCases {
		svc.handleIncompleteStreamFailure(context.Background(), c, account, []byte(`{}`), true, err)
	}
	require.Len(t, cache.deleteCalls, 0, "client-disconnect / timeout variants must NOT unbind sticky")
}

// TestOpenAIHandleIncompleteStreamFailure_NilGuards 验证 nil 守卫不 panic 也不解 sticky。
func TestOpenAIHandleIncompleteStreamFailure_NilGuards(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 1}
	matchErr := fmt.Errorf("stream usage incomplete: missing terminal event")
	c, _ := makeOpenAITestContext(t, "sess-nil-guard", 1)

	svc.handleIncompleteStreamFailure(context.Background(), c, nil, []byte(`{}`), true, matchErr)
	svc.handleIncompleteStreamFailure(context.Background(), c, account, []byte(`{}`), true, nil)
	// nil c 会让 sessionHash 为空（GenerateSessionHash 早 return），不解 sticky 但不 panic
	require.NotPanics(t, func() {
		svc.handleIncompleteStreamFailure(context.Background(), nil, account, []byte(`{}`), true, matchErr)
	})

	require.Len(t, cache.deleteCalls, 0, "nil account/err/c must not unbind")
}

// TestOpenAIHandleIncompleteStreamFailure_SkipNonMatchingError 验证：
// 不匹配的错误（500、429 等）一律不动作。
func TestOpenAIHandleIncompleteStreamFailure_SkipNonMatchingError(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 1}
	c, _ := makeOpenAITestContext(t, "sess-non-match", 1)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		[]byte(`{}`),
		true,
		errors.New("upstream 500 internal server error"),
	)
	require.Len(t, cache.deleteCalls, 0)
}

// TestOpenAIHandleIncompleteStreamFailure_EmitsErrorEventBeforeUnbind 验证：
// 流式请求触发后客户端收到 SSE error event，sticky 也被解绑。
func TestOpenAIHandleIncompleteStreamFailure_EmitsErrorEventBeforeUnbind(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 98}
	c, w := makeOpenAITestContext(t, "sess-emit-1", 42)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		[]byte(`{}`),
		true, // isStream
		fmt.Errorf("stream usage incomplete: missing terminal event"),
	)

	require.Contains(t, w.Body.String(), `"type":"error"`)
	require.Contains(t, w.Body.String(), "missing terminal event")
	require.Len(t, cache.deleteCalls, 1)
}

// TestOpenAIHandleIncompleteStreamFailure_NoSSEBytesWhenBuffered 验证：
// buffered（isStream=false）路径不能写 SSE 字节到客户端（避免污染 JSON 响应），
// 但 sticky 仍要解（buffered 路径下用户下次请求依然会被 sticky 钉死）。
func TestOpenAIHandleIncompleteStreamFailure_NoSSEBytesWhenBuffered(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 98}
	c, w := makeOpenAITestContext(t, "sess-buffered", 42)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		[]byte(`{}`),
		false, // isStream=false（buffered/JSON 路径）
		fmt.Errorf("upstream stream ended without terminal event"),
	)

	require.Empty(t, w.Body.String(), "buffered path must NOT receive SSE bytes")
	require.Len(t, cache.deleteCalls, 1, "buffered path must STILL unbind sticky")
}

// withStubOpenaiPolicyTrigger 临时替换 openaiIncompleteStreamPolicyTrigger，测试结束自动恢复。
func withStubOpenaiPolicyTrigger(t *testing.T, returnTriggered bool) *policyCallCapture {
	t.Helper()
	capture := &policyCallCapture{}
	orig := openaiIncompleteStreamPolicyTrigger
	openaiIncompleteStreamPolicyTrigger = func(_ *OpenAIGatewayService, _ context.Context, account *Account, code int, body []byte) bool {
		capture.called = true
		if account != nil {
			capture.accountID = account.ID
		}
		capture.code = code
		capture.body = string(body)
		return returnTriggered
	}
	t.Cleanup(func() {
		openaiIncompleteStreamPolicyTrigger = orig
	})
	return capture
}

// TestOpenAIHandleIncompleteStreamFailure_HandsErrorToPolicyTrigger 验证：
// 真实 incomplete-stream 错误必须传给 tryTempUnschedulable 评估器，参数正确。
func TestOpenAIHandleIncompleteStreamFailure_HandsErrorToPolicyTrigger(t *testing.T) {
	capture := withStubOpenaiPolicyTrigger(t, true)
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 98}
	c, _ := makeOpenAITestContext(t, "sess-policy-1", 42)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		[]byte(`{}`),
		true,
		fmt.Errorf("stream usage incomplete: missing terminal event"),
	)

	require.True(t, capture.called, "policy trigger must be invoked on true incomplete-stream")
	require.Equal(t, int64(98), capture.accountID, "must pass the upstream account being faulted")
	require.Equal(t, incompleteStreamSyntheticCode, capture.code, "must pass synthetic 599 so admin rules can match")
	require.Contains(t, capture.body, "missing terminal event", "body must carry the keyword admins match on")
}

// TestOpenAIHandleIncompleteStreamFailure_SkipsPolicyTriggerOnDisconnect 验证：
// 客户端断开 / 读超时不触发 policy trigger（避免误标坏账号）。
func TestOpenAIHandleIncompleteStreamFailure_SkipsPolicyTriggerOnDisconnect(t *testing.T) {
	capture := withStubOpenaiPolicyTrigger(t, false)
	cache := &stubIncompleteStreamCache{}
	svc := &OpenAIGatewayService{cache: cache}
	account := &Account{ID: 98}
	c, _ := makeOpenAITestContext(t, "sess-policy-skip", 42)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		[]byte(`{}`),
		true,
		fmt.Errorf("stream usage incomplete after disconnect: %w", context.Canceled),
	)

	require.False(t, capture.called, "client-disconnect variant must NOT hit policy trigger")
}
