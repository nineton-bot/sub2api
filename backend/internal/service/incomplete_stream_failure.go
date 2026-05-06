package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// incompleteStreamCleanupTimeout 为 sticky 解绑/账号健康标记的兜底超时。
// 触发本函数时客户端 ctx 多半已被 idle timeout 取消，必须用独立 ctx 才能完成清理。
const incompleteStreamCleanupTimeout = 5 * time.Second

// incompleteStreamSyntheticCode 是把"上游 SSE 流截断"伪装成 HTTP 错误码的合成值，
// 用于喂给 RateLimitService.tryTempUnschedulable 的规则匹配引擎。
// 599 是非标 RFC 状态码（业界惯例 "Network Connect Timeout Error"），主流上游不会用，
// 与真实上游错误码无冲突。运维在「临时不可调度」UI 里配规则时填这个值。
const incompleteStreamSyntheticCode = 599

// incompleteStreamErrorEvent 是当我们检测到上游截断时，主动写给客户端的 SSE 错误事件。
// schema 与 gateway_handler.go:handleStreamingAwareError 保持一致，避免 SDK 兼容问题。
const incompleteStreamErrorEvent = `data: {"type":"error","error":{"type":"upstream_error",` +
	`"message":"upstream stream truncated (missing terminal event); please retry"}}` + "\n\n"

// incompleteStreamPolicyTrigger 把"对账号触发临时不可调度规则评估"包成函数变量，
// 让测试可以在不构造完整 *RateLimitService 的前提下断言调用参数（code/body）。
// 生产路径直调 RateLimitService.tryTempUnschedulable；测试可临时覆盖为 stub。
var incompleteStreamPolicyTrigger = func(s *GatewayService, ctx context.Context, account *Account, code int, body []byte) bool {
	if s == nil || s.rateLimitService == nil {
		return false
	}
	return s.rateLimitService.tryTempUnschedulable(ctx, account, code, body)
}

// handleIncompleteStreamFailure 解决"用户被坏渠道钉死、不能自动回落"的问题。
//
// 现象（生产事故）：上游返回 HTTP 200 + 截断 SSE 流（缺 message_stop / terminal
// event）时，原代码默默关闭响应、客户端 SDK 等到自身 idle timeout 才报错；
// 同时 sticky session 把用户钉在这个坏账号上长达 1 小时（sticky TTL）。
// 个别头部用户连续重试都命中同一个坏账号 → 表现为"消息发了 15 分钟没回复"。
//
// 修复路径（按重要性，做的事情都是为了让用户成功回落到其他渠道）：
//
//  1. 主动给客户端发 SSE error event（emitIncompleteStreamErrorEvent）
//     → 让 SDK 立刻知道失败，不再苦等自身 idle timeout
//     → 用户/SDK 可以立即触发重试
//
//  2. 解除当前用户的 sticky 绑定（DeleteSessionAccountID）
//     → 让用户下一次请求重新调度到健康账号
//     → 这是核心的"自动回落"机制
//
//  3. （可选）把这个故障作为合成 HTTP 599 错误喂给 tryTempUnschedulable
//     → 如果运维在该账号配了匹配规则，会触发账号级临时不可调度
//     → 默认无效果，建议仅给"持续坏" (>30% 错误率) 的渠道配；偶发抖动 (<5%)
//       的渠道不要配，让 1+2 自然处理，给渠道自我恢复机会
//
// 注意：当前请求本身无法重试——响应已开始流式写入客户端（gateway_handler 在
// 写过任何字节后显式禁用 failover，避免流拼接腐化）。所以本函数的所有动作都
// 仅作用于"下次请求"的路由，目标是让该用户的下一次请求成功落到健康账号。
//
// 仅匹配 "missing terminal event" 这一上游确定性截断错误；客户端断开 / 读超时
// 等其他 incomplete-stream 变体不算上游故障，跳过（避免误伤）。
func (s *GatewayService) handleIncompleteStreamFailure(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *ParsedRequest,
	streamErr error,
) {
	if s == nil || account == nil || streamErr == nil || parsed == nil {
		return
	}
	if !isUpstreamIncompleteStreamError(streamErr) {
		return
	}

	// 0) 让客户端立刻知道失败 → 触发 SDK 立即重试到下一次请求（核心目标）。
	//    必须最先做：客户端连接随时可能被 SDK idle-timeout 关掉。
	//    如果不做这步，用户要等 SDK 自身 idle timeout（通常 1-3 分钟）才知道。
	emitIncompleteStreamErrorEvent(c, parsed.Stream)

	// 调用方 ctx 在 incomplete-stream 触发时通常已被取消（客户端 idle timeout
	// 是这类故障的典型触发器）。必须切到独立 ctx，否则 cleanup 会立即失败、
	// sticky 没解到，下次请求继续粘在坏账号。
	cleanupCtx, cancel := context.WithTimeout(context.Background(), incompleteStreamCleanupTimeout)
	defer cancel()

	// 1) 解 sticky：让该用户的下一次请求**自动回落**到其他健康账号（核心目标）。
	//    如果不做这步，sticky 会把用户继续路由到这个坏账号，重试也是白搭。
	//    这是修复"个别头部用户连续 15 分钟没回复"问题的关键。
	if s.cache != nil {
		sessionHash := s.GenerateSessionHash(parsed)
		if sessionHash != "" {
			groupID := derefGroupID(parsed.GroupID)
			if delErr := s.cache.DeleteSessionAccountID(cleanupCtx, groupID, sessionHash); delErr != nil {
				logger.LegacyPrintf("service.gateway",
					"incomplete_stream_unbind_failed account=%d group=%d session=%s err=%v",
					account.ID, groupID, shortSessionHash(sessionHash), delErr)
			} else {
				logger.LegacyPrintf("service.gateway",
					"incomplete_stream_sticky_unbound account=%d group=%d session=%s model=%s",
					account.ID, groupID, shortSessionHash(sessionHash), parsed.Model)
			}
		}
	}

	// 2) 可选保险：把故障作为合成 HTTP 599 喂给 temp_unschedulable_rules 评估器。
	//    这是给"持续坏" 渠道的兜底。**默认无效果**（运维未配规则时）；
	//    建议仅给真正需要保护其他用户的渠道配规则（如近 1h 错误率 > 30%）。
	//    偶发抖动 (<5%) 的渠道**不要配**——让 0+1 自然处理，给渠道恢复机会。
	//    通过 incompleteStreamPolicyTrigger 函数变量做测试 seam；生产路径直调
	//    底层 tryTempUnschedulable，绕过 CheckErrorPolicy 的 custom_error_codes
	//    / pool_mode gate（让 pool_mode 账号也能被规则保护）。
	triggered := incompleteStreamPolicyTrigger(
		s,
		cleanupCtx,
		account,
		incompleteStreamSyntheticCode,
		[]byte(streamErr.Error()),
	)
	if triggered {
		logger.LegacyPrintf("service.gateway",
			"incomplete_stream_handed_to_policy account=%d code=%d outcome=temp_unscheduled",
			account.ID, incompleteStreamSyntheticCode)
	}
}

// emitIncompleteStreamErrorEvent 主动给客户端发一个 SSE error event，
// 避免 SDK 等到自身 idle timeout 才发现失败。
//
// 仅在 isStream=true 时写入：避免给非流式（JSON）响应注入 SSE 字节、
// 产生畸形 body。即便 isUpstreamIncompleteStreamError 上游只在流式路径产生，
// 这里也按构造防御一次，杜绝跨包数据流耦合带来的潜在污染。
//
// 调用方传 isStream 而不是 *ParsedRequest，是为了让 Anthropic / OpenAI
// 两个 gateway 复用同一份写入逻辑（OpenAI 没有 ParsedRequest 类型）。
//
// 客户端连接 reset / nil context / 不支持 flusher 时静默忽略，绝不 panic。
func emitIncompleteStreamErrorEvent(c *gin.Context, isStream bool) {
	if c == nil || c.Writer == nil {
		return
	}
	if !isStream {
		return
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return
	}
	if _, err := fmt.Fprint(c.Writer, incompleteStreamErrorEvent); err != nil {
		// 客户端可能已 reset，silent ignore。
		return
	}
	flusher.Flush()
}

// isUpstreamIncompleteStreamError 仅匹配上游确定性截断（HTTP 200 但无 terminal event）。
//
// 双层判别：
//
//  1. 早 reject：任何包裹了 context.Canceled / context.DeadlineExceeded 的错误一律
//     不算上游故障。这是为了应对 OpenAI buffered 路径的字符串歧义——同一句
//     "upstream stream ended without terminal event" 既可能是上游真截断、也可能是
//     客户端取消导致 scanner 提前 EOF。openai_gateway_chat_completions.go /
//     openai_gateway_messages.go 在 client cancel 场景下已被改造成
//     fmt.Errorf("...: %w", scanErr) 包裹 ctx 错误，让此处 errors.Is 能识别。
//
//  2. 字符串匹配两条上游侧成因：
//      - "missing terminal event" — Anthropic gateway (gateway_service.go:5352, :7388)
//        与 OpenAI streaming 路径 (openai_gateway_service.go:3511, :4166)
//      - "upstream stream ended without terminal event" — OpenAI buffered 路径
//        (openai_gateway_chat_completions.go:408, openai_gateway_messages.go:358)
//
// 故意不匹配以下变体（它们由客户端断开或读超时导致，不是上游故障）：
//   - "stream usage incomplete after disconnect: ..."
//   - "stream usage incomplete after timeout"
//   - "stream usage incomplete: context canceled"（其本身已被 errors.Is 早 reject）
func isUpstreamIncompleteStreamError(err error) bool {
	if err == nil {
		return false
	}
	// 客户端侧成因 → 不动作。防止误判用户取消为上游故障，避免错解 sticky / 错触
	// 599 policy（运维若开了 599 临时不可调度规则，误判会拉黑健康账号、影响所有用户）。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "missing terminal event") ||
		strings.Contains(msg, "upstream stream ended without terminal event")
}
