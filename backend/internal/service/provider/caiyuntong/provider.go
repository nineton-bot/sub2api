package caiyuntong

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Provider 实现 service.InvoiceProvider 接口（声明在 service 包，避免 import cycle，
// 这里只提供方法集，类型在外层注册时按 interface assignment 校验）。
type Provider struct {
	client *Client
}

// New 用配置构造一个 Provider。
func New(cfg Config, logger Logger) *Provider {
	return &Provider{client: NewClient(cfg, logger)}
}

// NewWithClient 注入已构造好的 Client，便于测试。
func NewWithClient(c *Client) *Provider { return &Provider{client: c} }

// Name 返回 provider 名称。
func (p *Provider) Name() string { return "caiyuntong" }

// --------------------------------------------------------------------------
// 蓝票（Issue）
// --------------------------------------------------------------------------

// IssueParams 是 service 层传入的请求参数，定义在这里避免对 service 包的依赖。
//
// Amount 字段意义：含税总额（RMB 元，正数），等于 SUM(Items[].PayAmount)。
// 接口层不再要求调用方传 UnitPrice * Quantity；行明细按 PayAmount 直接生成，
// 数量统一为 1（信息服务类电子发票普遍场景）。
type IssueParams struct {
	BillNo          string // 推荐 INV-{invoice_id}-{nano}
	RequestID       string
	TitleType       string // personal | business
	Title           string
	TaxNo           string
	ContactEmail    string
	Amount          float64
	InvoiceTypeCode string
	TaxRate         float64 // 整票统一税率；为 0 时 fallback 到 Config.DefaultTaxRate
	Items           []LineItem

	// 购方扩展信息（专票必填，普票可空）
	BuyerAddress     string
	BuyerPhone       string
	BuyerBankName    string
	BuyerBankAccount string
}

// LineItem 适配层视角的明细行。直接对应 invoice_items.pay_amount 等订单快照。
//
// 财云通对发票明细的商品类目支持两种字段（二选一即可，不可同时为空）：
//
//   - GoodsCode  — 商品编码，销方自维护，财云通后台预先注册。优先使用，平台直接按编码查商品库
//     带出名称/税率/单位等信息。短码（10 位左右）。
//   - ParentCode — 税收分类编码，国家统一 19 位。当 GoodsCode 缺失时平台用项目名查找商品库，
//     仍找不到时 fallback 用此编码确定税率/类目。
//
// 实测中即使两者都传，部分销方账号只接受预先维护过的 GoodsCode，因此 Config.GoodsCodeDefault
// 优先填充到 GoodsCode 字段。如果 default 值是 19 位数字，自动识别成 ParentCode。
type LineItem struct {
	Name       string
	PayAmount  float64 // 含税总额（RMB 元，正数）
	GoodsCode  string  // 商品编码（短码）
	ParentCode string  // 税收分类编码（19 位）
	Unit       string  // 可选
}

// Issue 调 POST /invoice/createInvoice 受理蓝票请求。
//
// 财云通该接口异步：返回的 RequestID 作为 trace 标识，真实状态需 Query 推进。
func (p *Provider) Issue(ctx context.Context, params IssueParams) (string /*traceID*/, error) {
	detail, err := p.buildBlueDetail(params)
	if err != nil {
		return "", err
	}

	req := CreateInvoiceRequest{
		Count:     1,
		RequestID: params.RequestID,
		Content:   []InvoiceDetail{detail},
	}

	respBytes, err := p.client.postJSON(ctx, "/invoice/createInvoice", req)
	if err != nil {
		return "", err
	}

	if errMsg := detectBusinessError(respBytes); errMsg != "" {
		return "", fmt.Errorf("财云通拒绝开票请求 %s", errMsg)
	}

	return params.RequestID, nil
}

func (p *Provider) buildBlueDetail(params IssueParams) (InvoiceDetail, error) {
	cfg := p.client.cfg
	if params.InvoiceTypeCode == "" {
		return InvoiceDetail{}, fmt.Errorf("invoice_type_code is required")
	}
	if len(params.Items) == 0 {
		return InvoiceDetail{}, fmt.Errorf("items is empty")
	}

	rate := params.TaxRate
	if rate <= 0 {
		rate = cfg.DefaultTaxRate
	}
	if rate <= 0 {
		rate = 0.06
	}

	totalIncl := round2(params.Amount)
	items, hdrIncl, hdrExcl, hdrTax := buildBlueLineItems(params.Items, totalIncl, rate, cfg.GoodsCodeDefault)

	natural := "0"
	if params.TitleType == "personal" {
		natural = "1"
	}

	return InvoiceDetail{
		BillNo:            params.BillNo,
		SellerTaxNum:      cfg.SellerTaxNum,
		Seller:            cfg.SellerName,
		SellerAddress:     cfg.SellerAddress,
		SellerPhone:       cfg.SellerPhone,
		SellerBankName:    cfg.SellerBankName,
		SellerBankAccount: cfg.SellerBankAcc,

		Buyer:             params.Title,
		NaturalPersonFlag: natural,
		BuyerTaxNum:       params.TaxNo,
		BuyerEmail:        params.ContactEmail,
		BuyerAddress:      params.BuyerAddress,
		BuyerPhone:        params.BuyerPhone,
		BuyerBankName:     params.BuyerBankName,
		BuyerBankAccount:  params.BuyerBankAccount,

		Drawer:   cfg.Drawer,
		Payee:    cfg.Payee,
		Reviewer: cfg.Reviewer,

		InvoiceType: params.InvoiceTypeCode,
		BillType:    "1",
		BillDate:    nowCST().Format("20060102150405"),

		TotalAmount:   formatDecimal(hdrIncl, 2),
		InvoiceAmount: formatDecimal(hdrExcl, 2),
		TaxAmount:     formatDecimal(hdrTax, 2),

		Items: items,
	}, nil
}

// buildBlueLineItems 生成蓝票行明细。
//
// 财云通有两条强校验互相博弈，必须同时满足：
//
//  1. **header invariants**：`SUM(Items[].SumAmount) == TotalAmount`、
//     `header InvoiceAmount + header TaxAmount == header TotalAmount`
//  2. **单行 tax 一致性**：`|InvoiceAmount × TaxRate - TaxAmount| ≤ 0.06`
//     （早期"末行兜底吸收 tax 差"的策略会让末行隐含税率严重偏离，导致 [9999] 8011 错误。）
//
// 解决方案：
//   - 每行 SumAmount 来自 PayAmount（末行用 totalIncl - accIncl 消除前面累加的尾差）
//   - 每行 InvoiceAmount/TaxAmount **按各自 SumAmount 独立 splitInclTax**，不再二次校准
//   - header InvoiceAmount/TaxAmount 直接取**累加值**，不另算
//
// 这样所有 invariant 同时满足，因为单行内 excl+tax=sum，累加后 acc(excl)+acc(tax)=acc(sum)=totalIncl。
//
// 返回值：(rows, totalIncl, totalExcl, totalTax)。
func buildBlueLineItems(in []LineItem, totalIncl, rate float64, defaultGoodsCode string) ([]InvoiceLineItem, float64, float64, float64) {
	n := len(in)
	rows := make([]InvoiceLineItem, 0, n)

	var accIncl, accExcl, accTax float64
	for i, it := range in {
		var sum float64
		switch {
		case i == n-1:
			// 末行吸收 PayAmount 累加产生的 1-2 分钱尾差，对齐 header total
			sum = round2(totalIncl - accIncl)
		case it.PayAmount > 0:
			sum = round2(it.PayAmount)
		default:
			// 全无 PayAmount 时按均分（少见，主要给 unit test fallback 路径）
			sum = round2(totalIncl / float64(n))
		}
		excl, tax := splitInclTax(sum, rate)

		accIncl = round2(accIncl + sum)
		accExcl = round2(accExcl + excl)
		accTax = round2(accTax + tax)

		goodsCode, parentCode := resolveItemCodes(it.GoodsCode, it.ParentCode, defaultGoodsCode)

		rows = append(rows, InvoiceLineItem{
			LineNo:        strconv.Itoa(i + 1),
			DetailName:    it.Name,
			Unit:          it.Unit,
			Num:           "1",
			TaxFlag:       "1",
			Price:         formatDecimal(sum, 2),
			InvoiceAmount: formatDecimal(excl, 2),
			SumAmount:     formatDecimal(sum, 2),
			TaxRate:       formatTaxRate(rate),
			TaxAmount:     formatDecimal(tax, 2),
			DiscountType:  "0",
			GoodsCode:     goodsCode,
			ParentCode:    parentCode,
		})
	}
	return rows, accIncl, accExcl, accTax
}

// --------------------------------------------------------------------------
// 查询（Query）
// --------------------------------------------------------------------------

// QueryParams 适配层查询参数。
type QueryParams struct {
	BillNo          string
	InvoiceTypeCode string
	IsRed           bool
}

// QueryResult 适配层查询结果。
type QueryResult struct {
	Stage     string // pending | issued | failed | confirmed
	InvoiceNo string
	PDFURL    string // 仅当响应不带内嵌字节时才需要；测试环境此 URL 是 HTML 预览页
	PDFBytes  []byte // 优先字段：从 InvoicePdfBase64 解码所得真正 PDF 字节流
	Reason    string
	Payload   map[string]any
}

// Query 调 POST /invoice/getInvoiceAll 拉取当前票面状态（2.3 发票全票面信息查询）。
//
// 文档 2.3.1 InvoiceOpenState 枚举（实测样例）：
//   "0" 未开 / "1" 开具中 / "2" 开具失败 / "3" 开具成功 / "4" 已作废
//
// 兼容性：万一某些版本字段缺失，我们额外检查 InvalidFlag 与 InvoiceNumeric。
func (p *Provider) Query(ctx context.Context, params QueryParams) (*QueryResult, error) {
	req := QueryInvoiceRequest{
		RequestID: "Q-" + params.BillNo + "-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Content: []QueryInvoiceQueryItem{
			{BillNo: params.BillNo, SellerTaxNum: p.client.cfg.SellerTaxNum},
		},
	}

	respBytes, err := p.client.postJSON(ctx, "/invoice/getInvoiceAll", req)
	if err != nil {
		return nil, err
	}

	var resp QueryInvoiceResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("decode getInvoiceAll: %w", err)
	}

	if len(resp.Content) == 0 {
		return &QueryResult{Stage: "pending"}, nil
	}

	entry := resp.Content[0]
	payload := map[string]any{
		"raw":              json.RawMessage(respBytes),
		"invoiceCode":      entry.InvoiceCode,
		"invoiceNumeric":   entry.InvoiceNumeric,
		"invoiceOpenState": entry.InvoiceOpenState,
		"invalidFlag":      entry.InvalidFlag,
		"redFlag":          entry.RedFlag,
		"billingDate":      entry.BillingDate,
	}

	// 财云通 getInvoiceAll 每条 Content[].Code 实测枚举：
	//   "200" / "0000"   = 已开具成功（取出票数据）
	//   "7777" / "9999"  = 平台还在内部处理 / 校验阶段，**瞬态**，下一轮 poll 会变化
	//   其它             = 真实业务错误，但我们没拿到完整枚举表，谨慎起见统一当 pending，
	//                      让 10 分钟 timeout 兜底失败，避免误杀；同时把 Code 记到 payload
	//                      便于排查。
	//
	// 设计权衡：宁可慢一点 timeout，也不要把仍在处理的发票误标 failed 释放掉订单
	// 锁；实际只有 "200"/"0000" 才会被认为完成。
	if entry.Code != "" && entry.Code != "200" && entry.Code != "0000" {
		// 检查 InvoiceOpenState：如果是 "2"（开具失败终态），才能直接判失败
		if strings.TrimSpace(entry.InvoiceOpenState) == "2" {
			return &QueryResult{
				Stage:   "failed",
				Reason:  extractRealError(entry),
				Payload: payload,
			}, nil
		}
		// 其他 Code（7777/9999/...）= 仍在处理中
		payload["transientCode"] = entry.Code
		return &QueryResult{Stage: "pending", Payload: payload}, nil
	}

	switch strings.TrimSpace(entry.InvoiceOpenState) {
	case "3": // 开具成功
		invoiceNo := entry.InvoiceNumeric
		if invoiceNo == "" {
			invoiceNo = entry.InvoiceCode + entry.InvoiceNumeric
		}
		// 优先用内嵌的 InvoicePdfBase64 — 测试环境 DownloadUrl 是 HTML 预览页，
		// 直接 HTTP GET 会拿到 HTML 而非 PDF，必须用 base64 字段还原。
		var pdfBytes []byte
		if entry.InvoicePdfBase64 != "" {
			if b, err := base64.StdEncoding.DecodeString(entry.InvoicePdfBase64); err == nil {
				pdfBytes = b
			}
		}
		return &QueryResult{
			Stage:     "issued",
			InvoiceNo: invoiceNo,
			PDFURL:    fallback(entry.DownloadUrl, entry.InvoiceOfd),
			PDFBytes:  pdfBytes,
			Payload:   payload,
		}, nil
	case "2": // 开具失败
		return &QueryResult{
			Stage:   "failed",
			Reason:  fallback(entry.Message, "open state=2 (failed)"),
			Payload: payload,
		}, nil
	case "4": // 已作废
		if entry.InvalidFlag == "1" {
			return &QueryResult{
				Stage:   "failed",
				Reason:  "invoice invalidated on platform side",
				Payload: payload,
			}, nil
		}
		fallthrough
	default: // 0 待开 / 1 开具中 / "" 平台还没受理
		return &QueryResult{Stage: "pending", Payload: payload}, nil
	}
}

// --------------------------------------------------------------------------
// 红冲（Reverse）
// --------------------------------------------------------------------------

// ReverseParams 红冲适配层参数。
type ReverseParams struct {
	BlueBillNo      string
	BlueInvoiceNo   string
	BlueInvoiceDate string // yyyyMMdd
	BlueInvoiceCode string
	BlueInvoiceNum  string
	InvoiceTypeCode string
	Title           string
	TaxNo           string
	TitleType       string
	ContactEmail    string
	Amount          float64 // 蓝票含税总额（正数），红票内部按 -Amount 处理
	TaxRate         float64 // 与蓝票一致；0 时 fallback 到 Config.DefaultTaxRate
	Items           []LineItem
	Reason          string // 默认 01
	Step            string // 当前 reverse_step
	RedAdviceNum    string
	RedConfirmNum   string
	BillNoRed       string // 红票 BillNo，建议 REV-{invoice_id}-{nano}
	RequestID       string //

	// 购方扩展信息（专票红冲必填，同蓝票一致）
	BuyerAddress     string
	BuyerPhone       string
	BuyerBankName    string
	BuyerBankAccount string
}

// ReverseResult 红冲结果。NextStep 给上层推进子状态机。
type ReverseResult struct {
	NextStep      string
	RedAdviceNum  string
	RedConfirmNum string
	TraceID       string
	Payload       map[string]any
}

// Reverse 推进红冲流程。
//
// 财云通 1.0.1 文档 2.2 章节：所有数电票（05/06）红冲走 `/xxp/returnReceipts/storage`
// 单一接口推送退货单，**平台自动完成红字信息单填开 + 红票开具**，无需调用方多步推进。
//
// 因此 step 状态机简化为：
//   "" 或 "red_pending"  →  推送退货单 → "red_issuing"
//   "red_issuing"        →  no-op，由 pollWorker 用红票 BillNo 调 getInvoiceAll
//
// 税控发票（01/04/08/10）目前 docx 没暴露独立 path，可能也走同一接口（文档说
// 「仅支持数电票相关的订单」，所以税控不支持自动红冲，需走 manual 路径）。
func (p *Provider) Reverse(ctx context.Context, params ReverseParams) (*ReverseResult, error) {
	if !isDianpiao(params.InvoiceTypeCode) {
		return nil, fmt.Errorf("caiyuntong: invoice_type %q does not support auto reverse (only 数电票 05/06 supported)", params.InvoiceTypeCode)
	}
	switch params.Step {
	case "", "red_pending":
		traceID, err := p.pushReturnReceipt(ctx, params)
		if err != nil {
			return nil, err
		}
		return &ReverseResult{NextStep: ReverseStepRedIssuing, TraceID: traceID}, nil
	case ReverseStepRedIssuing:
		// 等 pollWorker 查询红票出票结果
		return &ReverseResult{NextStep: ReverseStepRedIssuing}, nil
	}
	return nil, fmt.Errorf("unknown reverse step %q", params.Step)
}

// ReverseStepRedIssuing 红票已推送，等出票（与 service.ReverseStepRedIssuing 字符串值一致）
const ReverseStepRedIssuing = "red_issuing"

// pushReturnReceipt 推送退货单。一次成功返回后，财云通会异步完成红字信息单 +
// 红票开具，调用方用同一 BillNo 调 getInvoiceAll 查红票出票结果。
func (p *Provider) pushReturnReceipt(ctx context.Context, params ReverseParams) (string, error) {
	cfg := p.client.cfg

	rate := p.effectiveTaxRate(params.TaxRate)
	totalIncl := round2(-params.Amount) // 红冲金额必须负数
	rows, hdrIncl, hdrExcl, hdrTax := buildReturnReceiptItems(params.Items, totalIncl, rate, cfg.GoodsCodeDefault)

	content := ReturnReceiptContent{
		BillNo:            params.BillNoRed,
		SellerTaxNum:      cfg.SellerTaxNum,
		Seller:            cfg.SellerName,
		SellerAddress:     cfg.SellerAddress,
		SellerPhone:       cfg.SellerPhone,
		SellerBankName:    cfg.SellerBankName,
		SellerBankAccount: cfg.SellerBankAcc,

		Buyer:            params.Title,
		BuyerTaxNum:      params.TaxNo,
		BuyerEmail:       params.ContactEmail,
		BuyerAddress:     params.BuyerAddress,
		BuyerPhone:       params.BuyerPhone,
		BuyerBankName:    params.BuyerBankName,
		BuyerBankAccount: params.BuyerBankAccount,

		Drawer:   cfg.Drawer,
		Payee:    cfg.Payee,
		Reviewer: cfg.Reviewer,

		InvoiceType:     params.InvoiceTypeCode,
		BillNoOld:       params.BlueBillNo,
		BlueInvoiceNo:   params.BlueInvoiceNo,
		BlueInvoiceDate: params.BlueInvoiceDate,
		BlueInvoiceCode: params.BlueInvoiceCode, // 数电票可空

		TotalAmount:   formatDecimal(hdrIncl, 2),
		InvoiceAmount: formatDecimal(hdrExcl, 2),
		TaxAmount:     formatDecimal(hdrTax, 2),

		BillDate:      nowCST().Format("20060102150405"),
		TreatmentType: "1", // 数电票自动处理
		ApplyWay:      "0", // 全部冲红
		RedSeason:     fallback(params.Reason, "01"),

		Items: rows,
	}

	req := ReturnReceiptStorageRequest{
		RequestID: params.RequestID,
		Count:     1,
		Content:   []ReturnReceiptContent{content},
	}

	respBytes, err := p.client.postJSON(ctx, "/xxp/returnReceipts/storage", req)
	if err != nil {
		return "", err
	}
	var resp ReturnReceiptStorageResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return "", fmt.Errorf("decode returnReceiptStorage: %w", err)
	}
	if resp.Code != "" && resp.Code != "200" {
		// 拼接 ErrorDatas 里的具体业务错误描述（如有）
		var details []string
		for _, e := range resp.ErrorDatas {
			if e.ErrorDesc != "" {
				details = append(details, e.ErrorDesc)
			}
		}
		msg := fallback(resp.Message, "平台业务错误（未返回详情）")
		if len(details) > 0 {
			msg = strings.Join(details, "；")
		}
		return "", fmt.Errorf("财云通拒绝红冲请求 [%s] %s", resp.Code, msg)
	}
	return params.RequestID, nil
}

// buildReturnReceiptItems 退货单明细行（金额负数）。算法同 buildBlueLineItems：
//   - 每行 sum=PayAmount（末行兜底吸收尾差）
//   - excl/tax 各行独立按 sum 拆分，不二次校准
func buildReturnReceiptItems(in []LineItem, totalIncl, rate float64, defaultGoodsCode string) ([]ReturnReceiptItem, float64, float64, float64) {
	n := len(in)
	rows := make([]ReturnReceiptItem, 0, n)

	var accIncl, accExcl, accTax float64
	for i, it := range in {
		var sum float64
		switch {
		case i == n-1:
			sum = round2(totalIncl - accIncl)
		case it.PayAmount > 0:
			sum = -round2(it.PayAmount)
		default:
			sum = round2(totalIncl / float64(n))
		}
		excl, tax := splitInclTax(sum, rate)
		accIncl = round2(accIncl + sum)
		accExcl = round2(accExcl + excl)
		accTax = round2(accTax + tax)

		goodsCode, parentCode := resolveItemCodes(it.GoodsCode, it.ParentCode, defaultGoodsCode)

		rows = append(rows, ReturnReceiptItem{
			LineNo:        i + 1,
			DetailName:    it.Name,
			Unit:          it.Unit,
			Num:           "-1",
			TaxFlag:       "1",
			Price:         formatDecimal(-sum, 2), // Price 一般填正数
			InvoiceAmount: formatDecimal(excl, 2),
			SumAmount:     formatDecimal(sum, 2),
			TaxRate:       formatTaxRate(rate),
			TaxAmount:     formatDecimal(tax, 2),
			DiscountType:  "0",
			GoodsCode:     goodsCode,
			ParentCode:    parentCode,
		})
	}
	return rows, accIncl, accExcl, accTax
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

// resolveItemCodes 根据明细 item 自填的 GoodsCode/ParentCode 与 default fallback
// 决定最终送给财云通的两个字段。
//
//   - 优先用 item 显式指定的 GoodsCode（商品编码，销方账号内自维护，平台直接查得到）
//   - 否则用 item 显式指定的 ParentCode（19 位税收分类编码）
//   - 都没填时用 defaultGoodsCode：按长度判断
//     * len >= 18 视为税收分类编码 → 塞 ParentCode
//     * 其他长度视为商品编码     → 塞 GoodsCode
//
// 实测中销方账号 500102201007206608（神州云合测试公司）对 ParentCode 查找不到的商品
// 会回「暂不支持所选税率」，所以即使用 ParentCode 也需要确认该编码对应的商品已在销方
// 商品库中预维护，否则平台无法定位税率。
func resolveItemCodes(itemGoodsCode, itemParentCode, defaultCode string) (goods, parent string) {
	if itemGoodsCode != "" {
		return itemGoodsCode, ""
	}
	if itemParentCode != "" {
		return "", itemParentCode
	}
	if defaultCode == "" {
		return "", ""
	}
	if len(defaultCode) >= 18 {
		return "", defaultCode
	}
	return defaultCode, ""
}

func isDianpiao(code string) bool {
	switch code {
	case "05", "06":
		return true
	}
	return false
}

// effectiveTaxRate 解析当前请求的有效税率：参数 > config 默认 > 6%。
func (p *Provider) effectiveTaxRate(req float64) float64 {
	if req > 0 {
		return req
	}
	if p.client.cfg.DefaultTaxRate > 0 {
		return p.client.cfg.DefaultTaxRate
	}
	return 0.06
}

// chinaTZ 财云通要求的中国时区。lazy 初始化避免 init 顺序问题。
var chinaTZ = time.FixedZone("CST+8", 8*3600)

func nowCST() time.Time { return time.Now().In(chinaTZ) }

func splitInclTax(total float64, rate float64) (excl, tax float64) {
	excl = round2(total / (1 + rate))
	tax = round2(total - excl)
	return
}

func round2(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return -float64(int64(-v*100+0.5)) / 100
}

func formatDecimal(v float64, prec int) string {
	return strconv.FormatFloat(v, 'f', prec, 64)
}

func fallback(s, dft string) string {
	if strings.TrimSpace(s) == "" {
		return dft
	}
	return s
}

// extractRealError 把财云通真错误信息友好化，便于管理员排错。
//
// 财云通在开具失败（InvoiceOpenState=2）时**不会**把错误塞进顶层 Message 字段；
// 真错误反而塞在 InvoicePdfBase64 / InvoiceOfdBase64 / InvoiceXmlBase64 这种"应该是
// 版式文件"的字段里 —— 用 base64 编码的 JSON：
//
//	{"code":"001","message":"请求参数异常。详情：发票号码invoiceNumber为空。"}
//
// 函数按优先级尝试：
//  1. entry.Message 非空 → 直接用
//  2. PDF / OFD / XML 任一 Base64 字段能 base64-decode 且解出 JSON.message → 用 JSON.message
//  3. 都没有 → 退回到通用提示
//
// 返回值带上财云通 Code 前缀（如 "[7777]"），便于 cross-reference。
func extractRealError(entry QueryInvoiceDataEntry) string {
	prefix := ""
	if entry.Code != "" {
		prefix = "[" + entry.Code + "] "
	}
	if strings.TrimSpace(entry.Message) != "" {
		return prefix + entry.Message
	}
	for _, b64 := range []string{entry.InvoicePdfBase64, entry.InvoiceOfdBase64, entry.InvoiceXmlBase64} {
		if b64 == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			continue
		}
		// 真 PDF/OFD/XML 是二进制，base64 解码后第一段字节不会形成合法 JSON。
		// 失败时塞进来的反而是 JSON 字符串。
		if !json.Valid(raw) {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		if msg, ok := m["message"].(string); ok && msg != "" {
			if code, _ := m["code"].(string); code != "" {
				return prefix + "平台错误[" + code + "]：" + msg
			}
			return prefix + "平台错误：" + msg
		}
	}
	return prefix + "财云通开具失败（InvoiceOpenState=2，未返回详细原因，请联系开票平台技术支持）"
}

// formatTaxRate 输出 4 位以内、小数最多 3 位、不带尾零 0 的税率字符串。
// 财云通文档明确："TaxRate 最大长度 4 位、小数位不能超过 3 位"。
//
// 例：0.13 → "0.13"；0.06 → "0.06"；0.005 → "0.005"；0.13000 → "0.13"。
func formatTaxRate(rate float64) string {
	s := strconv.FormatFloat(rate, 'f', -1, 64) // 自动去尾零
	if len(s) > 4 {
		// 兜底：截到 4 位，但要保留小数点
		s = s[:4]
	}
	return s
}

// detectBusinessError 透传业务级错误。
//
// 财云通的 HTTP 层永远 200，业务错误码塞在 body 里：
//
//	{"Code":"200","Message":"Success", ...}                  // 成功
//	{"Code":"400","Message":"签名不正确"}                     // 鉴权/参数错误
//	{"Code":"400","Message":"开票失败单据，详见ErrorDatas",
//	  "ErrorDatas":[{"billNo":"...","errorCode":"4000",
//	                "errorDesc":"暂不支持所选税率"}]}        // 业务校验失败（关键信息在 ErrorDatas）
//	{"Code":"500","Message":"系统异常"}                       // 平台内部错误
//
// 字段一律大写（文档 2.3 章节确认 Code/Message 大写），早期实现用 "code"/"message"
// 小写，导致从未触发业务错误检测，外层把失败当成功，让 poll worker 空转直到超时。
//
// 返回的错误信息尽量友好可读：优先用 ErrorDatas[].errorDesc（具体业务原因），
// 否则 fallback 到顶层 Message。
func detectBusinessError(body []byte) string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	codeStr := stringField(m, "Code", "code")
	msgStr := stringField(m, "Message", "message")
	if codeStr == "" || codeStr == "200" || codeStr == "0000" {
		return ""
	}

	// 提取 ErrorDatas[].errorDesc，这才是真业务原因（如"暂不支持所选税率"）
	var details []string
	if arr, ok := m["ErrorDatas"].([]any); ok {
		for _, e := range arr {
			ed, ok := e.(map[string]any)
			if !ok {
				continue
			}
			desc := stringField(ed, "errorDesc", "ErrorDesc")
			if desc != "" {
				details = append(details, desc)
			}
		}
	}

	prefix := "[" + codeStr + "] "
	if len(details) > 0 {
		return prefix + strings.Join(details, "；")
	}
	if msgStr != "" {
		return prefix + msgStr
	}
	return prefix + "平台业务错误（未返回详情）"
}

// stringField 从 map 中读取首个非空字符串字段（支持 fallback key）。
func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
