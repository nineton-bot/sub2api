package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ReferralHandler 处理管理端的邀请返佣接口。
//
// 所有接口落在 /api/v1/admin/referral/*，鉴权由 admin 中间件负责。
type ReferralHandler struct {
	referralService *service.ReferralService
}

// NewReferralHandler creates a new admin ReferralHandler.
func NewReferralHandler(referralService *service.ReferralService) *ReferralHandler {
	return &ReferralHandler{referralService: referralService}
}

// GetOverview 返回全局邀请返佣统计。
//
// GET /api/v1/admin/referral/overview
func (h *ReferralHandler) GetOverview(c *gin.Context) {
	stats, err := h.referralService.GetGlobalReferralStats(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// ListTopReferrers 返回按总佣金或邀请人数排序的 Top 邀请人。
//
// GET /api/v1/admin/referral/top?sort=commission|count&limit=20
func (h *ReferralHandler) ListTopReferrers(c *gin.Context) {
	sortBy := strings.TrimSpace(c.Query("sort"))
	if sortBy != "count" {
		sortBy = "commission"
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 20
	}

	rows, err := h.referralService.ListTopReferrers(c.Request.Context(), sortBy, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rows)
}

// GetReferrerDrilldown 返回指定邀请人下的被邀请人列表（分页）。
//
// GET /api/v1/admin/referral/user/:id?page=1&page_size=20
func (h *ReferralHandler) GetReferrerDrilldown(c *gin.Context) {
	referrerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || referrerID <= 0 {
		response.BadRequest(c, "Invalid referrer ID")
		return
	}

	page, pageSize := response.ParsePagination(c)

	rows, total, err := h.referralService.GetReferrerDrilldown(c.Request.Context(), referrerID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, rows, total, page, pageSize)
}

// --- 单用户返佣配置（V2）---

// UpsertUserReferralConfigRequest 管理员写入单用户配置载荷。
//
// 字段语义：
//   - enabled 指针：nil=跟随全局；true/false=强制覆盖
//   - commission_rate_override 指针：nil=跟随全局，范围 [0,1]
//   - referee_bonus_override 指针：nil=跟随全局，>=0
//   - withdrawal_allowed：默认 false
//   - notes：管理员备注
type UpsertUserReferralConfigRequest struct {
	Enabled                *bool    `json:"enabled"`
	CommissionRateOverride *float64 `json:"commission_rate_override"`
	RefereeBonusOverride   *float64 `json:"referee_bonus_override"`
	WithdrawalAllowed      bool     `json:"withdrawal_allowed"`
	Notes                  string   `json:"notes"`
}

// GetUserReferralConfig 查单用户返佣配置。
//
// GET /api/v1/admin/users/:id/referral-config
func (h *ReferralHandler) GetUserReferralConfig(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	view, err := h.referralService.GetUserReferralConfig(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}

// UpsertUserReferralConfig 新增/更新单用户返佣配置。
//
// PUT /api/v1/admin/users/:id/referral-config
func (h *ReferralHandler) UpsertUserReferralConfig(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	var req UpsertUserReferralConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	view, err := h.referralService.UpsertUserReferralConfig(c.Request.Context(), userID, service.UserReferralConfigInput{
		Enabled:                req.Enabled,
		CommissionRateOverride: req.CommissionRateOverride,
		RefereeBonusOverride:   req.RefereeBonusOverride,
		WithdrawalAllowed:      req.WithdrawalAllowed,
		Notes:                  strings.TrimSpace(req.Notes),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, view)
}
