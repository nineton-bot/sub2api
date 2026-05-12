// Package service — Invoice (发票) business logic.
//
// 状态机：
//
//	pending  -> approved -> issued                （正常审批 + 上传 PDF）
//	pending  -> rejected                           （管理员驳回，订单释放）
//	pending  -> voided                             （用户取消，订单释放）
//	approved -> voided                             （管理员作废，订单释放）
//	issued   -> voided                             （管理员作废，订单释放，PDF 保留作历史）
//
// 防重复：invoice_items.payment_order_id 在 SQL 层 UNIQUE，
// 进入 rejected/voided 时事务内 hard-delete 对应 items 行释放约束。
//
// 已开发票订单不允许退款：退款入口调用 IsOrderInvoiceLocked 校验。
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/invoice"
	"github.com/Wei-Shaw/sub2api/ent/invoiceitem"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// --- 错误定义 ---

var (
	ErrInvoiceNotFound          = infraerrors.NotFound("INVOICE_NOT_FOUND", "invoice not found")
	ErrInvoiceForbidden         = infraerrors.Forbidden("INVOICE_FORBIDDEN", "invoice not accessible")
	ErrInvoiceNotAvailable      = infraerrors.Forbidden("INVOICE_NOT_AVAILABLE", "invoice feature is not available for this account")
	ErrInvoiceInvalidTitleType  = infraerrors.BadRequest("INVOICE_INVALID_TITLE_TYPE", "title_type must be personal or business")
	ErrInvoiceTitleRequired     = infraerrors.BadRequest("INVOICE_TITLE_REQUIRED", "invoice title is required")
	ErrInvoiceTaxNoRequired     = infraerrors.BadRequest("INVOICE_TAX_NO_REQUIRED", "tax_no is required for business invoices")
	ErrInvoiceNoOrders          = infraerrors.BadRequest("INVOICE_NO_ORDERS", "at least one order must be selected")
	ErrInvoiceTooManyOrders     = infraerrors.BadRequest("INVOICE_TOO_MANY_ORDERS", "too many orders selected")
	ErrInvoiceOrderAlreadyUsed  = infraerrors.Conflict("INVOICE_ORDER_ALREADY_USED", "one or more orders are already bound to another invoice")
	ErrInvoiceOrderIneligible   = infraerrors.BadRequest("INVOICE_ORDER_INELIGIBLE", "one or more orders are not eligible for invoicing")
	ErrInvoiceNotPending        = infraerrors.Conflict("INVOICE_NOT_PENDING", "invoice is not in pending state")
	ErrInvoiceNotApprovable     = infraerrors.Conflict("INVOICE_NOT_APPROVABLE", "invoice cannot be approved in current state")
	ErrInvoiceNotIssuable       = infraerrors.Conflict("INVOICE_NOT_ISSUABLE", "invoice cannot be issued in current state")
	ErrInvoiceNotVoidable       = infraerrors.Conflict("INVOICE_NOT_VOIDABLE", "invoice cannot be voided in current state")
	ErrInvoicePDFMissing        = infraerrors.NotFound("INVOICE_PDF_MISSING", "invoice pdf is not available")
	ErrInvoiceInvoiceNoRequired = infraerrors.BadRequest("INVOICE_NO_REQUIRED", "invoice_no is required to mark issued")
	ErrInvoiceInvalidAmount     = infraerrors.BadRequest("INVOICE_INVALID_AMOUNT", "invoice amount must be greater than zero")
	ErrInvoiceContactEmail      = infraerrors.BadRequest("INVOICE_CONTACT_EMAIL_INVALID", "contact_email format is invalid")
	ErrInvoiceNotesTooLong      = infraerrors.BadRequest("INVOICE_NOTES_TOO_LONG", "notes is too long")
	ErrInvoiceReasonTooLong     = infraerrors.BadRequest("INVOICE_REASON_TOO_LONG", "reason is too long")
)

// 字段长度限制
const (
	maxInvoiceNotesLen  = 2000
	maxInvoiceReasonLen = 2000
)

// --- 状态常量 ---

const (
	InvoiceStatusPending  = "pending"
	InvoiceStatusApproved = "approved"
	InvoiceStatusIssued   = "issued"
	InvoiceStatusRejected = "rejected"
	InvoiceStatusVoided   = "voided"

	InvoiceTitleTypePersonal = "personal"
	InvoiceTitleTypeBusiness = "business"

	// PaymentOrder.invoice_status 反规范化字段值
	orderInvoiceStatusPending = "pending"
	orderInvoiceStatusIssued  = "issued"
)

// --- DTO ---

// EligibleOrder 可开票订单（用于申请页订单选择列表）
type EligibleOrder struct {
	ID          int64     `json:"id"`
	OrderNo     string    `json:"order_no"`
	ProductName string    `json:"product_name"`
	OrderType   string    `json:"order_type"`
	PayAmount   float64   `json:"pay_amount"`
	PaidAt      time.Time `json:"paid_at"`
}

// CreateInvoiceRequest 用户提交发票申请的入参
type CreateInvoiceRequest struct {
	TitleType    string  `json:"title_type"`
	Title        string  `json:"title"`
	TaxNo        string  `json:"tax_no"`
	ContactEmail string  `json:"contact_email"`
	Notes        string  `json:"notes"`
	OrderIDs     []int64 `json:"order_ids"`
}

// InvoiceDTO 列表视图
type InvoiceDTO struct {
	ID           int64     `json:"id"`
	InvoiceNo    string    `json:"invoice_no"`
	UserID       int64     `json:"user_id"`
	UserEmail    string    `json:"user_email"`
	TitleType    string    `json:"title_type"`
	Title        string    `json:"title"`
	TaxNo        string    `json:"tax_no"`
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	Status       string    `json:"status"`
	OrderCount   int       `json:"order_count"`
	SubmittedAt  time.Time `json:"submitted_at"`
	ContactEmail string    `json:"contact_email"`
}

// InvoiceItemDTO 订单条目
type InvoiceItemDTO struct {
	OrderID     int64     `json:"order_id"`
	OrderNo     string    `json:"order_no"`
	ProductName string    `json:"product_name"`
	OrderType   string    `json:"order_type"`
	PayAmount   float64   `json:"pay_amount"`
	PaidAt      time.Time `json:"paid_at"`
}

// InvoiceDetailDTO 详情视图
type InvoiceDetailDTO struct {
	InvoiceDTO
	Notes           string           `json:"notes"`
	ReviewedAt      *time.Time       `json:"reviewed_at,omitempty"`
	ReviewedBy      *int64           `json:"reviewed_by,omitempty"`
	ReviewNotes     string           `json:"review_notes"`
	IssuedAt        *time.Time       `json:"issued_at,omitempty"`
	PDFOriginalName string           `json:"pdf_original_name,omitempty"`
	PDFAvailable    bool             `json:"pdf_available"`
	Provider        string           `json:"provider"`
	Items           []InvoiceItemDTO `json:"items"`
}

// InvoiceListFilter 用户列表筛选
type InvoiceListFilter struct {
	Status   string
	Page     int
	PageSize int
}

// AdminInvoiceListFilter 管理员列表筛选
type AdminInvoiceListFilter struct {
	Status   string
	UserID   int64
	Email    string
	Page     int
	PageSize int
}

// PaginatedInvoices 通用分页响应
type PaginatedInvoices struct {
	Items []InvoiceDTO `json:"items"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
	Size  int          `json:"size"`
}

// InvoicePDFContent PDF 下载结果
type InvoicePDFContent struct {
	Reader      io.ReadCloser
	Filename    string
	ContentType string
	Size        int64
}

// --- Service ---

type InvoiceService struct {
	entClient      *dbent.Client
	pdfStore       InvoicePDFStore
	cfg            *config.Config
	settingService *SettingService
	log            *slog.Logger
}

func NewInvoiceService(entClient *dbent.Client, pdfStore InvoicePDFStore, cfg *config.Config, settingService *SettingService) *InvoiceService {
	return &InvoiceService{
		entClient:      entClient,
		pdfStore:       pdfStore,
		cfg:            cfg,
		settingService: settingService,
		log:            slog.Default(),
	}
}

// --- 可见性 ---

// IsGloballyEnabled 全局开关 invoice_enabled。
func (s *InvoiceService) IsGloballyEnabled(ctx context.Context) bool {
	if s.settingService == nil {
		return false
	}
	return s.settingService.IsInvoiceEnabled(ctx)
}

// IsDefaultForAllUsers 全局开关 invoice_default_for_all_users。
// 仅在 IsGloballyEnabled 为 true 时有意义。
func (s *InvoiceService) IsDefaultForAllUsers(ctx context.Context) bool {
	if s.settingService == nil {
		return false
	}
	return s.settingService.IsInvoiceDefaultForAllUsers(ctx)
}

// IsVisibleForUser 真值表：
//
//	A=false                  → false
//	A=true && B=true         → true
//	A=true && B=false && C=t → true
//	A=true && B=false && C=f → false
func (s *InvoiceService) IsVisibleForUser(ctx context.Context, userID int64) (bool, error) {
	if !s.IsGloballyEnabled(ctx) {
		return false, nil
	}
	if s.IsDefaultForAllUsers(ctx) {
		return true, nil
	}
	if userID <= 0 {
		return false, nil
	}
	u, err := s.entClient.User.Query().
		Where(user.IDEQ(userID)).
		Select(user.FieldInvoiceEnabled).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("query user invoice flag: %w", err)
	}
	return u.InvoiceEnabled, nil
}

// AdminSetUserInvoiceEnabled 管理员设置单用户 invoice_enabled。
func (s *InvoiceService) AdminSetUserInvoiceEnabled(ctx context.Context, userID int64, enabled bool) error {
	if userID <= 0 {
		return infraerrors.BadRequest("INVALID_USER_ID", "user id is required")
	}
	if _, err := s.entClient.User.UpdateOneID(userID).SetInvoiceEnabled(enabled).Save(ctx); err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("USER_NOT_FOUND", "user not found")
		}
		return fmt.Errorf("update user invoice_enabled: %w", err)
	}
	s.log.Info("admin set user invoice_enabled", "user_id", userID, "enabled", enabled)
	return nil
}

// GetUserInvoiceEnabled 读单用户 invoice_enabled 字段（管理员后台用）。
func (s *InvoiceService) GetUserInvoiceEnabled(ctx context.Context, userID int64) (bool, error) {
	u, err := s.entClient.User.Query().
		Where(user.IDEQ(userID)).
		Select(user.FieldInvoiceEnabled).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return false, infraerrors.NotFound("USER_NOT_FOUND", "user not found")
		}
		return false, fmt.Errorf("query user invoice flag: %w", err)
	}
	return u.InvoiceEnabled, nil
}

// --- 配置访问 ---

func (s *InvoiceService) windowDays() int {
	d := s.cfg.Invoice.WindowDays
	if d <= 0 {
		return 180
	}
	return d
}

func (s *InvoiceService) maxOrders() int {
	m := s.cfg.Invoice.MaxOrders
	if m <= 0 {
		return 50
	}
	return m
}

// PDFMaxBytes 单文件 PDF 上限（handler 校验时用）
func (s *InvoiceService) PDFMaxBytes() int64 {
	if s.cfg.Invoice.PDFMaxBytes <= 0 {
		return 8 * 1024 * 1024
	}
	return s.cfg.Invoice.PDFMaxBytes
}

// --- 用户侧方法 ---

// ListEligibleOrders 列出半年内可开票订单。
//   - status = COMPLETED
//   - paid_at >= now() - WindowDays
//   - refund_amount = 0
//   - invoice_status = '' （未被任何活跃发票占用）
func (s *InvoiceService) ListEligibleOrders(ctx context.Context, userID int64) ([]EligibleOrder, error) {
	if userID <= 0 {
		return nil, ErrInvoiceForbidden
	}
	cutoff := time.Now().Add(-time.Duration(s.windowDays()) * 24 * time.Hour)
	rows, err := s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.UserIDEQ(userID),
			paymentorder.StatusEQ(payment.OrderStatusCompleted),
			paymentorder.PaidAtGTE(cutoff),
			paymentorder.RefundAmountEQ(0),
			paymentorder.InvoiceStatusEQ(""),
		).
		Order(dbent.Desc(paymentorder.FieldPaidAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list eligible orders: %w", err)
	}
	out := make([]EligibleOrder, 0, len(rows))
	for _, o := range rows {
		var paidAt time.Time
		if o.PaidAt != nil {
			paidAt = *o.PaidAt
		}
		out = append(out, EligibleOrder{
			ID:          o.ID,
			OrderNo:     o.OutTradeNo,
			ProductName: orderProductName(o),
			OrderType:   o.OrderType,
			PayAmount:   o.PayAmount,
			PaidAt:      paidAt,
		})
	}
	return out, nil
}

// CreateApplication 创建发票申请。事务内：
//  1. 校验入参
//  2. 锁订单行（FOR UPDATE 由 ent 表达为 ForUpdate）
//  3. 校验每个订单仍可开（COMPLETED + 未占用 + 未退款 + 在窗口内 + 属于本人）
//  4. 重算 amount = SUM(pay_amount)
//  5. INSERT invoices + invoice_items
//  6. UPDATE payment_orders.invoice_status='pending', invoice_id=newInvoiceID
//
// 唯一索引兜底：并发时仅一者成功，另一者得 ORDER_ALREADY_USED。
func (s *InvoiceService) CreateApplication(ctx context.Context, userID int64, req CreateInvoiceRequest) (*InvoiceDetailDTO, error) {
	if userID <= 0 {
		return nil, ErrInvoiceForbidden
	}

	titleType := strings.TrimSpace(req.TitleType)
	if titleType != InvoiceTitleTypePersonal && titleType != InvoiceTitleTypeBusiness {
		return nil, ErrInvoiceInvalidTitleType
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, ErrInvoiceTitleRequired
	}
	taxNo := strings.TrimSpace(req.TaxNo)
	if titleType == InvoiceTitleTypeBusiness && taxNo == "" {
		return nil, ErrInvoiceTaxNoRequired
	}
	if titleType == InvoiceTitleTypePersonal {
		taxNo = ""
	}
	if len(req.OrderIDs) == 0 {
		return nil, ErrInvoiceNoOrders
	}
	if len(req.OrderIDs) > s.maxOrders() {
		return nil, ErrInvoiceTooManyOrders
	}

	// 去重 order_ids
	seen := make(map[int64]struct{}, len(req.OrderIDs))
	orderIDs := make([]int64, 0, len(req.OrderIDs))
	for _, id := range req.OrderIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		orderIDs = append(orderIDs, id)
	}

	cutoff := time.Now().Add(-time.Duration(s.windowDays()) * 24 * time.Hour)

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin invoice tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 锁定 + 校验订单
	orders, err := tx.PaymentOrder.Query().
		Where(paymentorder.IDIn(orderIDs...)).
		ForUpdate().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock orders: %w", err)
	}
	if len(orders) != len(orderIDs) {
		return nil, ErrInvoiceOrderIneligible
	}
	var amount float64
	var userEmail string
	for _, o := range orders {
		if o.UserID != userID {
			return nil, ErrInvoiceForbidden
		}
		if o.Status != payment.OrderStatusCompleted {
			return nil, ErrInvoiceOrderIneligible
		}
		if o.PaidAt == nil || o.PaidAt.Before(cutoff) {
			return nil, ErrInvoiceOrderIneligible
		}
		if o.RefundAmount > 0 {
			return nil, ErrInvoiceOrderIneligible
		}
		if o.InvoiceStatus != "" {
			return nil, ErrInvoiceOrderAlreadyUsed
		}
		amount += o.PayAmount
		if userEmail == "" {
			userEmail = o.UserEmail
		}
	}
	if amount <= 0 {
		return nil, ErrInvoiceInvalidAmount
	}

	now := time.Now()
	contactEmail := strings.TrimSpace(req.ContactEmail)
	if contactEmail != "" && !looksLikeEmail(contactEmail) {
		return nil, ErrInvoiceContactEmail
	}
	notes := strings.TrimSpace(req.Notes)
	if len(notes) > maxInvoiceNotesLen {
		return nil, ErrInvoiceNotesTooLong
	}

	inv, err := tx.Invoice.Create().
		SetUserID(userID).
		SetUserEmail(userEmail).
		SetTitleType(titleType).
		SetTitle(title).
		SetTaxNo(taxNo).
		SetContactEmail(contactEmail).
		SetAmount(amount).
		SetCurrency("CNY").
		SetNotes(notes).
		SetStatus(InvoiceStatusPending).
		SetSubmittedAt(now).
		SetProvider("manual").
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create invoice: %w", err)
	}

	// 创建 items + 标记订单
	for _, o := range orders {
		paidAt := time.Time{}
		if o.PaidAt != nil {
			paidAt = *o.PaidAt
		}
		_, err := tx.InvoiceItem.Create().
			SetInvoiceID(inv.ID).
			SetPaymentOrderID(o.ID).
			SetOrderNo(o.OutTradeNo).
			SetProductName(orderProductName(o)).
			SetOrderType(o.OrderType).
			SetPayAmount(o.PayAmount).
			SetPaidAt(paidAt).
			Save(ctx)
		if err != nil {
			if isInvoiceUniqueViolation(err) {
				return nil, ErrInvoiceOrderAlreadyUsed
			}
			return nil, fmt.Errorf("create invoice item: %w", err)
		}
		if _, err := tx.PaymentOrder.UpdateOneID(o.ID).
			SetInvoiceStatus(orderInvoiceStatusPending).
			SetInvoiceID(inv.ID).
			Save(ctx); err != nil {
			return nil, fmt.Errorf("mark order with invoice: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit invoice tx: %w", err)
	}
	s.logTransition(inv.ID, userID, "", InvoiceStatusPending, userID, fmt.Sprintf("create amount=%.2f orders=%d", amount, len(orders)))

	return s.GetMyInvoiceDetail(ctx, userID, inv.ID)
}

// ListMyInvoices 用户自己的发票列表（带分页）。
func (s *InvoiceService) ListMyInvoices(ctx context.Context, userID int64, filter InvoiceListFilter) (*PaginatedInvoices, error) {
	if userID <= 0 {
		return nil, ErrInvoiceForbidden
	}
	q := s.entClient.Invoice.Query().Where(invoice.UserIDEQ(userID))
	if status := strings.TrimSpace(filter.Status); status != "" && status != "all" {
		q = q.Where(invoice.StatusEQ(status))
	}
	return s.paginateInvoices(ctx, q, filter.Page, filter.PageSize)
}

// GetMyInvoiceDetail 用户查看自己的发票详情。
func (s *InvoiceService) GetMyInvoiceDetail(ctx context.Context, userID int64, invoiceID int64) (*InvoiceDetailDTO, error) {
	if userID <= 0 {
		return nil, ErrInvoiceForbidden
	}
	inv, err := s.entClient.Invoice.Query().
		Where(invoice.IDEQ(invoiceID)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	if inv.UserID != userID {
		return nil, ErrInvoiceForbidden
	}
	return s.buildDetailDTO(ctx, inv)
}

// CancelByUser 用户取消（仅 pending）。
func (s *InvoiceService) CancelByUser(ctx context.Context, userID int64, invoiceID int64) error {
	if userID <= 0 {
		return ErrInvoiceForbidden
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inv, err := tx.Invoice.Query().Where(invoice.IDEQ(invoiceID)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return fmt.Errorf("lock invoice: %w", err)
	}
	if inv.UserID != userID {
		return ErrInvoiceForbidden
	}
	if inv.Status != InvoiceStatusPending {
		return ErrInvoiceNotPending
	}

	if _, err := tx.Invoice.UpdateOneID(inv.ID).
		SetStatus(InvoiceStatusVoided).
		SetVoidedBy(userID).
		SetReviewNotes("用户取消").
		Save(ctx); err != nil {
		return fmt.Errorf("mark invoice voided: %w", err)
	}
	if err := releaseInvoiceOrders(ctx, tx, inv.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.logTransition(inv.ID, inv.UserID, inv.Status, InvoiceStatusVoided, userID, "user_cancel")
	return nil
}

// GetPDFForUser 用户下载 PDF（必须 issued + 是本人）。
func (s *InvoiceService) GetPDFForUser(ctx context.Context, userID int64, invoiceID int64) (*InvoicePDFContent, error) {
	inv, err := s.entClient.Invoice.Query().Where(invoice.IDEQ(invoiceID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	if inv.UserID != userID {
		return nil, ErrInvoiceForbidden
	}
	return s.openInvoicePDF(ctx, inv)
}

// --- 管理员侧方法 ---

// AdminListInvoices 管理员查看全部发票（分页 + 筛选）。
func (s *InvoiceService) AdminListInvoices(ctx context.Context, filter AdminInvoiceListFilter) (*PaginatedInvoices, error) {
	q := s.entClient.Invoice.Query()
	if status := strings.TrimSpace(filter.Status); status != "" && status != "all" {
		q = q.Where(invoice.StatusEQ(status))
	}
	if filter.UserID > 0 {
		q = q.Where(invoice.UserIDEQ(filter.UserID))
	}
	if email := strings.TrimSpace(filter.Email); email != "" {
		q = q.Where(invoice.UserEmailContainsFold(email))
	}
	return s.paginateInvoices(ctx, q, filter.Page, filter.PageSize)
}

// AdminGetDetail 管理员查看任意发票详情。
func (s *InvoiceService) AdminGetDetail(ctx context.Context, invoiceID int64) (*InvoiceDetailDTO, error) {
	inv, err := s.entClient.Invoice.Query().Where(invoice.IDEQ(invoiceID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	return s.buildDetailDTO(ctx, inv)
}

// GetAdminPDF 管理员下载 PDF（不限状态，便于审核校对）。
func (s *InvoiceService) GetAdminPDF(ctx context.Context, invoiceID int64) (*InvoicePDFContent, error) {
	inv, err := s.entClient.Invoice.Query().Where(invoice.IDEQ(invoiceID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	return s.openInvoicePDF(ctx, inv)
}

// AdminApprove 通过审核（pending → approved）。
func (s *InvoiceService) AdminApprove(ctx context.Context, adminID int64, invoiceID int64, notes string) error {
	return s.adminTransitionFromPending(ctx, adminID, invoiceID, InvoiceStatusApproved, notes, false)
}

// AdminReject 驳回（pending → rejected）+ 释放订单。
func (s *InvoiceService) AdminReject(ctx context.Context, adminID int64, invoiceID int64, reason string) error {
	r := strings.TrimSpace(reason)
	if r == "" {
		return infraerrors.BadRequest("INVOICE_REJECT_REASON_REQUIRED", "rejection reason is required")
	}
	if len(r) > maxInvoiceReasonLen {
		return ErrInvoiceReasonTooLong
	}
	return s.adminTransitionFromPending(ctx, adminID, invoiceID, InvoiceStatusRejected, r, true)
}

// adminTransitionFromPending 通用辅助：pending → approved/rejected，rejected 时释放订单
func (s *InvoiceService) adminTransitionFromPending(ctx context.Context, adminID int64, invoiceID int64, target string, notes string, releaseOrders bool) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inv, err := tx.Invoice.Query().Where(invoice.IDEQ(invoiceID)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return fmt.Errorf("lock invoice: %w", err)
	}
	if inv.Status != InvoiceStatusPending {
		if target == InvoiceStatusApproved {
			return ErrInvoiceNotApprovable
		}
		return ErrInvoiceNotPending
	}

	now := time.Now()
	upd := tx.Invoice.UpdateOneID(inv.ID).
		SetStatus(target).
		SetReviewedAt(now).
		SetReviewedBy(adminID).
		SetReviewNotes(strings.TrimSpace(notes))
	if _, err := upd.Save(ctx); err != nil {
		return fmt.Errorf("update invoice: %w", err)
	}
	if releaseOrders {
		if err := releaseInvoiceOrders(ctx, tx, inv.ID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.logTransition(inv.ID, inv.UserID, inv.Status, target, adminID, notes)
	return nil
}

// AdminUploadPDF 上传 PDF + 标记 issued。
//
// 允许的状态：approved（首次上传）或 issued（替换上传）。
// invoiceNo 可选；不为空则同时落库覆盖。
func (s *InvoiceService) AdminUploadPDF(ctx context.Context, adminID int64, invoiceID int64, src io.Reader, originalName string, invoiceNo string) error {
	if s.pdfStore == nil {
		return infraerrors.InternalServer("INVOICE_PDF_STORE_UNAVAILABLE", "pdf store not configured")
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inv, err := tx.Invoice.Query().Where(invoice.IDEQ(invoiceID)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return fmt.Errorf("lock invoice: %w", err)
	}
	if inv.Status != InvoiceStatusApproved && inv.Status != InvoiceStatusIssued {
		return ErrInvoiceNotIssuable
	}

	// 写入新文件（事务外操作 — 失败不污染 DB；DB 提交失败时孤立文件由后续 GC 处理）
	keyOld := inv.PdfPath
	storageOld := inv.PdfStorage
	key, size, err := s.pdfStore.Put(ctx, inv.ID, src)
	if err != nil {
		return fmt.Errorf("put pdf: %w", err)
	}
	now := time.Now()

	upd := tx.Invoice.UpdateOneID(inv.ID).
		SetStatus(InvoiceStatusIssued).
		SetIssuedAt(now).
		SetReviewedAt(now).
		SetReviewedBy(adminID).
		SetPdfPath(key).
		SetPdfStorage(s.pdfStore.Storage()).
		SetPdfSize(size).
		SetPdfOriginalName(strings.TrimSpace(originalName))
	if no := strings.TrimSpace(invoiceNo); no != "" {
		upd = upd.SetInvoiceNo(no)
	} else if inv.InvoiceNo == "" {
		upd = upd.SetInvoiceNo(generateInvoiceNo(inv.ID, now))
	}
	if _, err := upd.Save(ctx); err != nil {
		_ = s.pdfStore.Delete(ctx, key)
		return fmt.Errorf("update invoice with pdf: %w", err)
	}
	// 标记订单为 issued
	if _, err := tx.PaymentOrder.Update().
		Where(paymentorder.InvoiceIDEQ(inv.ID)).
		SetInvoiceStatus(orderInvoiceStatusIssued).
		Save(ctx); err != nil {
		_ = s.pdfStore.Delete(ctx, key)
		return fmt.Errorf("mark orders issued: %w", err)
	}
	if err := tx.Commit(); err != nil {
		_ = s.pdfStore.Delete(ctx, key)
		return fmt.Errorf("commit: %w", err)
	}
	s.logTransition(inv.ID, inv.UserID, inv.Status, InvoiceStatusIssued, adminID, "upload_pdf")

	// 提交成功后清理旧文件（替换上传场景）
	if keyOld != "" && keyOld != key && storageOld == s.pdfStore.Storage() {
		if err := s.pdfStore.Delete(ctx, keyOld); err != nil {
			s.log.Warn("delete old invoice pdf failed", "invoice_id", inv.ID, "key", keyOld, "err", err)
		}
	}
	return nil
}

// AdminMarkIssued 备用：无 PDF 上传场景下，仅标记 issued + 写发票号。
func (s *InvoiceService) AdminMarkIssued(ctx context.Context, adminID int64, invoiceID int64, invoiceNo string) error {
	no := strings.TrimSpace(invoiceNo)
	if no == "" {
		return ErrInvoiceInvoiceNoRequired
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inv, err := tx.Invoice.Query().Where(invoice.IDEQ(invoiceID)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return fmt.Errorf("lock invoice: %w", err)
	}
	if inv.Status != InvoiceStatusApproved {
		return ErrInvoiceNotIssuable
	}
	now := time.Now()
	if _, err := tx.Invoice.UpdateOneID(inv.ID).
		SetStatus(InvoiceStatusIssued).
		SetIssuedAt(now).
		SetReviewedAt(now).
		SetReviewedBy(adminID).
		SetInvoiceNo(no).
		Save(ctx); err != nil {
		return fmt.Errorf("mark issued: %w", err)
	}
	if _, err := tx.PaymentOrder.Update().
		Where(paymentorder.InvoiceIDEQ(inv.ID)).
		SetInvoiceStatus(orderInvoiceStatusIssued).
		Save(ctx); err != nil {
		return fmt.Errorf("mark orders issued: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.logTransition(inv.ID, inv.UserID, inv.Status, InvoiceStatusIssued, adminID, "mark_issued no="+no)
	return nil
}

// AdminVoid 作废（approved/issued → voided）+ 释放订单。
// PDF 文件保留作为历史记录，由后续 GC 任务清理。
func (s *InvoiceService) AdminVoid(ctx context.Context, adminID int64, invoiceID int64, reason string) error {
	r := strings.TrimSpace(reason)
	if r == "" {
		return infraerrors.BadRequest("INVOICE_VOID_REASON_REQUIRED", "void reason is required")
	}
	if len(r) > maxInvoiceReasonLen {
		return ErrInvoiceReasonTooLong
	}
	reason = r
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inv, err := tx.Invoice.Query().Where(invoice.IDEQ(invoiceID)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return fmt.Errorf("lock invoice: %w", err)
	}
	if inv.Status != InvoiceStatusApproved && inv.Status != InvoiceStatusIssued {
		return ErrInvoiceNotVoidable
	}
	if _, err := tx.Invoice.UpdateOneID(inv.ID).
		SetStatus(InvoiceStatusVoided).
		SetVoidedBy(adminID).
		SetReviewNotes(reason).
		Save(ctx); err != nil {
		return fmt.Errorf("mark voided: %w", err)
	}
	if err := releaseInvoiceOrders(ctx, tx, inv.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.logTransition(inv.ID, inv.UserID, inv.Status, InvoiceStatusVoided, adminID, reason)
	return nil
}

// IsOrderInvoiceLocked 供 PaymentService.RequestRefund / ProcessRefund 调用，
// 校验订单是否被活跃发票（pending/approved/issued）锁定。
//
// 当 locked=true 时，调用方应返回 ORDER_LOCKED_BY_INVOICE 错误码（前端展示
// 「该订单已开/正在申请发票，如需退款请先联系客服作废发票」）。
//
// 注意：依赖 payment_orders.invoice_status 反规范化字段；该字段由
// InvoiceService 在状态流转时同步维护。
func (s *InvoiceService) IsOrderInvoiceLocked(ctx context.Context, orderID int64) (locked bool, invoiceID int64, status string, err error) {
	if orderID <= 0 {
		return false, 0, "", nil
	}
	o, qerr := s.entClient.PaymentOrder.Query().
		Where(paymentorder.IDEQ(orderID)).
		Select(paymentorder.FieldInvoiceStatus, paymentorder.FieldInvoiceID).
		Only(ctx)
	if qerr != nil {
		if dbent.IsNotFound(qerr) {
			return false, 0, "", nil
		}
		return false, 0, "", fmt.Errorf("query order invoice lock: %w", qerr)
	}
	if o.InvoiceStatus == "" {
		return false, 0, "", nil
	}
	id := int64(0)
	if o.InvoiceID != nil {
		id = *o.InvoiceID
	}
	return true, id, o.InvoiceStatus, nil
}

// --- 内部辅助 ---

// releaseInvoiceOrders 释放该发票占用的订单：
//   - DELETE FROM invoice_items WHERE invoice_id = ?
//   - UPDATE payment_orders SET invoice_status='', invoice_id=NULL WHERE invoice_id = ?
//
// 调用方负责事务提交。
func releaseInvoiceOrders(ctx context.Context, tx *dbent.Tx, invoiceID int64) error {
	if _, err := tx.InvoiceItem.Delete().Where(invoiceitem.InvoiceIDEQ(invoiceID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete invoice items: %w", err)
	}
	if _, err := tx.PaymentOrder.Update().
		Where(paymentorder.InvoiceIDEQ(invoiceID)).
		SetInvoiceStatus("").
		ClearInvoiceID().
		Save(ctx); err != nil {
		return fmt.Errorf("clear order invoice mark: %w", err)
	}
	return nil
}

func (s *InvoiceService) buildDetailDTO(ctx context.Context, inv *dbent.Invoice) (*InvoiceDetailDTO, error) {
	items, err := s.entClient.InvoiceItem.Query().
		Where(invoiceitem.InvoiceIDEQ(inv.ID)).
		Order(dbent.Asc(invoiceitem.FieldPaidAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoice items: %w", err)
	}
	itemDTOs := make([]InvoiceItemDTO, 0, len(items))
	for _, it := range items {
		itemDTOs = append(itemDTOs, InvoiceItemDTO{
			OrderID:     it.PaymentOrderID,
			OrderNo:     it.OrderNo,
			ProductName: it.ProductName,
			OrderType:   it.OrderType,
			PayAmount:   it.PayAmount,
			PaidAt:      it.PaidAt,
		})
	}
	dto := &InvoiceDetailDTO{
		InvoiceDTO:      toInvoiceDTO(inv, len(items)),
		Notes:           inv.Notes,
		ReviewedAt:      inv.ReviewedAt,
		ReviewedBy:      inv.ReviewedBy,
		ReviewNotes:     inv.ReviewNotes,
		IssuedAt:        inv.IssuedAt,
		PDFOriginalName: inv.PdfOriginalName,
		PDFAvailable:    inv.Status == InvoiceStatusIssued && inv.PdfPath != "",
		Provider:        inv.Provider,
		Items:           itemDTOs,
	}
	return dto, nil
}

func (s *InvoiceService) openInvoicePDF(ctx context.Context, inv *dbent.Invoice) (*InvoicePDFContent, error) {
	if s.pdfStore == nil {
		return nil, infraerrors.InternalServer("INVOICE_PDF_STORE_UNAVAILABLE", "pdf store not configured")
	}
	if inv.Status != InvoiceStatusIssued || inv.PdfPath == "" {
		return nil, ErrInvoicePDFMissing
	}
	if inv.PdfStorage != "" && inv.PdfStorage != s.pdfStore.Storage() {
		return nil, infraerrors.InternalServer("INVOICE_PDF_STORAGE_MISMATCH",
			fmt.Sprintf("pdf storage mismatch: stored=%s, current=%s", inv.PdfStorage, s.pdfStore.Storage()))
	}
	rc, err := s.pdfStore.Get(ctx, inv.PdfPath)
	if err != nil {
		return nil, ErrInvoicePDFMissing
	}
	filename := strings.TrimSpace(inv.InvoiceNo)
	if filename == "" {
		filename = fmt.Sprintf("invoice-%d", inv.ID)
	}
	return &InvoicePDFContent{
		Reader:      rc,
		Filename:    filename + ".pdf",
		ContentType: "application/pdf",
		Size:        inv.PdfSize,
	}, nil
}

// paginateInvoices 公共分页（list / admin list 共用）。
func (s *InvoiceService) paginateInvoices(ctx context.Context, q *dbent.InvoiceQuery, page, size int) (*PaginatedInvoices, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count invoices: %w", err)
	}
	rows, err := q.
		Order(dbent.Desc(invoice.FieldSubmittedAt)).
		Limit(size).
		Offset((page - 1) * size).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	if len(rows) == 0 {
		return &PaginatedInvoices{Items: []InvoiceDTO{}, Total: total, Page: page, Size: size}, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	// 一次性取所有 items 计数，避免 N+1
	countByInv, err := invoiceItemCounts(ctx, s.entClient, ids)
	if err != nil {
		return nil, err
	}
	items := make([]InvoiceDTO, 0, len(rows))
	for _, r := range rows {
		items = append(items, toInvoiceDTO(r, countByInv[r.ID]))
	}
	return &PaginatedInvoices{Items: items, Total: total, Page: page, Size: size}, nil
}

func invoiceItemCounts(ctx context.Context, client *dbent.Client, invoiceIDs []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(invoiceIDs))
	if len(invoiceIDs) == 0 {
		return out, nil
	}
	rows, err := client.InvoiceItem.Query().
		Where(invoiceitem.InvoiceIDIn(invoiceIDs...)).
		Select(invoiceitem.FieldInvoiceID).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("count invoice items: %w", err)
	}
	for _, r := range rows {
		out[r.InvoiceID]++
	}
	return out, nil
}

func toInvoiceDTO(inv *dbent.Invoice, orderCount int) InvoiceDTO {
	return InvoiceDTO{
		ID:           inv.ID,
		InvoiceNo:    inv.InvoiceNo,
		UserID:       inv.UserID,
		UserEmail:    inv.UserEmail,
		TitleType:    inv.TitleType,
		Title:        inv.Title,
		TaxNo:        inv.TaxNo,
		Amount:       inv.Amount,
		Currency:     inv.Currency,
		Status:       inv.Status,
		OrderCount:   orderCount,
		SubmittedAt:  inv.SubmittedAt,
		ContactEmail: inv.ContactEmail,
	}
}

// orderProductName 按订单类型生成展示名。
//   - balance: "RMB X 元 余额充值"
//   - subscription: "订阅充值（X 天）"
//
// 当未来订单表里有真正的 product_name 字段时可改为读取该字段。
func orderProductName(o *dbent.PaymentOrder) string {
	switch o.OrderType {
	case payment.OrderTypeSubscription:
		days := 0
		if o.SubscriptionDays != nil {
			days = *o.SubscriptionDays
		}
		if days > 0 {
			return fmt.Sprintf("订阅充值 %d 天", days)
		}
		return "订阅充值"
	default:
		return fmt.Sprintf("余额充值 %.2f 元", o.PayAmount)
	}
}

// logTransition 把发票状态流转写到结构化日志。
//
// 不写到 PaymentAuditLog 表的原因：
// PaymentAuditLog 以单订单 (string) 为键，而一张发票可能跨多个订单，落表会造成
// 重复行或键冲突。后续如有完整审计需求，应另建 invoice_audit_logs 表。
func (s *InvoiceService) logTransition(invoiceID, userID int64, from, to string, actor int64, detail string) {
	s.log.Info("invoice transition",
		"invoice_id", invoiceID,
		"user_id", userID,
		"from", from,
		"to", to,
		"actor", actor,
		"detail", detail,
	)
}

// looksLikeEmail 用 net/mail 做一次轻量校验。仅用于挡掉明显畸形输入，
// 不保证投递可达（投递失败由发邮件链路处理）。
func looksLikeEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	// ParseAddress 会接受类似 "User <a@b>" 的格式，限制只接受裸地址
	return addr.Address == s
}

// generateInvoiceNo 在管理员未填发票号时生成一个占位号。
// 真实业务里建议管理员手填发票号；该函数仅作为兜底。
func generateInvoiceNo(invoiceID int64, t time.Time) string {
	return fmt.Sprintf("INV%s%d", t.Format("20060102"), invoiceID)
}

// isInvoiceUniqueViolation 判断是否为 PostgreSQL 唯一索引冲突（23505）。
//
// 注：与 referral_service.go 中的 isUniqueViolation 函数功能等价；
// 命名加 Invoice 前缀避免符号重复。
func isInvoiceUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		if pgErr.SQLState() == "23505" {
			return true
		}
	}
	if dbent.IsConstraintError(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate") {
		return true
	}
	return false
}
