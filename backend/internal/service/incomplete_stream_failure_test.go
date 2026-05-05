//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestCanary_MissingTerminalEventStringExists 守护字符串耦合：
// isUpstreamIncompleteStreamError 用 strings.Contains 匹配错误文案，
// 一旦 upstream 改了 gateway_service.go 里的 "missing terminal event" 措辞，
// 我们的判别会静默失效。让这个测试红，强制开发者来同步更新匹配条件。
func TestCanary_MissingTerminalEventStringExists(t *testing.T) {
	src, err := os.ReadFile("gateway_service.go")
	require.NoError(t, err)
	require.Contains(t, string(src), "missing terminal event",
		"upstream renamed the incomplete-stream error; update isUpstreamIncompleteStreamError to match")
}

// incompleteStreamDeleteCall 记录一次 DeleteSessionAccountID 的调用参数。
// 不复用其它测试文件里同名的辅助类型，避免跨文件耦合 / 编译时依赖。
type incompleteStreamDeleteCall struct {
	groupID     int64
	sessionHash string
}

// stubIncompleteStreamCache 仅记录 DeleteSessionAccountID 调用，其它方法不应被触达。
type stubIncompleteStreamCache struct {
	GatewayCache
	deleteCalls []incompleteStreamDeleteCall
}

func (c *stubIncompleteStreamCache) DeleteSessionAccountID(_ context.Context, groupID int64, sessionHash string) error {
	c.deleteCalls = append(c.deleteCalls, incompleteStreamDeleteCall{groupID: groupID, sessionHash: sessionHash})
	return nil
}

// validMetadataUserID 返回一个能让 GenerateSessionHash 解析成功的合法 user_id 字符串。
func validMetadataUserID() string {
	return "user_" + strings.Repeat("a", 64) + "_account__session_12345678-1234-1234-1234-123456789012"
}

// makeGinContextWithRecorder 构造一个能记录 SSE 写入的 gin.Context。
// 通过 httptest.ResponseRecorder 同时验证：
//  1. 写入的内容
//  2. http.Flusher 接口可用（gin.ResponseWriter 实现了 http.Flusher）
func makeGinContextWithRecorder(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestIsUpstreamIncompleteStreamError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"missing terminal event", fmt.Errorf("stream usage incomplete: missing terminal event"), true},
		{"after disconnect", fmt.Errorf("stream usage incomplete after disconnect: %w", context.Canceled), false},
		{"after timeout", fmt.Errorf("stream usage incomplete after timeout"), false},
		{"context canceled wrapped", fmt.Errorf("stream usage incomplete: %w", context.Canceled), false},
		{"unrelated error", errors.New("upstream 500"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isUpstreamIncompleteStreamError(tc.err))
		})
	}
}

func TestHandleIncompleteStreamFailure_UnbindStickyOnTrueIncomplete(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	groupID := int64(42)
	parsed := &ParsedRequest{
		GroupID:        &groupID,
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: validMetadataUserID(),
	}
	svc := &GatewayService{cache: cache}
	account := &Account{ID: 98}
	c, _ := makeGinContextWithRecorder(t)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		parsed,
		fmt.Errorf("stream usage incomplete: missing terminal event"),
	)

	require.Len(t, cache.deleteCalls, 1, "true incomplete-stream should unbind sticky once")
	require.Equal(t, groupID, cache.deleteCalls[0].groupID)
	require.NotEmpty(t, cache.deleteCalls[0].sessionHash, "session hash should be derived from parsed request")
}

func TestHandleIncompleteStreamFailure_SkipOnClientDisconnect(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	groupID := int64(42)
	parsed := &ParsedRequest{
		GroupID:        &groupID,
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: validMetadataUserID(),
	}
	svc := &GatewayService{cache: cache}
	account := &Account{ID: 98}
	c, _ := makeGinContextWithRecorder(t)

	skipCases := []error{
		fmt.Errorf("stream usage incomplete after disconnect: %w", context.Canceled),
		fmt.Errorf("stream usage incomplete: %w", context.Canceled),
		fmt.Errorf("stream usage incomplete after timeout"),
	}
	for _, err := range skipCases {
		svc.handleIncompleteStreamFailure(context.Background(), c, account, parsed, err)
	}
	require.Len(t, cache.deleteCalls, 0, "client-disconnect / timeout variants must NOT unbind sticky")
}

func TestHandleIncompleteStreamFailure_NilGuards(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	groupID := int64(1)
	parsed := &ParsedRequest{GroupID: &groupID, Model: "m"}
	svc := &GatewayService{cache: cache}
	account := &Account{ID: 1}
	matchErr := fmt.Errorf("stream usage incomplete: missing terminal event")
	c, _ := makeGinContextWithRecorder(t)

	// nil account / nil parsed / nil err / nil c 都不应 panic 也不应触发 cache 调用
	svc.handleIncompleteStreamFailure(context.Background(), c, nil, parsed, matchErr)
	svc.handleIncompleteStreamFailure(context.Background(), c, account, nil, matchErr)
	svc.handleIncompleteStreamFailure(context.Background(), c, account, parsed, nil)
	svc.handleIncompleteStreamFailure(context.Background(), nil, account, parsed, matchErr)

	require.Len(t, cache.deleteCalls, 0, "nil account/parsed/err must not unbind")
}

func TestHandleIncompleteStreamFailure_SkipNonMatchingError(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	groupID := int64(1)
	parsed := &ParsedRequest{GroupID: &groupID, Model: "m", MetadataUserID: "u"}
	svc := &GatewayService{cache: cache}
	account := &Account{ID: 1}
	c, _ := makeGinContextWithRecorder(t)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		parsed,
		errors.New("some unrelated upstream error"),
	)
	require.Len(t, cache.deleteCalls, 0)
}

// TestEmitIncompleteStreamErrorEvent_WritesSSEFormat 验证 emit 函数：
//   1. 写出的内容是 SSE error event 格式
//   2. 与 gateway_handler.go:handleStreamingAwareError 的 schema 一致（type/error/upstream_error）
func TestEmitIncompleteStreamErrorEvent_WritesSSEFormat(t *testing.T) {
	c, w := makeGinContextWithRecorder(t)
	parsed := &ParsedRequest{Stream: true}

	emitIncompleteStreamErrorEvent(c, parsed)

	body := w.Body.String()
	require.Contains(t, body, `data: `, "must be SSE format")
	require.Contains(t, body, `"type":"error"`, "must carry SSE error type")
	require.Contains(t, body, `"upstream_error"`, "must mark upstream as the cause")
	require.Contains(t, body, "missing terminal event", "must mention root cause for debuggability")
	require.True(t, strings.HasSuffix(body, "\n\n"), "SSE event must terminate with blank line")
}

// TestEmitIncompleteStreamErrorEvent_NilSafe 验证 nil c / nil parsed 不 panic。
func TestEmitIncompleteStreamErrorEvent_NilSafe(t *testing.T) {
	require.NotPanics(t, func() {
		emitIncompleteStreamErrorEvent(nil, nil)
		emitIncompleteStreamErrorEvent(nil, &ParsedRequest{Stream: true})
		c, _ := makeGinContextWithRecorder(t)
		emitIncompleteStreamErrorEvent(c, nil)
	})
}

// TestEmitIncompleteStreamErrorEvent_SkipsNonStreamRequest 防 SSE 污染：
// 非流式（JSON）请求不应被注入 SSE 字节，否则客户端拿到畸形 body。
func TestEmitIncompleteStreamErrorEvent_SkipsNonStreamRequest(t *testing.T) {
	c, w := makeGinContextWithRecorder(t)
	parsed := &ParsedRequest{Stream: false}

	emitIncompleteStreamErrorEvent(c, parsed)

	require.Empty(t, w.Body.String(), "non-stream request must NOT receive SSE bytes")
}

// TestHandleIncompleteStreamFailure_EmitsErrorEventBeforeUnbind 端到端验证：
// 真实 incomplete-stream 触发后，客户端能立刻收到 SSE error event，
// 同时 sticky 也被解绑。
func TestHandleIncompleteStreamFailure_EmitsErrorEventBeforeUnbind(t *testing.T) {
	cache := &stubIncompleteStreamCache{}
	groupID := int64(42)
	parsed := &ParsedRequest{
		GroupID:        &groupID,
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: validMetadataUserID(),
	}
	svc := &GatewayService{cache: cache}
	account := &Account{ID: 98}
	c, w := makeGinContextWithRecorder(t)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		parsed,
		fmt.Errorf("stream usage incomplete: missing terminal event"),
	)

	// 客户端必须收到 SSE error event
	require.Contains(t, w.Body.String(), `"type":"error"`)
	require.Contains(t, w.Body.String(), "missing terminal event")
	// sticky 必须被解绑
	require.Len(t, cache.deleteCalls, 1)
}

// policyCallCapture 记录 incompleteStreamPolicyTrigger 被调用时的参数。
type policyCallCapture struct {
	called    bool
	accountID int64
	code      int
	body      string
}

// withStubPolicyTrigger 临时把 incompleteStreamPolicyTrigger 替换成 stub，
// 测试结束自动恢复。返回的 capture 指针用来断言调用情况。
func withStubPolicyTrigger(t *testing.T, returnTriggered bool) *policyCallCapture {
	t.Helper()
	capture := &policyCallCapture{}
	orig := incompleteStreamPolicyTrigger
	incompleteStreamPolicyTrigger = func(_ *GatewayService, _ context.Context, account *Account, code int, body []byte) bool {
		capture.called = true
		if account != nil {
			capture.accountID = account.ID
		}
		capture.code = code
		capture.body = string(body)
		return returnTriggered
	}
	t.Cleanup(func() {
		incompleteStreamPolicyTrigger = orig
	})
	return capture
}

// TestHandleIncompleteStreamFailure_HandsErrorToPolicyTrigger 验证：
// 真正的 incomplete-stream 错误必须被转交给 tryTempUnschedulable 评估器，
// 且调用参数正确（合成码 599、错误消息含 "missing terminal event"）。
func TestHandleIncompleteStreamFailure_HandsErrorToPolicyTrigger(t *testing.T) {
	capture := withStubPolicyTrigger(t, true)
	cache := &stubIncompleteStreamCache{}
	groupID := int64(42)
	parsed := &ParsedRequest{
		GroupID:        &groupID,
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: validMetadataUserID(),
	}
	svc := &GatewayService{cache: cache}
	account := &Account{ID: 98}
	c, _ := makeGinContextWithRecorder(t)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		parsed,
		fmt.Errorf("stream usage incomplete: missing terminal event"),
	)

	require.True(t, capture.called, "policy trigger must be invoked on true incomplete-stream")
	require.Equal(t, int64(98), capture.accountID, "must pass the upstream account being faulted")
	require.Equal(t, incompleteStreamSyntheticCode, capture.code, "must pass synthetic 599 so admin rules can match")
	require.Contains(t, capture.body, "missing terminal event", "body must carry the keyword admins match on")
}

// TestHandleIncompleteStreamFailure_SkipsPolicyTriggerOnDisconnect 验证：
// 客户端断开 / 读超时变体不应触发账号级保护（避免误标坏账号）。
func TestHandleIncompleteStreamFailure_SkipsPolicyTriggerOnDisconnect(t *testing.T) {
	capture := withStubPolicyTrigger(t, false)
	cache := &stubIncompleteStreamCache{}
	groupID := int64(42)
	parsed := &ParsedRequest{
		GroupID:        &groupID,
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: validMetadataUserID(),
	}
	svc := &GatewayService{cache: cache}
	account := &Account{ID: 98}
	c, _ := makeGinContextWithRecorder(t)

	svc.handleIncompleteStreamFailure(
		context.Background(),
		c,
		account,
		parsed,
		fmt.Errorf("stream usage incomplete after disconnect: %w", context.Canceled),
	)

	require.False(t, capture.called, "client-disconnect variant must NOT hit policy trigger")
}
