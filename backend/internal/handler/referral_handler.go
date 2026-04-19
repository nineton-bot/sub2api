package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ReferralHandler 处理用户视角的邀请返佣接口。
//
// 所有接口均落在 /api/v1/user/referral/*，鉴权由 JWT 中间件负责。
type ReferralHandler struct {
	referralService *service.ReferralService
	settingService  *service.SettingService
}

// NewReferralHandler creates a new ReferralHandler.
func NewReferralHandler(
	referralService *service.ReferralService,
	settingService *service.SettingService,
) *ReferralHandler {
	return &ReferralHandler{
		referralService: referralService,
		settingService:  settingService,
	}
}

// EnsureInviteCodeResponse 确保邀请码存在的响应
type EnsureInviteCodeResponse struct {
	InviteCode string `json:"invite_code"`
	Enabled    bool   `json:"enabled"`
}

// EnsureInviteCode 幂等生成或返回当前用户的邀请码。
//
// POST /api/v1/user/referral/ensure-code
func (h *ReferralHandler) EnsureInviteCode(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	enabled := h.settingService != nil && h.settingService.IsReferralEnabled(c.Request.Context())

	// 即使返佣未启用也允许获取/生成邀请码（便于用户预先查看），仅用于展示。
	code, err := h.referralService.EnsureInviteCode(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, EnsureInviteCodeResponse{
		InviteCode: code,
		Enabled:    enabled,
	})
}

// GetMyOverview 返回当前用户的邀请返佣统计。
//
// GET /api/v1/user/referral/overview
func (h *ReferralHandler) GetMyOverview(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	stats, err := h.referralService.GetMyReferralStats(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, stats)
}

// EligibilityResponse 推广页可见性响应
type EligibilityResponse struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason"`
}

// GetEligibility 返回当前用户是否可以访问推广页。
//
// 前端在 ReferralView mount 时调用；enabled=false 时展示占位，
// 并隐藏侧边栏"我的推广"入口。
//
// GET /api/v1/user/referral/eligibility
func (h *ReferralHandler) GetEligibility(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	enabled, reason, err := h.referralService.IsReferralVisibleForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, EligibilityResponse{
		Enabled: enabled,
		Reason:  reason,
	})
}

// ListMyCommissions 返回当前用户的返佣明细（分页）。
//
// GET /api/v1/user/referral/commissions?page=&size=
func (h *ReferralHandler) ListMyCommissions(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	page, size := response.ParsePagination(c)

	logs, total, err := h.referralService.ListMyCommissionLogs(c.Request.Context(), subject.UserID, page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Paginated(c, logs, total, page, size)
}

// TransferToBalanceRequest 转入余额请求
type TransferToBalanceRequest struct {
	Amount float64 `json:"amount" binding:"required"`
}

// TransferToBalance 把 referral_usable 池里的指定金额转入账户余额（balance）。
//
// 前置：V2 使用池模式，每次释放直接入 referral_usable；用户需主动调用此接口才会进 balance。
//
// 行为（事务内）：
//  1. 校验用户可访问推广页（eligibility）
//  2. 校验 amount >= ReferralUsableMinTransfer 且 referral_usable >= amount
//  3. UpdateReferralUsable(-amount) + UpdateBalance(+amount)
//  4. 失效用户缓存
//
// POST /api/v1/user/referral/transfer-to-balance
func (h *ReferralHandler) TransferToBalance(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req TransferToBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}

	// 首先校验推广可见性（master + per-user override）；不可见直接拒绝
	visible, _, err := h.referralService.IsReferralVisibleForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if !visible {
		response.Forbidden(c, "Referral program is not available for your account")
		return
	}

	if err := h.referralService.TransferUsableToBalance(c.Request.Context(), subject.UserID, req.Amount); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// 返回最新 stats，前端可直接刷新统计卡
	stats, err := h.referralService.GetMyReferralStats(c.Request.Context(), subject.UserID)
	if err != nil {
		response.Success(c, gin.H{"ok": true})
		return
	}
	response.Success(c, stats)
}

// ListMyReleaseLogsDaily 返回当前用户的释放日志，按 (day, trigger_type) 聚合分页。
//
// GET /api/v1/user/referral/release-logs?commission_id=&page=&size=
//
// 查询参数：
//   - commission_id（可选）：限定单笔 commission。不传则返回用户全部释放记录。
//   - page / size：分页，size 默认 30，最大 100。
//
// 单次释放日志粒度对用户过细；此接口按天聚合，单笔 commission 30 天订阅 ≤ 30 行，
// 频繁的充值消费归因当天会合并为一行。
func (h *ReferralHandler) ListMyReleaseLogsDaily(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var commissionIDPtr *int64
	if raw := c.Query("commission_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			response.BadRequest(c, "Invalid commission_id")
			return
		}
		commissionIDPtr = &v
	}

	page, size := response.ParsePagination(c)

	rows, total, err := h.referralService.ListMyReleaseLogsDaily(
		c.Request.Context(), subject.UserID, commissionIDPtr, page, size,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, rows, total, page, size)
}
