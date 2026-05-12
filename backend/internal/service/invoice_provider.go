package service

import (
	"context"
	"fmt"
)

// InvoiceProvider 第三方自动开票服务的抽象。
//
// V1 仅 manual（不调用任何外部服务、由管理员上传 PDF）。后续接入诺诺 / 百望 / 航信
// 等真实电子发票供应商时，新建一个实现并注册到 InvoiceProviderRegistry 即可。
//
// 一次成功的 Issue 应返回：
//   - InvoiceNo:  真实发票号（必填）
//   - PDFData:    PDF 字节流（可空 — 部分供应商只回 URL，由调用方再下载）
//   - PDFName:    建议文件名
//   - Payload:    供应商透传字段（例如对账 trace id），落库到 invoices.provider_payload
//
// 当 InvoiceProvider 实现失败时返回 error；上层 service 不会落库 issued 状态。
type InvoiceProvider interface {
	Name() string
	Issue(ctx context.Context, req InvoiceProviderRequest) (*InvoiceProviderResult, error)
}

// InvoiceProviderRequest 调用上下文（不依赖 ent 类型，便于跨层使用）。
type InvoiceProviderRequest struct {
	InvoiceID    int64
	UserID       int64
	UserEmail    string
	TitleType    string
	Title        string
	TaxNo        string
	ContactEmail string
	Notes        string
	Amount       float64
	Currency     string
	Items        []InvoiceProviderItem
}

type InvoiceProviderItem struct {
	OrderNo     string
	ProductName string
	OrderType   string
	PayAmount   float64
}

type InvoiceProviderResult struct {
	InvoiceNo string
	PDFData   []byte
	PDFName   string
	Payload   map[string]any
}

// InvoiceProviderRegistry 维护已注册的 provider 实现。
//
// 当前 V1 默认只注册 manualInvoiceProvider（no-op）。新供应商接入步骤：
//  1. 实现 InvoiceProvider 接口
//  2. 在 wire.go 的 ProvideInvoiceProviderRegistry 中注册
//  3. 在 admin 后台允许管理员选择 provider（invoices.provider 字段）
//  4. 在 InvoiceService.AdminMarkIssued / AdminUploadPDF 中根据 provider 分支：
//     - "manual"：当前手动流程
//     - 其它：调 registry.Get(provider).Issue(ctx, req)
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

// manualInvoiceProvider 是默认的 no-op 实现：
// 不调用任何外部服务，直接返回错误，提示由管理员手动上传 PDF。
type manualInvoiceProvider struct{}

func (manualInvoiceProvider) Name() string { return "manual" }

func (manualInvoiceProvider) Issue(_ context.Context, _ InvoiceProviderRequest) (*InvoiceProviderResult, error) {
	return nil, fmt.Errorf("manual provider does not auto-issue; admin must upload PDF")
}
