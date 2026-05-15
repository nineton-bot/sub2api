package caiyuntong

import "strconv"

// SettingKeys 列出所有需要从 SettingService 读取的 key（与 domain_constants.go 同名常量保持一致）。
// 这里冗余一份字符串字面量是为了避免 caiyuntong 包反向依赖 service 包。
// 修改 service.SettingKeyInvoiceCaiyuntong* 时务必同步本切片。
var SettingKeys = []string{
	"invoice_caiyuntong_endpoint",
	"invoice_caiyuntong_access_key_id",
	"invoice_caiyuntong_access_key_secret",
	"invoice_caiyuntong_seller_tax_num",
	"invoice_caiyuntong_seller_name",
	"invoice_caiyuntong_seller_address",
	"invoice_caiyuntong_seller_phone",
	"invoice_caiyuntong_seller_bank_name",
	"invoice_caiyuntong_seller_bank_account",
	"invoice_caiyuntong_drawer",
	"invoice_caiyuntong_payee",
	"invoice_caiyuntong_reviewer",
	"invoice_caiyuntong_type_normal",
	"invoice_caiyuntong_type_special",
	"invoice_caiyuntong_goods_code_default",
	"invoice_caiyuntong_default_tax_rate",
}

// LoadConfig 从一个 settings map (来自 SettingService.GetMultiple) 构造 Config。
// 完全不依赖 service 包。
func LoadConfig(settings map[string]string) Config {
	rate, _ := strconv.ParseFloat(settings["invoice_caiyuntong_default_tax_rate"], 64)
	return Config{
		Endpoint:         settings["invoice_caiyuntong_endpoint"],
		AccessKeyID:      settings["invoice_caiyuntong_access_key_id"],
		AccessKeySecret:  settings["invoice_caiyuntong_access_key_secret"],
		SellerTaxNum:     settings["invoice_caiyuntong_seller_tax_num"],
		SellerName:       settings["invoice_caiyuntong_seller_name"],
		SellerAddress:    settings["invoice_caiyuntong_seller_address"],
		SellerPhone:      settings["invoice_caiyuntong_seller_phone"],
		SellerBankName:   settings["invoice_caiyuntong_seller_bank_name"],
		SellerBankAcc:    settings["invoice_caiyuntong_seller_bank_account"],
		Drawer:           settings["invoice_caiyuntong_drawer"],
		Payee:            settings["invoice_caiyuntong_payee"],
		Reviewer:         settings["invoice_caiyuntong_reviewer"],
		TypeForNormal:    settings["invoice_caiyuntong_type_normal"],
		TypeForSpecial:   settings["invoice_caiyuntong_type_special"],
		GoodsCodeDefault: settings["invoice_caiyuntong_goods_code_default"],
		DefaultTaxRate:   rate,
	}
}

// InvoiceTypeFor 按票种返回对应的 InvoiceType 代码。
// kind: "normal" | "special"。未配置时回 fallback。
func (c Config) InvoiceTypeFor(kind string) string {
	switch kind {
	case "special":
		if c.TypeForSpecial != "" {
			return c.TypeForSpecial
		}
		return "05"
	default: // normal
		if c.TypeForNormal != "" {
			return c.TypeForNormal
		}
		return "06"
	}
}

// Validate 必填项检查（供 admin Settings 保存时做粗筛）。
func (c Config) Validate() error {
	if c.Endpoint == "" || c.AccessKeyID == "" || c.AccessKeySecret == "" {
		return errMissingCore
	}
	if c.SellerTaxNum == "" {
		return errMissingSellerTax
	}
	return nil
}

var (
	errMissingCore      = &configError{"caiyuntong: endpoint / access_key_id / access_key_secret are required"}
	errMissingSellerTax = &configError{"caiyuntong: seller_tax_num is required"}
)

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }
