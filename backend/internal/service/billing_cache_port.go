package service

import (
	"time"
)

// SubscriptionCacheData represents cached subscription data
type SubscriptionCacheData struct {
	Status              string
	ExpiresAt           time.Time
	SubscriptionMeter   string
	DailyWindowStart    *time.Time
	WeeklyWindowStart   *time.Time
	MonthlyWindowStart  *time.Time
	DailyUsage          float64
	WeeklyUsage         float64
	MonthlyUsage        float64
	DailyRequestCount   int
	WeeklyRequestCount  int
	MonthlyRequestCount int
	Version             int64
}
