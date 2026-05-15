package caiyuntong

// 请求 / 响应 DTO。字段命名严格对齐财云通《销项应用系统接口 1.0.1》文档。
//
// JSON 大小写以接口文档为准（多数字段为 CapitalCase）。红冲样例报文里部分字段呈 camelCase，
// 实际以销项平台返回为准，由调用方在 unmarshal 时按 fuzzy 匹配处理（见 query.go）。

// CreateInvoiceRequest 对应 POST /invoice/createInvoice。
type CreateInvoiceRequest struct {
	Count     int             `json:"Count"`
	RequestID string          `json:"RequestID"`
	Content   []InvoiceDetail `json:"Content"`
}

// InvoiceDetail 单张发票。蓝票 / 红票 共用同一结构，红票需要额外填写 BlueInvoiceNo 等字段。
type InvoiceDetail struct {
	// 必填核心
	BillNo       string `json:"BillNo"`
	SellerTaxNum string `json:"SellerTaxNum"`
	Seller       string `json:"Seller,omitempty"`

	// 销方信息
	SellerAddress     string `json:"SellerAddress,omitempty"`
	SellerPhone       string `json:"SellerPhone,omitempty"`
	SellerBankName    string `json:"SellerBankName,omitempty"`
	SellerBankAccount string `json:"SellerBankAccount,omitempty"`

	// 购方
	Buyer             string `json:"Buyer"`
	NaturalPersonFlag string `json:"NaturalPersonFlag,omitempty"` // "0"|"1"
	BuyerTaxNum       string `json:"BuyerTaxNum,omitempty"`
	BuyerAddress      string `json:"BuyerAddress,omitempty"`
	BuyerPhone        string `json:"BuyerPhone,omitempty"`
	BuyerEmail        string `json:"BuyerEmail,omitempty"`
	BuyerBankName     string `json:"BuyerBankName,omitempty"`
	BuyerBankAccount  string `json:"BuyerBankAccount,omitempty"`

	// 经办人
	Drawer   string `json:"Drawer,omitempty"`
	Payee    string `json:"Payee,omitempty"`
	Reviewer string `json:"Reviewer,omitempty"`

	// 票种与开票类型
	InvoiceType string `json:"InvoiceType"`        // 04/10/01/08/05/06
	BillType    string `json:"BillType"`           // 1 蓝票 / 2 红票
	BillDate    string `json:"BillDate"`           // yyyyMMddHHmmss
	BillAmount  string `json:"BillAmount,omitempty"`

	// 金额（红票为负值）
	TotalAmount   string `json:"TotalAmount"`
	InvoiceAmount string `json:"InvoiceAmount"`
	TaxAmount     string `json:"TaxAmount"`

	// 红票专用
	BillNoOld          string `json:"BillNoOld,omitempty"`
	InvoiceCodeOld     string `json:"InvoiceCodeOld,omitempty"`
	InvoiceNumericOld  string `json:"InvoiceNumericOld,omitempty"`
	BlueInvoiceNo      string `json:"BlueInvoiceNo,omitempty"`
	BlueInvoiceCode    string `json:"BlueInvoiceCode,omitempty"`
	BlueInvoiceDate    string `json:"BlueInvoiceDate,omitempty"` // yyyyMMdd
	RedAdviceNum       string `json:"RedAdviceNum,omitempty"`
	RedConfirmNum      string `json:"RedConfirmNum,omitempty"`
	RedReason          string `json:"RedReason,omitempty"` // 01 销货退回 ...
	ApplyWay           string `json:"applyWay,omitempty"`
	RedSeason          string `json:"redSeason,omitempty"`
	SpecialInvoiceRR   string `json:"specialInvoiceRedReason,omitempty"`

	Remarks   string             `json:"Remarks,omitempty"`
	StoreCode string             `json:"StoreCode,omitempty"`
	TaxMethod string             `json:"TaxMethod,omitempty"`
	MachineNo string             `json:"MachineNo,omitempty"`
	Version   string             `json:"Version,omitempty"`
	Items     []InvoiceLineItem  `json:"Items"`
	Extf1     string             `json:"extf1,omitempty"`
	Extf2     string             `json:"extf2,omitempty"`
	Extf3     string             `json:"extf3,omitempty"`
}

// InvoiceLineItem 单条明细行。
type InvoiceLineItem struct {
	LineNo        string `json:"LineNo,omitempty"`
	DetailName    string `json:"DetailName"`
	Unit          string `json:"Unit,omitempty"`
	Standard      string `json:"Standard,omitempty"`
	Num           string `json:"Num,omitempty"`
	TaxFlag       string `json:"TaxFlag"` // 1 含税 0 不含税
	Price         string `json:"Price,omitempty"`
	InvoiceAmount string `json:"InvoiceAmount"`
	SumAmount     string `json:"SumAmount"`
	TaxRate       string `json:"TaxRate"`
	TaxAmount     string `json:"TaxAmount"`
	DiscountType  string `json:"DiscountType"` // 0 正常 1 折扣 2 被折扣
	GoodsCode     string `json:"GoodsCode,omitempty"`
	ParentCode    string `json:"ParentCode,omitempty"`
	FreeTax       string `json:"FreeTax,omitempty"`
}

// GenericResponse 通用应答外壳。
//
// 财云通的应答各接口字段略有差异，这里只取共用字段；具体 invoice/redInfo 状态需要单独
// 解构后再判断。
type GenericResponse struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Success   bool                   `json:"success"`
	RequestID string                 `json:"requestID"`
	Data      map[string]any         `json:"data"`
	Raw       map[string]any         `json:"-"`
}

// QueryInvoiceRequest 对应 POST /invoice/getInvoiceAll（2.3 发票全票面信息查询）。
//
// 注意 Content 是数组，可同时查询多个 BillNo（文档说最多 100 个）。
type QueryInvoiceRequest struct {
	RequestID string                  `json:"RequestID"`
	Content   []QueryInvoiceQueryItem `json:"Content"`
}

type QueryInvoiceQueryItem struct {
	BillNo       string `json:"BillNo"`
	SellerTaxNum string `json:"SellerTaxNum"`
}

// QueryInvoiceResponse 查询结果，按 2.3.1.3 返回数据样例。
type QueryInvoiceResponse struct {
	RequestID string                  `json:"RequestID"`
	Message   string                  `json:"Message"`
	Content   []QueryInvoiceDataEntry `json:"Content"`
}

// QueryInvoiceDataEntry 单张发票的全票面信息。
//
// 数电发票交付有两种形态：
//
//  1. **InvoicePdfBase64**：响应内嵌的 base64 编码 PDF 字节流，最常用，无需二次 HTTP。
//  2. **InvoiceOfdBase64**：OFD 版式（GB/T 33190-2016 中国电子文件官方格式），税务系统优先。
//
// **DownloadUrl 在测试环境指向 invoicePreviewTest.html 网页预览，下载会得到 HTML 而非 PDF**——
// 因此 service 层下载 PDF 时应优先用 InvoicePdfBase64 解码，DownloadUrl 仅作 fallback。
type QueryInvoiceDataEntry struct {
	BillNo           string `json:"BillNo"`
	BillType         string `json:"BillType"` // 1 蓝 / 2 红
	Code             string `json:"Code"`     // "0000" 成功；其他为业务错误码
	Message          string `json:"Message"`
	InvoiceCode      string `json:"InvoiceCode"` // 发票代码（数电票通常为空）
	InvoiceNumeric   string `json:"InvoiceNumeric"`
	InvoiceOpenState string `json:"InvoiceOpenState"` // "3" 开具成功 / "0" 待开 / "1" 开具中 / "2" 失败
	InvalidFlag      string `json:"InvalidFlag"`      // "1" 已作废
	RedFlag          string `json:"RedFlag"`          // "1" 红票

	// 版式 URL（部分在测试环境是 preview 网页，不推荐做主下载源）
	DownloadUrl string `json:"DownloadUrl"`
	InvoiceOfd  string `json:"InvoiceOfd"`
	InvoiceXml  string `json:"InvoiceXml"`
	InvoiceImg  string `json:"InvoiceImg"`

	// 内嵌 base64 资源（推荐主路径用 PDF Base64）
	InvoicePdfBase64 string `json:"InvoicePdfBase64"`
	InvoiceOfdBase64 string `json:"InvoiceOfdBase64"`
	InvoiceXmlBase64 string `json:"InvoiceXmlBase64"`

	BillingDate  string `json:"BillingDate"`
	InvoiceState string `json:"InvoiceState"`
	InvoiceType  string `json:"InvoiceType"`
}

// ReturnReceiptStorageRequest 对应 POST /xxp/returnReceipts/storage（2.2 退货单存储）。
//
// **注意字段命名是 camelCase**（与 createInvoice 的 CapitalCase 不同！）。
//
// 文档「该接口入数后，财云通平台会自动进行红字信息单的填开、红票的开具」——
// 推送一次即可，平台后台自动完成红字信息单 + 红票出具，无需我们手动多步推进。
type ReturnReceiptStorageRequest struct {
	RequestID string                  `json:"requestID"`
	Count     int                     `json:"count"`
	Content   []ReturnReceiptContent  `json:"content"`
}

// ReturnReceiptContent 单张退货单。金额一律负数（红冲规则）。
type ReturnReceiptContent struct {
	BillNo            string `json:"billNo"`            // 红票订单号（唯一），如 REV-{invoiceID}-{nano}
	SellerTaxNum      string `json:"sellerTaxNum"`
	Seller            string `json:"seller"`
	SellerAddress     string `json:"sellerAddress"`
	SellerPhone       string `json:"sellerPhone"`
	SellerBankName    string `json:"sellerBankName"`
	SellerBankAccount string `json:"sellerBankAccount"`

	Buyer            string `json:"buyer"`
	BuyerTaxNum      string `json:"buyerTaxNum,omitempty"`
	BuyerAddress     string `json:"buyerAddress,omitempty"`
	BuyerPhone       string `json:"buyerPhone,omitempty"`
	BuyerEmail       string `json:"buyerEmail,omitempty"`
	BuyerBankName    string `json:"buyerBankName,omitempty"`
	BuyerBankAccount string `json:"buyerBankAccount,omitempty"`

	Drawer   string `json:"drawer"`
	Payee    string `json:"payee,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`

	InvoiceType string `json:"invoiceType"` // 同蓝票 InvoiceType (05/06/04/10/01/08)
	BillNoOld   string `json:"billNoOld,omitempty"`

	// 关键：必须传原蓝票号 + 蓝票开具日期（数电 05/06 可不传 BlueInvoiceCode）
	BlueInvoiceCode string `json:"blueInvoiceCode,omitempty"` // 数电票非必填
	BlueInvoiceNo   string `json:"blueInvoiceNo"`
	BlueInvoiceDate string `json:"blueInvoiceDate"` // yyyyMMdd

	TotalAmount   string `json:"totalAmount"`   // 负数
	InvoiceAmount string `json:"invoiceAmount"` // 负数
	TaxAmount     string `json:"taxAmount"`     // 负数

	Remarks   string `json:"remarks,omitempty"`
	StoreCode string `json:"storeCode,omitempty"`
	BillDate  string `json:"billDate"` // yyyyMMddHHmmss

	// 处理方式
	TreatmentType string `json:"treatmentType,omitempty"` // 数电默认 "1" 自动；税控默认 "0" 手动
	ApplyWay      string `json:"applyWay"`                // 必填："0" 全部冲红 / "1" 部分红冲
	RedSeason     string `json:"redSeason,omitempty"`     // "01" 开票有误 / "02" 销货退回 / "03" 服务中止 / "04" 销售折让

	// 红字信息单（手工填好时可直接带）
	RedAdviceNum  string `json:"redAdviceNum,omitempty"`
	RedConfirmNum string `json:"redConfirmNum,omitempty"`

	// 作废通道（蓝票当月、税控 01/04 时使用）
	VoidedPerson string `json:"voidedPerson,omitempty"`
	VoidedReason string `json:"voidedReason,omitempty"`

	Items []ReturnReceiptItem `json:"items"`
}

// ReturnReceiptItem 退货单明细。同样 camelCase + 金额负数。
type ReturnReceiptItem struct {
	LineNo        int    `json:"lineNo,omitempty"`
	DetailName    string `json:"detailName"`
	Unit          string `json:"unit,omitempty"`
	Standard      string `json:"standard,omitempty"`
	Num           string `json:"num,omitempty"`
	TaxFlag       string `json:"taxFlag"` // 1 含税 0 不含税
	Price         string `json:"price,omitempty"`
	InvoiceAmount string `json:"invoiceAmount"` // 负数
	SumAmount     string `json:"sumAmount"`     // 负数
	TaxRate       string `json:"taxRate"`
	TaxAmount     string `json:"taxAmount"` // 负数
	DiscountType  string `json:"discountType"`
	GoodsCode     string `json:"goodsCode,omitempty"`
	ParentCode    string `json:"parentCode,omitempty"`
	FreeTax       string `json:"freeTax,omitempty"`
}

// ReturnReceiptStorageResponse 退货单存储响应。Code "200" 表示成功受理。
type ReturnReceiptStorageResponse struct {
	HostID    string `json:"HostID"`
	RequestID string `json:"RequestID"`
	Code      string `json:"Code"`
	Message   string `json:"Message"`
	ErrorDatas []struct {
		BillNo    string `json:"billNo"`
		ErrorDesc string `json:"errorDesc"`
		ErrorCode string `json:"errorCode"`
	} `json:"ErrorDatas"`
}
