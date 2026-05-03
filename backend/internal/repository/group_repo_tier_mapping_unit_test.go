package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupEntityToService_PreservesTierMapping(t *testing.T) {
	group := &dbent.Group{
		ID:               1,
		Name:             "domestic-anthropic",
		Platform:         service.PlatformAnthropic,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		RateMultiplier:   1,
		ConfigTemplate:   service.ConfigTemplateDomesticAnthropic,
		TierMapping: domain.GroupTierMapping{
			Default: "qwen-plus",
			Haiku:   "qwen-flash",
			Sonnet:  "qwen-plus",
			Opus:    "qwen-max",
		},
	}

	got := groupEntityToService(group)
	require.NotNil(t, got)
	require.Equal(t, group.TierMapping, got.TierMapping)
	require.Equal(t, "qwen-flash", got.TierMapping.Haiku)
	require.Equal(t, "qwen-plus", got.TierMapping.Sonnet)
	require.Equal(t, "qwen-max", got.TierMapping.Opus)
	require.Equal(t, "qwen-plus", got.TierMapping.Default)
}

func TestGroupTierMapping_IsEmpty(t *testing.T) {
	require.True(t, domain.GroupTierMapping{}.IsEmpty())
	require.False(t, domain.GroupTierMapping{Default: "qwen-plus"}.IsEmpty())
	require.False(t, domain.GroupTierMapping{Haiku: "qwen-flash"}.IsEmpty())
}
