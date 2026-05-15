package service

import (
	"context"
	"fmt"
)

// InvoiceProvider 第三方自动开票服务的抽象。
//
// V3 引入 Query / Reverse，配合 invoice_issue_worker / invoice_poll_worker /
// invoice_reverse_worker 完成异步开票 + 自动红冲全链路。
//
// Issue 与 Reverse 通常返回受理 ack（trace_id），真实结果由 Query 轮询拿到。
type InvoiceProvider interface {
	Name() string

	// Issue 提交开票请求。多数实现是异步：返回受理 trace_id，
	// 真实出票结果由 Query 推进。同步出票（test stub）也可直接返回 InvoiceNo + PDFData。
	Issue(ctx context.Context, req InvoiceProviderRequest) (*InvoiceProviderResult, error)

	// Query 查询蓝票或红票当前状态。一次调用应返回稳定的 ProviderStatus。
	Query(ctx context.Context, req InvoiceProviderQuery) (*InvoiceProviderStatus, error)

	// Reverse 红冲（作废 / 冲红）。
	//   - 数电票：内部要走 申请红字信息单 -> 等确认 -> 开红票 -> 查出票 四步链路，
	//     调用方按 reverse_step 多次调用本方法直至 RedDone。
	//   - 税控票：单次调用直接提交红票，再用 Query 查出票。
	Reverse(ctx context.Context, req InvoiceProviderReverseRequest) (*InvoiceProviderReverseResult, error)
}

// InvoiceProviderRequest 调用上下文（不依赖 ent 类型，便于跨层使用）。
type InvoiceProviderRequest struct {
	InvoiceID       int64
	UserID          int64
	UserEmail       string
	TitleType       string // personal | business
	Title           string
	TaxNo           string
	ContactEmail    string
	Notes           string
	Amount          float64
	Currency        string
	InvoiceKind     string // normal | special
	InvoiceTypeCode string // 04 / 10 / 01 / 08 / 05 / 06
	Items           []InvoiceProviderItem
}

type InvoiceProviderItem struct {
	OrderNo     string
	ProductName string
	OrderType   string
	PayAmount   float64
}

type InvoiceProviderResult struct {
	TraceID   string         // 受理 ID（财云通 RequestID）
	InvoiceNo string         // 同步出票时填，否则空
	PDFData   []byte         // 同步出票时填
	PDFName   string         //
	Payload   map[string]any //
}

// InvoiceProviderQuery 查询参数。
type InvoiceProviderQuery struct {
	InvoiceID       int64
	BillNo          string // 财云通 BillNo（蓝票或红票）
	TraceID         string
	InvoiceTypeCode string
	IsRed           bool // true 表示查红票
}

// InvoiceProviderStatus 统一查询结果。
type InvoiceProviderStatus struct {
	Stage     ProviderStage  // 进入状态机的哪个里程碑
	InvoiceNo string         // 成功时填
	PDFURL    string         //
	PDFData   []byte         // 已下载到的 PDF 字节流，可空
	Reason    string         // 失败原因
	Payload   map[string]any //
}

type ProviderStage string

const (
	ProviderStagePending   ProviderStage = "pending"   // 还在处理中
	ProviderStageIssued    ProviderStage = "issued"    // 开票成功
	ProviderStageFailed    ProviderStage = "failed"    // 永久失败
	ProviderStageConfirmed ProviderStage = "confirmed" // 数电红字信息单已确认
)

// InvoiceProviderReverseRequest 红冲请求。
//
// 调用方根据红冲子状态机 reverse_step 决定要执行哪个步骤：
//   - "" / red_pending: 数电 -> 申请红字信息单；税控 -> 直接开红票
//   - red_confirmed:    数电 -> 拿 RedConfirmNum 开红票
//   - red_issuing:      仅查询，调用 Query
type InvoiceProviderReverseRequest struct {
	InvoiceID       int64
	BlueBillNo      string
	BlueInvoiceNo   string
	BlueInvoiceDate string // yyyyMMdd
	BlueInvoiceCode string // 税控发票必填
	BlueInvoiceNum  string // 税控发票必填
	InvoiceTypeCode string
	Title           string
	TaxNo           string
	TitleType       string
	ContactEmail    string
	Amount          float64
	Items           []InvoiceProviderItem
	Reason          string // 默认 01 销货退回
	Step            string // 当前 reverse_step
	RedAdviceNum    string // step >= red_confirmed 时填
	RedConfirmNum   string //
}

type InvoiceProviderReverseResult struct {
	NextStep      string         // 推进到的子状态：red_applying | red_confirmed | red_issuing | red_done
	RedAdviceNum  string         //
	RedConfirmNum string         //
	TraceID       string         // 红票 RequestID
	Payload       map[string]any //
}

// InvoiceProviderRegistry 维护已注册的 provider 实现。
type InvoiceProviderRegistry struct {
	providers map[string]InvoiceProvider
}

func NewInvoiceProviderRegistry(providers ...InvoiceProvider) *InvoiceProviderRegistry {
	r := &InvoiceProviderRegistry{providers: make(map[string]InvoiceProvider, len(providers)+1)}
	r.providers["manual"] = manualInvoiceProvider{}
	for _, p := range providers {
		if p == nil {
			continue
		}
		r.providers[p.Name()] = p
	}
	return r
}

// Get 按名字查找 provider。未注册时回 manual。
func (r *InvoiceProviderRegistry) Get(name string) InvoiceProvider {
	if name == "" {
		return r.providers["manual"]
	}
	if p, ok := r.providers[name]; ok {
		return p
	}
	return r.providers["manual"]
}

// Names 列出已注册 provider（管理员后台展示用）。
func (r *InvoiceProviderRegistry) Names() []string {
	out := make([]string, 0, len(r.providers))
	for k := range r.providers {
		out = append(out, k)
	}
	return out
}

// manualInvoiceProvider 是默认的 no-op 实现。
type manualInvoiceProvider struct{}

func (manualInvoiceProvider) Name() string { return "manual" }

func (manualInvoiceProvider) Issue(_ context.Context, _ InvoiceProviderRequest) (*InvoiceProviderResult, error) {
	return nil, fmt.Errorf("manual provider does not auto-issue; admin must upload PDF")
}

func (manualInvoiceProvider) Query(_ context.Context, _ InvoiceProviderQuery) (*InvoiceProviderStatus, error) {
	return &InvoiceProviderStatus{Stage: ProviderStagePending}, nil
}

func (manualInvoiceProvider) Reverse(_ context.Context, _ InvoiceProviderReverseRequest) (*InvoiceProviderReverseResult, error) {
	return nil, fmt.Errorf("manual provider does not auto-reverse")
}
