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
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/invoice"
	"github.com/Wei-Shaw/sub2api/ent/invoiceitem"
	"github.com/Wei-Shaw/sub2api/ent/invoicevoidrequest"
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

	// 票种（invoice_kind 字段）
	InvoiceKindNormal  = "normal"  // 普票
	InvoiceKindSpecial = "special" // 专票

	// Provider 名称
	InvoiceProviderManual     = "manual"
	InvoiceProviderCaiyuntong = "caiyuntong"

	// provider_state：开票子状态机
	ProviderStateNone    = "none"
	ProviderStateQueued  = "queued"  // 等待 dispatch_worker 拾取
	ProviderStateIssuing = "issuing" // 已提交，等 poll_worker 查询结果
	ProviderStateSuccess = "success" // 蓝票出票成功
	ProviderStateFailed  = "failed"  // 蓝票最终失败

	// provider_state：红冲子状态机
	ProviderStateReversePending = "reverse_pending" // 红冲入队
	ProviderStateReversing      = "reversing"       // 红冲进行中（多步走 reverse_step 推进）
	ProviderStateReverseSuccess = "reverse_success"
	ProviderStateReverseFailed  = "reverse_failed"

	// reverse_step：红冲多步子状态（仅 reversing 时有意义）
	ReverseStepRedApplying  = "red_applying"  // 数电：已申请红字信息单，等确认
	ReverseStepRedConfirmed = "red_confirmed" // 数电：红字信息单已确认，待开红票
	ReverseStepRedIssuing   = "red_issuing"   // 红票已提交，等查询出票
	ReverseStepRedDone      = "red_done"      // 红票出票成功

	// invoice_void_requests.status
	VoidRequestStatusPending  = "pending_review"
	VoidRequestStatusApproved = "approved"
	VoidRequestStatusRejected = "rejected"
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

	// 购方扩展信息（专票必填，普票可空）
	BuyerAddress     string `json:"buyer_address"`
	BuyerPhone       string `json:"buyer_phone"`
	BuyerBankName    string `json:"buyer_bank_name"`
	BuyerBankAccount string `json:"buyer_bank_account"`
}

// InvoiceDTO 列表视图
type InvoiceDTO struct {
	ID            int64  `json:"id"`
	ApplicationNo string `json:"application_no"`
	InvoiceNo     string `json:"invoice_no"`
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

	// v3：开票渠道与子状态机
	Provider          string `json:"provider"`
	ProviderState     string `json:"provider_state"`
	InvoiceKind       string `json:"invoice_kind"`
	ProviderLastError string `json:"provider_last_error,omitempty"`

	// 购方扩展信息（专票场景）
	BuyerAddress     string `json:"buyer_address,omitempty"`
	BuyerPhone       string `json:"buyer_phone,omitempty"`
	BuyerBankName    string `json:"buyer_bank_name,omitempty"`
	BuyerBankAccount string `json:"buyer_bank_account,omitempty"`

	// 挂起的作废申请（仅 admin / 用户自己看到的发票里 inline；nil 表示无挂起申请）
	PendingVoidRequest *PendingVoidRequestInfo `json:"pending_void_request,omitempty"`
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
	Status      string
	UserID      int64
	Email       string
	Page        int
	PageSize    int
	VoidPending bool // 只看「有挂起作废申请」的发票
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
	emailService   *EmailService         // v3：开票成功 / 红冲完成邮件通知（可空，未配置 SMTP 时跳过）
	refundExecutor InvoiceRefundExecutor // v3：红冲完成后驱动实际资金退款
	log            *slog.Logger

	// 异步开票 / 自动红冲 worker 控制（v3）
	workerStop     chan struct{}
	workerStopOnce sync.Once
	workerWG       sync.WaitGroup
	workerStarted  bool
}

func NewInvoiceService(entClient *dbent.Client, pdfStore InvoicePDFStore, cfg *config.Config, settingService *SettingService) *InvoiceService {
	return &InvoiceService{
		entClient:      entClient,
		pdfStore:       pdfStore,
		cfg:            cfg,
		settingService: settingService,
		log:            slog.Default(),
		workerStop:     make(chan struct{}),
	}
}

// SetEmailService 注入邮件服务（在 wire provider 里调用）。
// 设计成可选 setter 而非构造参数，避免 wire 中循环依赖；EmailService 当前已在容器里，
// 直接传给 NewInvoiceService 也行，但 setter 方式跟 TokenRefreshService 设置 RefreshAPI
// 的既有模式一致。
func (s *InvoiceService) SetEmailService(es *EmailService) {
	if s == nil {
		return
	}
	s.emailService = es
}

// InvoiceRefundExecutor 是 InvoiceService 用来在红冲成功后驱动实际资金退款的钩子。
// 由 PaymentService 实现，避免 service 包内 InvoiceService 直接持有 *PaymentService
// 引用而带来循环依赖风险（虽然同包，但语义解耦）。
type InvoiceRefundExecutor interface {
	// FinalizeRefundAfterReverse 在红冲成功后执行实际的资金退款。
	// refundRequestID 关联 refund_requests 表，由实现方负责状态推进与审计日志。
	FinalizeRefundAfterReverse(ctx context.Context, invoiceID int64) error
}

// SetRefundExecutor 注入退款执行器（v3 红冲完成后驱动退款）。
func (s *InvoiceService) SetRefundExecutor(ex InvoiceRefundExecutor) {
	if s == nil {
		return
	}
	s.refundExecutor = ex
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
		SetBuyerAddress(strings.TrimSpace(req.BuyerAddress)).
		SetBuyerPhone(strings.TrimSpace(req.BuyerPhone)).
		SetBuyerBankName(strings.TrimSpace(req.BuyerBankName)).
		SetBuyerBankAccount(strings.TrimSpace(req.BuyerBankAccount)).
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

	// 回填申请单号 APP-YYYYMMDD-{id 6 位补零}，给 UI 一个在 invoice_no 落地前就稳定可见的标识
	applicationNo := fmt.Sprintf("APP-%s-%06d", now.Format("20060102"), inv.ID)
	if _, err := tx.Invoice.UpdateOneID(inv.ID).SetApplicationNo(applicationNo).Save(ctx); err != nil {
		return nil, fmt.Errorf("set application_no: %w", err)
	}
	inv.ApplicationNo = applicationNo

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
	if filter.VoidPending {
		// 子查询：有挂起作废申请的 invoice_id 集合
		invoiceIDs, err := s.entClient.InvoiceVoidRequest.Query().
			Where(invoicevoidrequest.StatusEQ(VoidRequestStatusPending)).
			Select(invoicevoidrequest.FieldInvoiceID).
			Ints(ctx)
		if err != nil {
			return nil, fmt.Errorf("list pending void invoice ids: %w", err)
		}
		idList := make([]int64, 0, len(invoiceIDs))
		for _, id := range invoiceIDs {
			idList = append(idList, int64(id))
		}
		if len(idList) == 0 {
			// 没挂起申请直接返回空结果
			return &PaginatedInvoices{Items: []InvoiceDTO{}, Total: 0, Page: filter.Page, Size: filter.PageSize}, nil
		}
		q = q.Where(invoice.IDIn(idList...))
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

// AdminApproveParams 审批参数（v3）。
//
// InvoiceKind / Provider 为审批阶段的二次决策，前端在弹窗里收集。
// 留空时回退到当前 invoice 字段值或 SettingService 全局默认。
type AdminApproveParams struct {
	AdminID         int64
	InvoiceID       int64
	Notes           string
	InvoiceKind     string // normal | special；空字符串保留申请单原值，再 fallback normal
	Provider        string // manual | caiyuntong；空字符串走系统默认
	InvoiceTypeCode string // 可选；空时由 Provider 配置 + InvoiceKind 推导
}

// AdminApprove 通过审核（pending → approved）。
//
// 当所选 provider 为自动开票渠道时，会在事务内把 provider_state 置 "queued"，
// 由 invoice_dispatch_worker 异步拾取并调 Provider.Issue()。
// provider == "manual" 时维持现状（管理员手动上传 PDF）。
func (s *InvoiceService) AdminApprove(ctx context.Context, params AdminApproveParams) error {
	provider := strings.TrimSpace(params.Provider)
	if provider == "" {
		provider = InvoiceProviderManual
	}
	kind := strings.TrimSpace(params.InvoiceKind)
	if kind != InvoiceKindNormal && kind != InvoiceKindSpecial {
		kind = InvoiceKindNormal
	}
	return s.adminApproveWith(ctx, params.AdminID, params.InvoiceID, params.Notes, kind, provider, strings.TrimSpace(params.InvoiceTypeCode))
}

// AdminRetryIssue 把 failed 的发票重新放回 queued，等 dispatch worker 再试一次。
// 适用场景：第三方暂时性故障（鉴权失败、网络抖动）解决后管理员手动恢复。
func (s *InvoiceService) AdminRetryIssue(ctx context.Context, invoiceID int64) error {
	inv, err := s.entClient.Invoice.Get(ctx, invoiceID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return err
	}
	if inv.ProviderState != ProviderStateFailed {
		return infraerrors.Conflict("INVOICE_NOT_RETRYABLE", "only failed invoices can be retried")
	}
	if inv.Status != InvoiceStatusApproved {
		return infraerrors.Conflict(
			"INVOICE_NOT_APPROVED_FOR_RETRY",
			"invoice has already been "+inv.Status+"; cannot retry. Ask the user to resubmit a new invoice request.",
		)
	}
	_, err = s.entClient.Invoice.UpdateOneID(invoiceID).
		SetProviderState(ProviderStateQueued).
		SetProviderRetryCount(0).
		SetProviderLastError("").
		Save(ctx)
	return err
}

// AdminRetryReverse 把 reverse_failed 的发票重新置 reverse_pending，重启红冲流程。
func (s *InvoiceService) AdminRetryReverse(ctx context.Context, invoiceID int64) error {
	inv, err := s.entClient.Invoice.Get(ctx, invoiceID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return err
	}
	if inv.ProviderState != ProviderStateReverseFailed {
		return infraerrors.Conflict("INVOICE_NOT_REVERSE_RETRYABLE", "only reverse_failed invoices can be retried")
	}
	_, err = s.entClient.Invoice.UpdateOneID(invoiceID).
		SetProviderState(ProviderStateReversePending).
		SetReverseStep("").
		SetProviderRetryCount(0).
		SetProviderLastError("").
		Save(ctx)
	return err
}

// AdminMarkReversed 管理员标记「已在第三方平台手工红冲」。
// 适用场景：自动红冲反复失败，管理员到财云通后台手动开了红票回来兜底。
//
// 直接把发票 status 置为 voided + 释放订单，跳过资金退款（资金退款由管理员独立操作）。
func (s *InvoiceService) AdminMarkReversed(ctx context.Context, adminID int64, invoiceID int64, redInvoiceNo string) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	inv, err := tx.Invoice.Query().Where(invoice.IDEQ(invoiceID)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return err
	}
	if inv.Status != InvoiceStatusIssued {
		return infraerrors.Conflict("INVOICE_NOT_ISSUED", "only issued invoices can be marked as reversed")
	}
	upd := tx.Invoice.UpdateOneID(inv.ID).
		SetStatus(InvoiceStatusVoided).
		SetProviderState(ProviderStateReverseSuccess).
		SetVoidedBy(adminID).
		SetReverseStep(ReverseStepRedDone)
	if strings.TrimSpace(redInvoiceNo) != "" {
		upd = upd.SetRedInvoiceNo(strings.TrimSpace(redInvoiceNo))
	}
	if _, err := upd.Save(ctx); err != nil {
		return err
	}
	if err := releaseInvoiceOrders(ctx, tx, inv.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// adminApproveWith pending → approved 内部实现：写入 kind/provider/typeCode 并按需投递 issue job。
func (s *InvoiceService) adminApproveWith(ctx context.Context, adminID, invoiceID int64, notes, kind, provider, invoiceTypeCode string) error {
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
		return ErrInvoiceNotApprovable
	}

	// 专票必填校验：税号 + 地址 + 电话 + 开户行 + 银行账号（财云通强校验，
	// 缺一会被平台拒「请求参数异常：发票号码 invoiceNumber 为空」）
	if kind == InvoiceKindSpecial {
		var missing []string
		if strings.TrimSpace(inv.TaxNo) == "" {
			missing = append(missing, "税号")
		}
		if strings.TrimSpace(inv.BuyerAddress) == "" {
			missing = append(missing, "地址")
		}
		if strings.TrimSpace(inv.BuyerPhone) == "" {
			missing = append(missing, "电话")
		}
		if strings.TrimSpace(inv.BuyerBankName) == "" {
			missing = append(missing, "开户行")
		}
		if strings.TrimSpace(inv.BuyerBankAccount) == "" {
			missing = append(missing, "银行账号")
		}
		if len(missing) > 0 {
			return infraerrors.BadRequest(
				"INVOICE_SPECIAL_FIELDS_MISSING",
				"专票必填购方信息不完整："+strings.Join(missing, "、")+"。请改选普票，或让用户补全后重新申请。",
			)
		}
	}

	now := time.Now()
	upd := tx.Invoice.UpdateOneID(inv.ID).
		SetStatus(InvoiceStatusApproved).
		SetReviewedAt(now).
		SetReviewedBy(adminID).
		SetReviewNotes(strings.TrimSpace(notes)).
		SetInvoiceKind(kind).
		SetProvider(provider)
	if invoiceTypeCode != "" {
		upd = upd.SetInvoiceTypeCode(invoiceTypeCode)
	}
	if provider != InvoiceProviderManual {
		// 入队等待异步开票
		upd = upd.SetProviderState(ProviderStateQueued).
			SetProviderRetryCount(0).
			SetProviderLastError("")
	}
	if _, err := upd.Save(ctx); err != nil {
		return fmt.Errorf("update invoice: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.logTransition(inv.ID, inv.UserID, inv.Status, InvoiceStatusApproved, adminID, notes)
	if provider != InvoiceProviderManual {
		slog.Info("invoice_approved_auto_issue_queued",
			"invoice_id", inv.ID,
			"provider", provider,
			"kind", kind,
			"invoice_type_code", invoiceTypeCode,
		)
	}
	return nil
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

// AdminVoid 作废发票。
//
// 行为按当前状态 + 渠道分两条路径：
//
//  1. **issued + 自动渠道（如 caiyuntong）**：转入红冲流水线。
//     invoice.status 保持 issued，provider_state 置 reverse_pending，
//     reverse_worker 调财云通红字接口 + 开红票，成功后才把 status 标 voided
//     并释放订单。这是「真红冲」—— 财云通侧也会被冲掉，税务上一致。
//
//  2. **approved 或 manual 渠道**：维持原"本地直接 voided"行为（紧急兜底）。
//     approved 状态尚未在第三方平台出票；manual 渠道没有自动红冲能力，
//     管理员需自行到对方平台同步处理。
//
// adminID 写入 voided_by 字段，便于审计。
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

	// 分支 1：issued + 自动渠道 → 触发红冲，不立即 voided
	if inv.Status == InvoiceStatusIssued && inv.Provider != "" && inv.Provider != InvoiceProviderManual {
		// 拒绝重复触发
		if inv.ProviderState == ProviderStateReversePending || inv.ProviderState == ProviderStateReversing {
			return infraerrors.Conflict("INVOICE_REVERSE_IN_PROGRESS", "reverse already in progress; please wait")
		}
		if _, err := tx.Invoice.UpdateOneID(inv.ID).
			SetProviderState(ProviderStateReversePending).
			SetReverseStep("").
			SetVoidedBy(adminID).
			SetReviewNotes(reason).
			SetProviderRetryCount(0).
			SetProviderLastError("").
			Save(ctx); err != nil {
			return fmt.Errorf("mark reverse_pending: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		s.log.Info("invoice_void_triggered_reverse",
			"invoice_id", inv.ID,
			"provider", inv.Provider,
			"actor_admin", adminID,
			"reason", reason,
		)
		return nil
	}

	// 分支 2：本地直接 voided（approved 或 manual 渠道）
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

// --- 作废申请（用户提报 → admin 审批 → 复用 AdminVoid 红冲流水线）---

// PendingVoidRequestInfo 嵌套到 InvoiceDTO 里，让 admin 在发票列表直接看到挂起的作废申请。
type PendingVoidRequestInfo struct {
	ID          int64     `json:"id"`
	Reason      string    `json:"reason"`
	RequestedAt time.Time `json:"requested_at"`
}

// VoidRequestDTO 列表 / 详情视图。
type VoidRequestDTO struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	InvoiceID  int64      `json:"invoice_id"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason"`
	AdminID    *int64     `json:"admin_id,omitempty"`
	AdminNotes string     `json:"admin_notes,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

var (
	ErrVoidRequestDuplicate    = infraerrors.Conflict("INVOICE_VOID_REQUEST_DUPLICATE", "该发票已有待审批的作废申请")
	ErrVoidRequestInvalidState = infraerrors.Conflict("INVOICE_VOID_REQUEST_INVALID_STATE", "仅已开具的发票可申请作废")
	ErrVoidRequestNotFound     = infraerrors.NotFound("INVOICE_VOID_REQUEST_NOT_FOUND", "作废申请不存在")
	ErrVoidRequestNotPending   = infraerrors.Conflict("INVOICE_VOID_REQUEST_NOT_PENDING", "作废申请已处理，不可重复操作")
	ErrVoidRequestManualOnly   = infraerrors.BadRequest("INVOICE_VOID_REQUEST_MANUAL_CHANNEL", "手工开票的发票需联系客服线下作废")
)

// CreateVoidRequest 用户对 issued 发票发起作废申请。
//
// 约束：
//   - invoice 必须属于该用户
//   - invoice.status = issued
//   - invoice.provider != manual（manual 渠道无法自动红冲）
//   - 同 invoice 同时只能有 1 条 pending_review 记录
func (s *InvoiceService) CreateVoidRequest(ctx context.Context, userID, invoiceID int64, reason string) (*VoidRequestDTO, error) {
	if userID <= 0 {
		return nil, ErrInvoiceForbidden
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	inv, err := tx.Invoice.Query().Where(invoice.IDEQ(invoiceID)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrInvoiceNotFound
		}
		return nil, fmt.Errorf("lock invoice: %w", err)
	}
	if inv.UserID != userID {
		return nil, ErrInvoiceForbidden
	}
	if inv.Status != InvoiceStatusIssued {
		return nil, ErrVoidRequestInvalidState
	}
	if inv.Provider == "" || inv.Provider == InvoiceProviderManual {
		return nil, ErrVoidRequestManualOnly
	}

	// 同 invoice 只允许一条挂起申请
	exists, err := tx.InvoiceVoidRequest.Query().
		Where(
			invoicevoidrequest.InvoiceIDEQ(invoiceID),
			invoicevoidrequest.StatusEQ(VoidRequestStatusPending),
		).Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("check duplicate void request: %w", err)
	}
	if exists {
		return nil, ErrVoidRequestDuplicate
	}

	row, err := tx.InvoiceVoidRequest.Create().
		SetUserID(userID).
		SetInvoiceID(invoiceID).
		SetStatus(VoidRequestStatusPending).
		SetReason(strings.TrimSpace(reason)).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create void request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.log.Info("invoice_void_request_created",
		"request_id", row.ID,
		"invoice_id", invoiceID,
		"user_id", userID,
	)
	return toVoidRequestDTO(row), nil
}

// ListMyPendingVoidRequest 用户查询某发票当前是否有挂起的作废申请（用于 UI 决定显示「申请作废」还是「已提交」徽标）。
// 返回 nil 表示无挂起申请。
func (s *InvoiceService) ListMyPendingVoidRequest(ctx context.Context, userID, invoiceID int64) (*VoidRequestDTO, error) {
	row, err := s.entClient.InvoiceVoidRequest.Query().
		Where(
			invoicevoidrequest.UserIDEQ(userID),
			invoicevoidrequest.InvoiceIDEQ(invoiceID),
			invoicevoidrequest.StatusEQ(VoidRequestStatusPending),
		).
		First(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return toVoidRequestDTO(row), nil
}

// AdminApproveVoidRequest 管理员通过作废申请，调用 AdminVoid 触发红冲。
//
// 时序：
//  1. 开事务，ForUpdate 锁 void_request + invoice，验证 request=pending、invoice=issued、
//     且 provider_state 不在 reverse_* 中间态（防止 admin 已经手动作废 / 红冲在跑）
//  2. 在同事务内把 void_request 标记为 approved → 提交
//  3. 提交后调 AdminVoid（它自己开事务）。失败时回滚 void_request 到 pending，便于重试
//
// 用独立 tx 推进状态而不嵌套 AdminVoid 的事务，避免 ent client 复用 + 长事务持有锁。
// 短临界区只校验竞态、抢占 pending 状态，把潜在阻塞的 AdminVoid 留在事务外。
func (s *InvoiceService) AdminApproveVoidRequest(ctx context.Context, adminID, requestID int64, notes string) error {
	if adminID <= 0 {
		return ErrInvoiceForbidden
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	req, err := tx.InvoiceVoidRequest.Query().
		Where(invoicevoidrequest.IDEQ(requestID)).
		ForUpdate().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrVoidRequestNotFound
		}
		return fmt.Errorf("lock void request: %w", err)
	}
	if req.Status != VoidRequestStatusPending {
		return ErrVoidRequestNotPending
	}

	// 再锁 invoice 校验状态，防止 admin 已通过其他入口作废 / 已经在红冲中
	inv, err := tx.Invoice.Query().Where(invoice.IDEQ(req.InvoiceID)).ForUpdate().Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrInvoiceNotFound
		}
		return fmt.Errorf("lock invoice: %w", err)
	}
	if inv.Status != InvoiceStatusIssued {
		return ErrVoidRequestInvalidState
	}
	switch inv.ProviderState {
	case ProviderStateReversePending, ProviderStateReversing,
		ProviderStateReverseSuccess, ProviderStateReverseFailed:
		return ErrVoidRequestInvalidState
	}

	now := time.Now()
	if _, err := tx.InvoiceVoidRequest.UpdateOneID(req.ID).
		SetStatus(VoidRequestStatusApproved).
		SetAdminID(adminID).
		SetAdminNotes(strings.TrimSpace(notes)).
		SetReviewedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("mark void request approved: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit void request approval: %w", err)
	}
	committed = true

	// 触发 AdminVoid：issued + 自动渠道时会自动转红冲；理由用用户的 reason
	voidReason := strings.TrimSpace(req.Reason)
	if voidReason == "" {
		voidReason = "用户申请作废"
	}
	if err := s.AdminVoid(ctx, adminID, req.InvoiceID, voidReason); err != nil {
		// AdminVoid 失败时回滚 void_request 状态便于重试。
		// 用 context.Background() 避免请求 ctx 已经被取消导致回滚失败。
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, rbErr := s.entClient.InvoiceVoidRequest.UpdateOneID(req.ID).
			SetStatus(VoidRequestStatusPending).
			ClearAdminID().
			ClearReviewedAt().
			Save(rollbackCtx); rbErr != nil {
			s.log.Error("void_request_rollback_failed",
				"request_id", req.ID,
				"invoice_id", req.InvoiceID,
				"trigger_err", err,
				"rollback_err", rbErr,
			)
		}
		return fmt.Errorf("trigger admin void: %w", err)
	}
	s.log.Info("invoice_void_request_approved",
		"request_id", req.ID,
		"invoice_id", req.InvoiceID,
		"admin_id", adminID,
	)
	return nil
}

// AdminRejectVoidRequest 管理员驳回作废申请。
func (s *InvoiceService) AdminRejectVoidRequest(ctx context.Context, adminID, requestID int64, reason string) error {
	if adminID <= 0 {
		return ErrInvoiceForbidden
	}
	r := strings.TrimSpace(reason)
	if r == "" {
		return infraerrors.BadRequest("INVOICE_VOID_REJECT_REASON_REQUIRED", "请填写驳回理由")
	}
	row, err := s.entClient.InvoiceVoidRequest.Get(ctx, requestID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return ErrVoidRequestNotFound
		}
		return err
	}
	if row.Status != VoidRequestStatusPending {
		return ErrVoidRequestNotPending
	}
	now := time.Now()
	if _, err := s.entClient.InvoiceVoidRequest.UpdateOneID(row.ID).
		SetStatus(VoidRequestStatusRejected).
		SetAdminID(adminID).
		SetAdminNotes(r).
		SetReviewedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("mark void request rejected: %w", err)
	}
	s.log.Info("invoice_void_request_rejected",
		"request_id", row.ID,
		"invoice_id", row.InvoiceID,
		"admin_id", adminID,
		"reason", r,
	)
	return nil
}

func toVoidRequestDTO(r *dbent.InvoiceVoidRequest) *VoidRequestDTO {
	if r == nil {
		return nil
	}
	return &VoidRequestDTO{
		ID:         r.ID,
		UserID:     r.UserID,
		InvoiceID:  r.InvoiceID,
		Status:     r.Status,
		Reason:     r.Reason,
		AdminID:    r.AdminID,
		AdminNotes: r.AdminNotes,
		ReviewedAt: r.ReviewedAt,
		CreatedAt:  r.CreatedAt,
	}
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
	// 同样一次性预加载挂起的作废申请，按 invoice_id 索引
	pendingByInv, err := pendingVoidRequestsForInvoices(ctx, s.entClient, ids)
	if err != nil {
		return nil, err
	}
	items := make([]InvoiceDTO, 0, len(rows))
	for _, r := range rows {
		dto := toInvoiceDTO(r, countByInv[r.ID])
		if p, ok := pendingByInv[r.ID]; ok {
			dto.PendingVoidRequest = p
		}
		items = append(items, dto)
	}
	return &PaginatedInvoices{Items: items, Total: total, Page: page, Size: size}, nil
}

// pendingVoidRequestsForInvoices 批量查询给定发票 ID 列表中，是否有挂起的作废申请。
// 一次 SQL 防止 N+1。
func pendingVoidRequestsForInvoices(ctx context.Context, client *dbent.Client, invoiceIDs []int64) (map[int64]*PendingVoidRequestInfo, error) {
	out := make(map[int64]*PendingVoidRequestInfo, len(invoiceIDs))
	if len(invoiceIDs) == 0 {
		return out, nil
	}
	rows, err := client.InvoiceVoidRequest.Query().
		Where(
			invoicevoidrequest.InvoiceIDIn(invoiceIDs...),
			invoicevoidrequest.StatusEQ(VoidRequestStatusPending),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("preload pending void requests: %w", err)
	}
	for _, r := range rows {
		out[r.InvoiceID] = &PendingVoidRequestInfo{
			ID:          r.ID,
			Reason:      r.Reason,
			RequestedAt: r.CreatedAt,
		}
	}
	return out, nil
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
		ID:                inv.ID,
		ApplicationNo:     inv.ApplicationNo,
		InvoiceNo:         inv.InvoiceNo,
		UserID:            inv.UserID,
		UserEmail:         inv.UserEmail,
		TitleType:         inv.TitleType,
		Title:             inv.Title,
		TaxNo:             inv.TaxNo,
		Amount:            inv.Amount,
		Currency:          inv.Currency,
		Status:            inv.Status,
		OrderCount:        orderCount,
		SubmittedAt:       inv.SubmittedAt,
		ContactEmail:      inv.ContactEmail,
		Provider:          inv.Provider,
		ProviderState:     inv.ProviderState,
		InvoiceKind:       inv.InvoiceKind,
		ProviderLastError: inv.ProviderLastError,
		BuyerAddress:      inv.BuyerAddress,
		BuyerPhone:        inv.BuyerPhone,
		BuyerBankName:     inv.BuyerBankName,
		BuyerBankAccount:  inv.BuyerBankAccount,
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
