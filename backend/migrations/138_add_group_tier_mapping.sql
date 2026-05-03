-- 138: 为 groups 表新增 tier_mapping 字段
-- 仅在 config_template=domestic_anthropic 时使用，决定"使用密钥 / 导入到 CCS"
-- 时写入 Claude Code env 的具体模型名（ANTHROPIC_MODEL / ANTHROPIC_DEFAULT_HAIKU_MODEL /
-- ANTHROPIC_DEFAULT_SONNET_MODEL / ANTHROPIC_DEFAULT_OPUS_MODEL）。
-- 与网关侧的 model_mapping、订阅计费逻辑无关。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS tier_mapping JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN groups.tier_mapping IS '国产 Anthropic 协议组的 Claude tier → 国产模型映射（导入到 CCS 时使用）';
