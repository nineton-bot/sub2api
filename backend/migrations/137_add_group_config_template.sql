-- 106: 为 groups 表新增 config_template 字段
-- 用于在 anthropic 平台下区分 "Claude 原生" 和 "国产模型（Anthropic 协议）" 两种配置模板
-- 影响前端 UseKeyModal 生成的 openclaw providers / Claude Code settings.json

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS config_template VARCHAR(32) NOT NULL DEFAULT 'claude_native';

COMMENT ON COLUMN groups.config_template IS '使用密钥弹窗配置模板：claude_native / domestic_anthropic';
