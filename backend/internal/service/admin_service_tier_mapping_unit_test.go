package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGroupTierMapping(t *testing.T) {
	t.Run("non-domestic template clears mapping", func(t *testing.T) {
		got := normalizeGroupTierMapping(ConfigTemplateClaudeNative, domain.GroupTierMapping{
			Default: "qwen-plus",
			Haiku:   "qwen-flash",
		})
		require.True(t, got.IsEmpty(), "claude_native should drop tier_mapping")

		got = normalizeGroupTierMapping("", domain.GroupTierMapping{Default: "x"})
		require.True(t, got.IsEmpty(), "empty template should drop tier_mapping")
	})

	t.Run("domestic_anthropic preserves and trims", func(t *testing.T) {
		got := normalizeGroupTierMapping(ConfigTemplateDomesticAnthropic, domain.GroupTierMapping{
			Default: "  qwen-plus  ",
			Haiku:   " qwen-flash",
			Sonnet:  "qwen-plus ",
			Opus:    "qwen-max",
		})
		require.Equal(t, "qwen-plus", got.Default)
		require.Equal(t, "qwen-flash", got.Haiku)
		require.Equal(t, "qwen-plus", got.Sonnet)
		require.Equal(t, "qwen-max", got.Opus)
	})

	t.Run("partial fill is permitted", func(t *testing.T) {
		got := normalizeGroupTierMapping(ConfigTemplateDomesticAnthropic, domain.GroupTierMapping{
			Default: "qwen-plus",
		})
		require.Equal(t, "qwen-plus", got.Default)
		require.Empty(t, got.Haiku)
		require.Empty(t, got.Sonnet)
		require.Empty(t, got.Opus)
		require.False(t, got.IsEmpty())
	})

	t.Run("empty input stays empty", func(t *testing.T) {
		got := normalizeGroupTierMapping(ConfigTemplateDomesticAnthropic, domain.GroupTierMapping{})
		require.True(t, got.IsEmpty())
	})
}
