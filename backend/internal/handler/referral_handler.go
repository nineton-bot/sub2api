package handler

import (
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
