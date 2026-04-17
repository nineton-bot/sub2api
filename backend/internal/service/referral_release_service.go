package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// ReferralReleaseService periodically recalculates subscription-type referral commissions.
//
// 充值型返佣在扣费钩子里实时释放，而订阅型佣金按天数线性释放，需要有独立的定时任务
// 在没有新扣费事件时仍然把应释放的金额按时入账。本服务就是干这件事的 —— 每个
// interval 扫一次所有 accruing / partial_reversed 状态的订阅型 commission，并按
// 当前时间应释放的进度把差额入邀请人 balance。
type ReferralReleaseService struct {
	svc      *ReferralService
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewReferralReleaseService constructs the service. interval <= 0 disables the ticker.
func NewReferralReleaseService(svc *ReferralService, interval time.Duration) *ReferralReleaseService {
	return &ReferralReleaseService{
		svc:      svc,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the background loop. No-op if misconfigured.
func (s *ReferralReleaseService) Start() {
	if s == nil || s.svc == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		// 启动时立即跑一次，避免进程刚起来就刚好错过一个 interval。
		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Stop signals the loop to exit and waits for it to finish.
func (s *ReferralReleaseService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

// runOnce executes a single pass of the subscription commission recalc.
func (s *ReferralReleaseService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.svc.RecalcSubscriptionCommissions(ctx); err != nil {
		log.Printf("[ReferralRelease] recalc subscription commissions failed: %v", err)
	}
}
