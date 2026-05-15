package admin

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// InvoiceHandler handles admin invoice review endpoints.
type InvoiceHandler struct {
	invoiceService *service.InvoiceService
}

// NewInvoiceHandler creates a new admin InvoiceHandler.
func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{invoiceService: invoiceService}
}

// ListInvoices returns paginated invoices for admin review.
// GET /api/v1/admin/invoices?status=&user_id=&email=&page=&page_size=
func (h *InvoiceHandler) ListInvoices(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	var userID int64
	if uid := c.Query("user_id"); uid != "" {
		if v, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = v
		}
	}
	out, err := h.invoiceService.AdminListInvoices(c.Request.Context(), service.AdminInvoiceListFilter{
		Status:      c.Query("status"),
		UserID:      userID,
		Email:       c.Query("email"),
		Page:        page,
		PageSize:    pageSize,
		VoidPending: c.Query("void_pending") == "true",
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, out.Items, int64(out.Total), out.Page, out.Size)
}

// GetInvoiceDetail returns a single invoice with items.
// GET /api/v1/admin/invoices/:id
func (h *InvoiceHandler) GetInvoiceDetail(c *gin.Context) {
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	dto, err := h.invoiceService.AdminGetDetail(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto)
}

// ApproveInvoice transitions pending → approved.
//
// POST /api/v1/admin/invoices/:id/approve
//
//	body: { "notes": "...", "invoice_kind": "normal|special", "provider": "caiyuntong|manual" }
//
// 当 provider 为非 manual 渠道时，service 层会在事务内把 provider_state 置 queued，
// 由后台 worker 异步调用第三方接口完成自动开票。
func (h *InvoiceHandler) ApproveInvoice(c *gin.Context) {
	adminID, ok := requireAdminID(c)
	if !ok {
		return
	}
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Notes           string `json:"notes"`
		InvoiceKind     string `json:"invoice_kind"`
		Provider        string `json:"provider"`
		InvoiceTypeCode string `json:"invoice_type_code"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.invoiceService.AdminApprove(c.Request.Context(), service.AdminApproveParams{
		AdminID:         adminID,
		InvoiceID:       id,
		Notes:           req.Notes,
		InvoiceKind:     req.InvoiceKind,
		Provider:        req.Provider,
		InvoiceTypeCode: req.InvoiceTypeCode,
	}); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "approved"})
}

// RejectInvoice transitions pending → rejected and releases the orders.
// POST /api/v1/admin/invoices/:id/reject  body: { "reason": "..." }
func (h *InvoiceHandler) RejectInvoice(c *gin.Context) {
	adminID, ok := requireAdminID(c)
	if !ok {
		return
	}
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if err := h.invoiceService.AdminReject(c.Request.Context(), adminID, id, req.Reason); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "rejected"})
}

// MarkIssued sets the invoice to issued without uploading a PDF.
// POST /api/v1/admin/invoices/:id/mark-issued  body: { "invoice_no": "..." }
func (h *InvoiceHandler) MarkIssued(c *gin.Context) {
	adminID, ok := requireAdminID(c)
	if !ok {
		return
	}
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	var req struct {
		InvoiceNo string `json:"invoice_no"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if err := h.invoiceService.AdminMarkIssued(c.Request.Context(), adminID, id, req.InvoiceNo); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "issued"})
}

// VoidInvoice transitions approved/issued → voided.
// POST /api/v1/admin/invoices/:id/void  body: { "reason": "..." }
func (h *InvoiceHandler) VoidInvoice(c *gin.Context) {
	adminID, ok := requireAdminID(c)
	if !ok {
		return
	}
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if err := h.invoiceService.AdminVoid(c.Request.Context(), adminID, id, req.Reason); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "voided"})
}

// UploadPDF accepts a multipart PDF + optional invoice_no and marks issued.
// POST /api/v1/admin/invoices/:id/upload-pdf
// Form fields:
//   - file       (required): the PDF binary
//   - invoice_no (optional): real invoice number
//
// Limits: max size = service.PDFMaxBytes(); content must start with "%PDF-".
func (h *InvoiceHandler) UploadPDF(c *gin.Context) {
	adminID, ok := requireAdminID(c)
	if !ok {
		return
	}
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}

	maxBytes := h.invoiceService.PDFMaxBytes()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes+4096)

	header, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file: "+err.Error())
		return
	}
	if header.Size > maxBytes {
		response.BadRequest(c, "file too large")
		return
	}
	f, err := header.Open()
	if err != nil {
		response.BadRequest(c, "open uploaded file: "+err.Error())
		return
	}
	defer f.Close()

	// Magic bytes 校验：合法 PDF 以 "%PDF-" 开头
	magic := make([]byte, 5)
	if _, err := io.ReadFull(f, magic); err != nil {
		response.BadRequest(c, "read file: "+err.Error())
		return
	}
	if string(magic) != "%PDF-" {
		response.BadRequest(c, "uploaded file is not a valid PDF")
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		response.BadRequest(c, "rewind file: "+err.Error())
		return
	}

	invoiceNo := strings.TrimSpace(c.PostForm("invoice_no"))
	if err := h.invoiceService.AdminUploadPDF(c.Request.Context(), adminID, id, f, header.Filename, invoiceNo); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "uploaded"})
}

// DownloadPDF streams a PDF file to admin (no status restriction, for review).
// GET /api/v1/admin/invoices/:id/pdf
func (h *InvoiceHandler) DownloadPDF(c *gin.Context) {
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	content, err := h.invoiceService.GetAdminPDF(c.Request.Context(), id)
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
		_ = c.Error(err)
	}
}

// GetUserInvoiceConfig 查询单用户发票可见性 override。
// GET /api/v1/admin/users/:id/invoice-config
func (h *InvoiceHandler) GetUserInvoiceConfig(c *gin.Context) {
	userID, ok := parseUserIDParam(c)
	if !ok {
		return
	}
	enabled, err := h.invoiceService.GetUserInvoiceEnabled(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"enabled": enabled})
}

// SetUserInvoiceConfig 写单用户发票可见性 override。
// PUT /api/v1/admin/users/:id/invoice-config
// body: { "enabled": bool }
func (h *InvoiceHandler) SetUserInvoiceConfig(c *gin.Context) {
	userID, ok := parseUserIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if err := h.invoiceService.AdminSetUserInvoiceEnabled(c.Request.Context(), userID, req.Enabled); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"enabled": req.Enabled})
}

func parseUserIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return 0, false
	}
	return id, true
}

func parseInvoiceIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid invoice ID")
		return 0, false
	}
	return id, true
}

// requireAdminID 从 context 取出当前管理员 user id；未登录返回 401。
func requireAdminID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "admin authentication required")
		return 0, false
	}
	return subject.UserID, true
}

// RetryIssue 重新尝试自动开票（仅 provider_state=failed 时有效）。
// POST /api/v1/admin/invoices/:id/retry-issue
func (h *InvoiceHandler) RetryIssue(c *gin.Context) {
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	if err := h.invoiceService.AdminRetryIssue(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "queued"})
}

// RetryReverse 重新尝试自动红冲（仅 provider_state=reverse_failed 时有效）。
// POST /api/v1/admin/invoices/:id/retry-reverse
func (h *InvoiceHandler) RetryReverse(c *gin.Context) {
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	if err := h.invoiceService.AdminRetryReverse(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "reverse_queued"})
}

// MarkReversed 管理员标记「已在第三方平台手工红冲」（兜底通道）。
// POST /api/v1/admin/invoices/:id/mark-reversed  body: { "red_invoice_no": "..." }
func (h *InvoiceHandler) MarkReversed(c *gin.Context) {
	adminID, ok := requireAdminID(c)
	if !ok {
		return
	}
	id, ok := parseInvoiceIDParam(c)
	if !ok {
		return
	}
	var req struct {
		RedInvoiceNo string `json:"red_invoice_no"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.invoiceService.AdminMarkReversed(c.Request.Context(), adminID, id, req.RedInvoiceNo); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "marked"})
}

// ApproveVoidRequest 管理员通过用户的作废申请，同事务触发红冲流水线。
// POST /api/v1/admin/invoice-void-requests/:id/approve  body: { "notes": "可选备注" }
func (h *InvoiceHandler) ApproveVoidRequest(c *gin.Context) {
	adminID, ok := requireAdminID(c)
	if !ok {
		return
	}
	requestID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || requestID <= 0 {
		response.BadRequest(c, "Invalid void request ID")
		return
	}
	var req struct {
		Notes string `json:"notes"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.invoiceService.AdminApproveVoidRequest(c.Request.Context(), adminID, requestID, req.Notes); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "approved"})
}

// RejectVoidRequest 管理员驳回作废申请，必填驳回理由。
// POST /api/v1/admin/invoice-void-requests/:id/reject  body: { "reason": "..." }
func (h *InvoiceHandler) RejectVoidRequest(c *gin.Context) {
	adminID, ok := requireAdminID(c)
	if !ok {
		return
	}
	requestID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || requestID <= 0 {
		response.BadRequest(c, "Invalid void request ID")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	if err := h.invoiceService.AdminRejectVoidRequest(c.Request.Context(), adminID, requestID, req.Reason); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "rejected"})
}
