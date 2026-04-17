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
