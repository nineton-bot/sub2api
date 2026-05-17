package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// usdPtr 让测试里写限额更简洁。
func usdPtr(v float64) *float64 { return &v }

// makeSub 构造一张满足 ValidateAndCheckLimits 入口条件（active、未过期）的 sub。
func makeSub(id int64, expiresIn time.Duration, monthlyUsage float64, group *Group) UserSubscription {
	now := time.Now()
	monthStart := now.Add(-time.Hour) // 月度窗口已激活但未到期
	return UserSubscription{
		ID:                 id,
		UserID:             1,
		GroupID:            group.ID,
		Status:             SubscriptionStatusActive,
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(expiresIn),
		DailyWindowStart:   &monthStart,
		WeeklyWindowStart:  &monthStart,
		MonthlyWindowStart: &monthStart,
		MonthlyUsageUSD:    monthlyUsage,
		Group:              group,
	}
}

// TestPickUsableSubscriptionFIFO_PicksFirstNonExhausted 验证叠加套餐场景下，
// 当 (user, group) 下存在多张 active sub 时，picker 按 expires_at ASC 跳过已耗尽配额的那张。
// 这是用户场景的核心：第一张月度配额用光 → 第二张要被自动选中而不需要重绑密钥。
func TestPickUsableSubscriptionFIFO_PicksFirstNonExhausted(t *testing.T) {
	svc := &SubscriptionService{} // ValidateAndCheckLimits 不需要 repo/cache
	group := &Group{
		ID:                  10,
		SubscriptionType:    SubscriptionTypeSubscription,
		MonthlyLimitUSD:     usdPtr(100),
	}

	// list 已按 expires_at ASC 排序：第一张先过期但月度配额已满，第二张未耗尽
	list := []UserSubscription{
		makeSub(1, 5*24*time.Hour, 100.01, group), // 配额耗尽（> limit）
		makeSub(2, 30*24*time.Hour, 0.0, group),  // 全新
	}

	idx := pickUsableSubscriptionIndex(svc, list)
	require.Equal(t, 1, idx, "picker should skip exhausted first sub and pick second")
	require.Equal(t, int64(2), list[idx].ID)
}

// TestPickUsableSubscriptionFIFO_AllExhaustedReturnsEarliest 确认全部超限时回退到最早过期那张，
// 让上游 ValidateAndCheckLimits 抛出标准的超限错误（用户感知不变）。
func TestPickUsableSubscriptionFIFO_AllExhaustedReturnsEarliest(t *testing.T) {
	svc := &SubscriptionService{}
	group := &Group{
		ID:                  10,
		SubscriptionType:    SubscriptionTypeSubscription,
		MonthlyLimitUSD:     usdPtr(100),
	}

	list := []UserSubscription{
		makeSub(1, 5*24*time.Hour, 100.01, group),
		makeSub(2, 30*24*time.Hour, 100.01, group),
	}

	idx := pickUsableSubscriptionIndex(svc, list)
	require.Equal(t, 0, idx, "all exhausted → picker should return earliest expiry (index 0)")
}

// TestPickUsableSubscriptionFIFO_SinglePicksThatOne 退化场景：仅一张可用，无论是否耗尽都返回它。
func TestPickUsableSubscriptionFIFO_SinglePicksThatOne(t *testing.T) {
	svc := &SubscriptionService{}
	group := &Group{
		ID:               10,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  usdPtr(100),
	}

	list := []UserSubscription{makeSub(1, 30*24*time.Hour, 50.0, group)}
	require.Equal(t, 0, pickUsableSubscriptionIndex(svc, list))

	// 即使耗尽也返回它（让超限错误从 ValidateAndCheckLimits 抛出）
	list = []UserSubscription{makeSub(1, 30*24*time.Hour, 100.01, group)}
	require.Equal(t, 0, pickUsableSubscriptionIndex(svc, list))
}

// TestPickUsableSubscriptionFIFO_EmptyList 空列表保护
func TestPickUsableSubscriptionFIFO_EmptyList(t *testing.T) {
	svc := &SubscriptionService{}
	require.Equal(t, 0, pickUsableSubscriptionIndex(svc, nil))
	require.Equal(t, 0, pickUsableSubscriptionIndex(svc, []UserSubscription{}))
}
