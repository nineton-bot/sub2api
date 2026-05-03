package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupFromService_OmitsEmptyTierMapping(t *testing.T) {
	t.Parallel()

	g := &service.Group{
		ID:       1,
		Name:     "claude-native",
		Platform: service.PlatformAnthropic,
		Status:   service.StatusActive,
		// TierMapping zero-value
	}

	out := GroupFromService(g)
	require.NotNil(t, out)
	require.Nil(t, out.TierMapping, "claude_native group should have nil tier_mapping pointer")

	body, err := json.Marshal(out)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(body), "tier_mapping"),
		"empty tier_mapping must not appear in JSON: %s", body)
}

func TestGroupFromService_PreservesPopulatedTierMapping(t *testing.T) {
	t.Parallel()

	g := &service.Group{
		ID:             1,
		Name:           "domestic-anthropic",
		Platform:       service.PlatformAnthropic,
		Status:         service.StatusActive,
		ConfigTemplate: service.ConfigTemplateDomesticAnthropic,
		TierMapping: domain.GroupTierMapping{
			Default: "qwen-plus",
			Haiku:   "qwen-flash",
			Sonnet:  "qwen-plus",
			Opus:    "qwen-max",
		},
	}

	out := GroupFromService(g)
	require.NotNil(t, out)
	require.NotNil(t, out.TierMapping)
	require.Equal(t, "qwen-plus", out.TierMapping.Default)
	require.Equal(t, "qwen-flash", out.TierMapping.Haiku)
	require.Equal(t, "qwen-plus", out.TierMapping.Sonnet)
	require.Equal(t, "qwen-max", out.TierMapping.Opus)

	body, err := json.Marshal(out)
	require.NoError(t, err)
	require.Contains(t, string(body), `"tier_mapping":{"default":"qwen-plus","haiku":"qwen-flash","sonnet":"qwen-plus","opus":"qwen-max"}`)
}

func TestGroupFromServiceAdmin_AlsoOmitsEmptyTierMapping(t *testing.T) {
	t.Parallel()

	g := &service.Group{
		ID:       2,
		Name:     "openai-grp",
		Platform: service.PlatformOpenAI,
		Status:   service.StatusActive,
	}

	out := GroupFromServiceAdmin(g)
	require.NotNil(t, out)
	require.Nil(t, out.TierMapping)

	body, err := json.Marshal(out)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(body), "tier_mapping"),
		"empty tier_mapping must be omitted on admin DTO: %s", body)
}
