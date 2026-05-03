// referral-backfill 一次性回填工具：把因 unified billing 路径漏挂释放钩子（修复见
// gateway_service.go asyncReleaseRechargeCommission）期间累积的"已消费但未归因"
// 充值型佣金一次性补齐。
//
// 算法（按被邀请人粒度）：
//   1. 取 referral_commissions 中所有 status IN (accruing,partial_reversed) 且
//      source_type='recharge' 的去重 referee_id
//   2. 每个 referee：
//        total_consumed     = SUM(usage_logs.actual_cost  WHERE user_id=referee_id)
//        already_attributed = SUM(referral_commissions.consumed_attributed
//                                 WHERE referee_id=referee_id AND source_type='recharge')
//        delta              = total_consumed - already_attributed
//      若 delta > 0，调用 ReleaseCommissionForRechargeConsumption(referee_id, delta)
//   3. release 函数内部按 FIFO 把 delta 归因到最早未占满的 commission，自然 cap 到
//      source_amount，多余的部分被丢弃 —— 跟正常实时路径完全等价
//
// 幂等：再次运行时 already_attributed 会反映上一次的回填结果，delta=0 → 跳过。
//
// 使用：
//   # dry-run（默认；不写入，只打印将释放的金额）
//   ./referral-backfill
//
//   # 仅模拟某个被邀请人
//   ./referral-backfill --referee-id 706
//
//   # 真正执行
//   ./referral-backfill --apply
//
//   # 真正执行且只处理一个被邀请人（用于先小流量验证）
//   ./referral-backfill --apply --referee-id 706
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func main() {
	apply := flag.Bool("apply", false, "Actually perform releases (default: dry-run that prints the plan only)")
	refereeID := flag.Int64("referee-id", 0, "If non-zero, only process this single referee (useful for incremental verification)")
	timeoutSec := flag.Int("timeout", 600, "Overall command timeout in seconds")
	flag.Parse()

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	client, sqlDB, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer func() { _ = client.Close() }()

	userRepo := repository.NewUserRepository(client, sqlDB)
	settingRepo := repository.NewSettingRepository(client)
	settingSvc := service.NewSettingService(settingRepo, cfg)

	// billingCacheService 与 authCacheInvalidator 都传 nil：本工具脱离 HTTP 服务上下文运行，
	// 缓存自然 TTL 过期或下次请求触发刷新即可，没必要打通到运行中的 Redis。
	// invalidateUserCache 内部对 nil 做了防御性判空。
	referralSvc := service.NewReferralService(client, sqlDB, userRepo, settingSvc, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSec)*time.Second)
	defer cancel()

	rows, err := loadReferees(ctx, client, *refereeID)
	if err != nil {
		log.Fatalf("scan referees: %v", err)
	}
	if len(rows) == 0 {
		fmt.Println("No referees with stuck recharge commissions. Nothing to backfill.")
		return
	}

	fmt.Printf("Found %d candidate referee(s) (apply=%v)\n", len(rows), *apply)
	fmt.Printf("%-10s %-10s %-18s %-18s %-18s\n", "REFEREE", "REFERRER", "ALREADY_ATTR", "TOTAL_CONSUMED", "DELTA_TO_RELEASE")

	var totalDelta float64
	var processed, skipped, failed int

	for _, r := range rows {
		delta := r.TotalConsumed - r.AlreadyAttributed
		if delta <= 0 {
			fmt.Printf("%-10d %-10d %-18.4f %-18.4f %-18s (skip: delta<=0)\n",
				r.RefereeID, r.ReferrerID, r.AlreadyAttributed, r.TotalConsumed, "0")
			skipped++
			continue
		}
		fmt.Printf("%-10d %-10d %-18.4f %-18.4f %-18.4f",
			r.RefereeID, r.ReferrerID, r.AlreadyAttributed, r.TotalConsumed, delta)
		totalDelta += delta

		if !*apply {
			fmt.Println("  (dry-run)")
			continue
		}

		if err := referralSvc.ReleaseCommissionForRechargeConsumption(ctx, r.RefereeID, delta); err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			failed++
			continue
		}
		fmt.Println("  OK")
		processed++
	}

	fmt.Println()
	fmt.Printf("=== Summary ===\n")
	fmt.Printf("Candidates: %d   Skipped (delta<=0): %d   Released: %d   Failed: %d\n",
		len(rows), skipped, processed, failed)
	fmt.Printf("Total delta consumption fed into release: %.4f USD\n", totalDelta)
	if !*apply {
		fmt.Println("This was a DRY-RUN. Re-run with --apply to actually release.")
	}
}

type refereeRow struct {
	RefereeID         int64
	ReferrerID        int64
	AlreadyAttributed float64
	TotalConsumed     float64
}

// loadReferees 一次性聚合查询：受影响的被邀请人 + 已归因 + 已实际消费。
//
// 用单条 SQL 而不是逐 referee 查，避免 N 次 round-trip。规模在万级以内时一次查
// 内存压力可忽略；本工具的目标场景预计 <1 万 referee。
func loadReferees(ctx context.Context, client *dbent.Client, onlyRefereeID int64) ([]refereeRow, error) {
	const baseSQL = `
WITH affected AS (
    SELECT DISTINCT referee_id, referrer_id
    FROM referral_commissions
    WHERE source_type = 'recharge'
      AND status IN ('accruing','partial_reversed')
      AND referee_id IS NOT NULL
      AND referrer_id IS NOT NULL
),
attribution AS (
    SELECT referee_id, COALESCE(SUM(consumed_attributed), 0) AS already_attributed
    FROM referral_commissions
    WHERE source_type = 'recharge'
    GROUP BY referee_id
),
consumption AS (
    SELECT user_id AS referee_id, COALESCE(SUM(actual_cost), 0) AS total_consumed
    FROM usage_logs
    WHERE user_id IN (SELECT referee_id FROM affected)
    GROUP BY user_id
)
SELECT a.referee_id,
       a.referrer_id,
       COALESCE(at.already_attributed, 0)::float8,
       COALESCE(c.total_consumed, 0)::float8
FROM affected a
LEFT JOIN attribution at ON at.referee_id = a.referee_id
LEFT JOIN consumption  c  ON c.referee_id = a.referee_id
`
	query := baseSQL + "WHERE ($1::bigint = 0 OR a.referee_id = $1) ORDER BY a.referee_id"

	rows, err := client.QueryContext(ctx, query, onlyRefereeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []refereeRow
	for rows.Next() {
		var r refereeRow
		if err := rows.Scan(&r.RefereeID, &r.ReferrerID, &r.AlreadyAttributed, &r.TotalConsumed); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
