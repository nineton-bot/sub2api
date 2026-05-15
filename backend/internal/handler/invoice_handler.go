package handler

import (
	"io"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// InvoiceHandler handles user-facing invoice endpoints.
type InvoiceHandler struct {
	invoiceService *service.InvoiceService
}

// NewInvoiceHandler creates a new InvoiceHandler.
func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{invoiceService: invoiceService}
}

// GetEligibility returns whether the invoice feature is visible/usable for the
// authenticated user. Frontend AppSidebar polls this to decide whether to show
// the "我的发票" menu entry.
// GET /api/v1/invoices/eligibility
func (h *InvoiceHandler) GetEligibility(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	enabled, err := h.invoiceService.IsVisibleForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"enabled": enabled})
}

// ListEligibleOrders returns the user's orders eligible for invoicing.
// GET /api/v1/invoices/eligible-orders
func (h *InvoiceHandler) ListEligibleOrders(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	if !h.guardVisibility(c, subject.UserID) {
		return
	}
	orders, err := h.invoiceService.ListEligibleOrders(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": orders})
}

// guardVisibility 在每个用户端 invoice 接口入口校验功能是否对该用户可见。
// 不可见时返回 INVOICE_NOT_AVAILABLE 并写响应；返回 false 表示已写响应、handler 应直接 return。
func (h *InvoiceHandler) guardVisibility(c *gin.Context, userID int64) bool {
	visible, err := h.invoiceService.IsVisibleForUser(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	if !visible {
		response.ErrorFrom(c, service.ErrInvoiceNotAvailable)
		return false
	}
	return true
}

// ListMyInvoices returns the user's invoice list.
// GET /api/v1/invoices
func (h *InvoiceHandler) ListMyInvoices(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	if !h.guardVisibility(c, subject.UserID) {
		return
	}
	page, pageSize := response.ParsePagination(c)
	out, err := h.invoiceService.ListMyInvoices(c.Request.Context(), subject.UserID, service.InvoiceListFilter{
		Status:   c.Query("status"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, out.Items, int64(out.Total), out.Page, out.Size)
}

// CreateInvoice handles user invoice application submission.
// POST /api/v1/invoices
func (h *InvoiceHandler) CreateInvoice(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	if !h.guardVisibility(c, subject.UserID) {
		return
	}
	var req service.CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	dto, err := h.invoiceService.CreateApplication(c.Request.Context(), subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto)
}

// GetInvoiceDetail returns a single invoice owned by the authenticated user.
// GET /api/v1/invoices/:id
func (h *InvoiceHandler) GetInvoiceDetail(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	if !h.guardVisibility(c, subject.UserID) {
		return
	}
	id, ok := parseInvoiceID(c)
	if !ok {
		return
	}
	dto, err := h.invoiceService.GetMyInvoiceDetail(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto)
}

// CancelInvoice allows the user to cancel a pending invoice application.
// POST /api/v1/invoices/:id/cancel
func (h *InvoiceHandler) CancelInvoice(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	if !h.guardVisibility(c, subject.UserID) {
		return
	}
	id, ok := parseInvoiceID(c)
	if !ok {
		return
	}
	if err := h.invoiceService.CancelByUser(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "invoice cancelled"})
}

// RequestVoid 用户对 issued 发票发起作废申请。
// POST /api/v1/invoices/:id/void-request  body: { "reason": "..." }
func (h *InvoiceHandler) RequestVoid(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	if !h.guardVisibility(c, subject.UserID) {
		return
	}
	id, ok := parseInvoiceID(c)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	dto, err := h.invoiceService.CreateVoidRequest(c.Request.Context(), subject.UserID, id, req.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto)
}

// DownloadInvoicePDF streams the issued PDF to the user.
// GET /api/v1/invoices/:id/pdf
func (h *InvoiceHandler) DownloadInvoicePDF(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	if !h.guardVisibility(c, subject.UserID) {
		return
	}
	id, ok := parseInvoiceID(c)
	if !ok {
		return
	}
	content, err := h.invoiceService.GetPDFForUser(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	defer content.Reader.Close()
	c.Header("Content-Type", content.ContentType)
	c.Header("Content-Disposition", `attachment; filename="`+content.Filename+`"`)
	if content.Size > 0 {
		c.Header("Content-Length", strconv.FormatInt(content.Size, 10))
	}
	if _, err := io.Copy(c.Writer, content.Reader); err != nil {
		// 已开始写响应，无法再返回结构化 error
		_ = c.Error(err)
	}
}

func parseInvoiceID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid invoice ID")
		return 0, false
	}
	return id, true
}
