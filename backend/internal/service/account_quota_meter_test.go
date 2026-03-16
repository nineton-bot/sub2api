package service

import "testing"

func TestAccountGetQuotaMeter_DefaultCost(t *testing.T) {
	account := &Account{}
	if got := account.GetQuotaMeter(); got != AccountQuotaMeterCost {
		t.Fatalf("expected default quota meter %q, got %q", AccountQuotaMeterCost, got)
	}
}

func TestAccountGetQuotaMeter_Requests(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			"quota_meter": AccountQuotaMeterRequests,
		},
	}
	if got := account.GetQuotaMeter(); got != AccountQuotaMeterRequests {
		t.Fatalf("expected quota meter %q, got %q", AccountQuotaMeterRequests, got)
	}
}

func TestAccountQuotaDelta_RequestMeterUsesOne(t *testing.T) {
	account := &Account{
		Type: AccountTypeAPIKey,
		Extra: map[string]any{
			"quota_meter":       AccountQuotaMeterRequests,
			"quota_daily_limit": 10,
		},
	}

	if got := accountQuotaDelta(account, &CostBreakdown{TotalCost: 123.45}, 2); got != 1 {
		t.Fatalf("expected request-meter quota delta 1, got %v", got)
	}
}
