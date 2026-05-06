package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// openaiIncompleteStreamPolicyTrigger 与 incompleteStreamPolicyTrigger（Anthropic 版）
// 一一对应，仅签名上接收 *OpenAIGatewayService。同样作为函数变量做测试 seam。
//
// 生产路径直调 RateLimitService.tryTempUnschedulable；测试可临时覆盖为 stub。
var openaiIncompleteStreamPolicyTrigger = func(s *OpenAIGatewayService, ctx context.Context, account *Account, code int, body []byte) bool {
	if s == nil || s.rateLimitService == nil {
		return false
	}
	return s.rateLimitService.tryTempUnschedulable(ctx, account, code, body)
}

// handleIncompleteStreamFailure 是 Anthropic 版 GatewayService.handleIncompleteStreamFailure
// 在 OpenAI gateway 上的对称实现。
//
// 触发条件、动作集合、失效约束与 Anthropic 版完全一致：
//
//   - 触发条件：streamErr 命中 isUpstreamIncompleteStreamError（"missing terminal event"
//     或 "upstream stream ended without terminal event"）。其他错误（disconnect、
//     timeout、context canceled、HTTP 4xx/5xx 等）一律不触发。
//
//   - 动作（按重要性）：
//      0) emit SSE error event 给客户端（仅 isStream=true 时）
//      1) 解 OpenAI sticky 绑定（让用户下一次请求自动回落到健康账号）
//      2) （可选）把故障作为合成 HTTP 599 喂给 tryTempUnschedulable，默认无效果
//
//   - 注意：与 Anthropic 版不同，OpenAI 的 sticky 走 deleteStickySessionAccountID，
//     该函数内部同时清理 primary key 与 legacy key（双写迁移期的兼容性）。
//
// 不命中字符串的所有错误路径完全不受影响；命中后唯一可观察到的副作用是
// SSE error event 字节、Redis 一次 DEL、可选的 RateLimitService 调用。
func (s *OpenAIGatewayService) handleIncompleteStreamFailure(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	isStream bool,
	streamErr error,
) {
	if s == nil || account == nil || streamErr == nil {
		return
	}
	if !isUpstreamIncompleteStreamError(streamErr) {
		return
	}

	// 0) 让客户端立刻知道失败。仅当本次请求是 stream=true 时才写 SSE 字节，
	//    避免给 buffered/JSON 路径注入畸形 body。
	emitIncompleteStreamErrorEvent(c, isStream)

	// 调用方 ctx 多半已被 idle timeout 取消，必须切到独立 ctx。
	cleanupCtx, cancel := context.WithTimeout(context.Background(), incompleteStreamCleanupTimeout)
	defer cancel()

	// 1) 解 sticky：让该用户的下一次请求自动回落到其他健康账号（核心目标）。
	//    sessionHash 由 GenerateSessionHash 按用户级信号（session_id /
	//    conversation_id / prompt_cache_key / 内容回退）算出，仅删除本用户的
	//    sticky 绑定，不影响其他用户在同一账号上的绑定。
	sessionHash := s.GenerateSessionHash(c, body)
	if sessionHash != "" {
		groupID := getOpenAIGroupIDFromContext(c)
		if delErr := s.deleteStickySessionAccountID(cleanupCtx, &groupID, sessionHash); delErr != nil {
			logger.LegacyPrintf("service.openai_gateway",
				"incomplete_stream_unbind_failed account=%d group=%d session=%s err=%v",
				account.ID, groupID, shortSessionHash(sessionHash), delErr)
		} else {
			logger.LegacyPrintf("service.openai_gateway",
				"incomplete_stream_sticky_unbound account=%d group=%d session=%s stream=%v",
				account.ID, groupID, shortSessionHash(sessionHash), isStream)
		}
	}

	// 2) 可选保险：把故障作为合成 HTTP 599 喂给 temp_unschedulable_rules 评估器。
	//    与 Anthropic 版同口径：默认无效果（运维未配规则时），建议仅给"持续坏"
	//    渠道（错误率 >30%）配；偶发抖动 (<5%) 不要配，避免误伤。
	triggered := openaiIncompleteStreamPolicyTrigger(
		s,
		cleanupCtx,
		account,
		incompleteStreamSyntheticCode,
		[]byte(streamErr.Error()),
	)
	if triggered {
		logger.LegacyPrintf("service.openai_gateway",
			"incomplete_stream_handed_to_policy account=%d code=%d outcome=temp_unscheduled",
			account.ID, incompleteStreamSyntheticCode)
	}
}
