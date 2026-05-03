package domain

// GroupTierMapping 控制 domestic_anthropic 配置模板下，"使用密钥"导入 CC Switch 时
// 写入 Claude Code env 的目标模型名（CC 会读 ANTHROPIC_MODEL / ANTHROPIC_DEFAULT_HAIKU_MODEL /
// ANTHROPIC_DEFAULT_SONNET_MODEL / ANTHROPIC_DEFAULT_OPUS_MODEL 等环境变量）。
//
// 留空表示该 tier 不发到 CCSwitch。前端可在 tier 全空时退化为只填 Default 字段。
type GroupTierMapping struct {
	Default string `json:"default,omitempty"`
	Haiku   string `json:"haiku,omitempty"`
	Sonnet  string `json:"sonnet,omitempty"`
	Opus    string `json:"opus,omitempty"`
}

// IsEmpty 报告映射是否完全未配置。
func (m GroupTierMapping) IsEmpty() bool {
	return m.Default == "" && m.Haiku == "" && m.Sonnet == "" && m.Opus == ""
}
