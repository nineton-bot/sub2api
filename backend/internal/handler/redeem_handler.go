package handler

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RedeemHandler handles redeem code-related requests
type RedeemHandler struct {
	redeemService *service.RedeemService
}

// NewRedeemHandler creates a new RedeemHandler
func NewRedeemHandler(redeemService *service.RedeemService) *RedeemHandler {
	return &RedeemHandler{
		redeemService: redeemService,
	}
}

// RedeemRequest represents the redeem code request payload.
// purchase_intent / renew_subscription_id 用于 subscription 类型的二选一弹窗（参考 PR 81ee8105）：
//   - 缺省 / "": 老行为，走 AssignOrExtendSubscription
//   - "renew":  对 renew_subscription_id 指定的现有订阅延长 expires_at（必填 renew_subscription_id）
//   - "new":    新建一行独立订阅
type RedeemRequest struct {
	Code                string `json:"code" binding:"required"`
	PurchaseIntent      string `json:"purchase_intent,omitempty"`
	RenewSubscriptionID int64  `json:"renew_subscription_id,omitempty"`
}

// RedeemResponse represents the redeem response
type RedeemResponse struct {
	Message        string   `json:"message"`
	Type           string   `json:"type"`
	Value          float64  `json:"value"`
	NewBalance     *float64 `json:"new_balance,omitempty"`
	NewConcurrency *int     `json:"new_concurrency,omitempty"`
}

// RedeemPreviewRequest 用户兑换前的只读校验请求
type RedeemPreviewRequest struct {
	Code string `json:"code" binding:"required"`
}

// RedeemPreviewSubInfoDTO 已持有订阅简要信息
type RedeemPreviewSubInfoDTO struct {
	ID        int64     `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RedeemPreviewResponse 预览响应。subscription 类型才有 group / 订阅相关字段。
type RedeemPreviewResponse struct {
	Type           string                    `json:"type"`
	Value          float64                   `json:"value"`
	GroupID        *int64                    `json:"group_id,omitempty"`
	GroupName      string                    `json:"group_name,omitempty"`
	ValidityDays   int                       `json:"validity_days,omitempty"`
	ExistingActive []RedeemPreviewSubInfoDTO `json:"existing_active_subs,omitempty"`
	StackCap       int                       `json:"stack_cap,omitempty"`
	StackCount     int                       `json:"stack_count,omitempty"`
	IsReduce       bool                      `json:"is_reduce,omitempty"`
}

// Redeem handles redeeming a code
// POST /api/v1/redeem
func (h *RedeemHandler) Redeem(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req RedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.redeemService.RedeemWithOptions(c.Request.Context(), subject.UserID, req.Code, service.RedeemOptions{
		PurchaseIntent:      req.PurchaseIntent,
		RenewSubscriptionID: req.RenewSubscriptionID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.RedeemCodeFromService(result))
}

// Preview validates a redeem code (read-only) so the frontend can decide whether to show
// a "renew vs buy another" dialog for subscription codes.
// POST /api/v1/redeem/preview
func (h *RedeemHandler) Preview(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req RedeemPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	preview, err := h.redeemService.PreviewRedeem(c.Request.Context(), subject.UserID, req.Code)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	resp := RedeemPreviewResponse{
		Type:         preview.Type,
		Value:        preview.Value,
		GroupID:      preview.GroupID,
		GroupName:    preview.GroupName,
		ValidityDays: preview.ValidityDays,
		StackCap:     preview.StackCap,
		StackCount:   preview.StackCount,
		IsReduce:     preview.IsReduce,
	}
	if len(preview.ExistingActive) > 0 {
		resp.ExistingActive = make([]RedeemPreviewSubInfoDTO, 0, len(preview.ExistingActive))
		for i := range preview.ExistingActive {
			resp.ExistingActive = append(resp.ExistingActive, RedeemPreviewSubInfoDTO{
				ID:        preview.ExistingActive[i].ID,
				ExpiresAt: preview.ExistingActive[i].ExpiresAt,
			})
		}
	}
	response.Success(c, resp)
}

// GetHistory returns the user's redemption history
// GET /api/v1/redeem/history
func (h *RedeemHandler) GetHistory(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	// Default limit is 25
	limit := 25

	codes, err := h.redeemService.GetUserHistory(c.Request.Context(), subject.UserID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]dto.RedeemCode, 0, len(codes))
	for i := range codes {
		out = append(out, *dto.RedeemCodeFromService(&codes[i]))
	}
	response.Success(c, out)
}
