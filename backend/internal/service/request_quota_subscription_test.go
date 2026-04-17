package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateSubscriptionMeterConfig_RequestQuotaRequiresAllLimits(t *testing.T) {
	err := validateSubscriptionMeterConfig(
		SubscriptionTypeSubscription,
		SubscriptionMeterRequestQuota,
		nil,
		nil,
		nil,
		testIntPtr(100),
		nil,
		testIntPtr(300),
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "daily_request_limit")
}

func TestSubscriptionServiceValidateAndCheckLimits_RequestQuotaBlocksAtLimit(t *testing.T) {
	svc := &SubscriptionService{}
	now := time.Now()
	sub := &UserSubscription{
		Status:              SubscriptionStatusActive,
		ExpiresAt:           now.Add(time.Hour),
		DailyWindowStart:    &now,
		WeeklyWindowStart:   &now,
		MonthlyWindowStart:  &now,
		DailyRequestCount:   11,
		WeeklyRequestCount:  20,
		MonthlyRequestCount: 30,
	}
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		SubscriptionMeter:   SubscriptionMeterRequestQuota,
		DailyRequestLimit:   testIntPtr(10),
		WeeklyRequestLimit:  testIntPtr(25),
		MonthlyRequestLimit: testIntPtr(35),
	}

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)

	require.False(t, needsMaintenance)
	require.ErrorIs(t, err, ErrDailyLimitExceeded)
}

func TestSubscriptionServiceValidateAndCheckLimits_RequestQuotaResetsExpiredCountersInMemory(t *testing.T) {
	svc := &SubscriptionService{}
	now := time.Now()
	pastDay := now.Add(-25 * time.Hour)
	pastWeek := now.Add(-8 * 24 * time.Hour)
	pastMonth := now.Add(-31 * 24 * time.Hour)
	sub := &UserSubscription{
		Status:              SubscriptionStatusActive,
		ExpiresAt:           now.Add(time.Hour),
		DailyWindowStart:    &pastDay,
		WeeklyWindowStart:   &pastWeek,
		MonthlyWindowStart:  &pastMonth,
		DailyRequestCount:   10,
		WeeklyRequestCount:  20,
		MonthlyRequestCount: 30,
	}
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		SubscriptionMeter:   SubscriptionMeterRequestQuota,
		DailyRequestLimit:   testIntPtr(10),
		WeeklyRequestLimit:  testIntPtr(20),
		MonthlyRequestLimit: testIntPtr(30),
	}

	needsMaintenance, err := svc.ValidateAndCheckLimits(sub, group)

	require.True(t, needsMaintenance)
	require.NoError(t, err)
	require.Zero(t, sub.DailyRequestCount)
	require.Zero(t, sub.WeeklyRequestCount)
	require.Zero(t, sub.MonthlyRequestCount)
}

// testIntPtr is shared across service tests regardless of build tag.
func testIntPtr(i int) *int { return &i }

