// Package service — Referral (邀请返佣) business logic.
//
// 本文件实现 plan 中 "邀请返佣激励机制" 的服务层全部逻辑，遵循 plan 中
// 定义的精确规则：
//
//   1. 佣金分为账面 (gross) 与可用 (released)；后者直接并入 balance。
//   2. 充值型佣金随被邀请人余额消费 FIFO 释放；订阅型按天释放（不满 1 天按 1 天）。
//   3. 退款按实际消费/保留天数重算 gross，已释放部分不回收。
//   4. 被邀请人延迟赠金：注册记录 pending，首次充值 / 订阅履约成功后入账。
//   5. 三个设置项控制整个系统：referral_enabled / rate / referee_bonus_amount。
package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/referralcommission"
	"github.com/Wei-Shaw/sub2api/ent/referralpendingbonus"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// --- 错误定义 ---

var (
	ErrReferralDisabled       = infraerrors.BadRequest("REFERRAL_DISABLED", "referral program is disabled")
	ErrReferrerNotFound       = infraerrors.NotFound("REFERRER_NOT_FOUND", "invite code is invalid")
	ErrReferralSelfReferrer   = infraerrors.BadRequest("REFERRAL_SELF", "cannot refer yourself")
	ErrReferralAlreadyBound   = infraerrors.Conflict("REFERRAL_ALREADY_BOUND", "already has a referrer")
	ErrReferralInviteGenLimit = infraerrors.InternalServer("REFERRAL_CODE_GEN_FAILED", "failed to generate invite code")
)

// --- 常量 ---

const (
	// 邀请码字符集：大小写字母 + 数字（去掉容易混淆的 0/O/1/l/I）
	inviteCodeCharset = "ABCDEFGHJKMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"
	inviteCodeLen     = 8
	// 邀请码生成重试次数（碰撞概率极低）
	inviteCodeMaxRetry = 10

	// referral_commissions.source_type
	ReferralSourceRecharge     = "recharge"
	ReferralSourceSubscription = "subscription"

	// referral_commissions.status
	ReferralStatusAccruing        = "accruing"
	ReferralStatusFullyReleased   = "fully_released"
	ReferralStatusReversed        = "reversed"
	ReferralStatusPartialReversed = "partial_reversed"

	// referral_pending_bonuses.status
	ReferralBonusPending = "pending"
	ReferralBonusGranted = "granted"

	ReferralBonusTriggerRecharge     = "first_recharge"
	ReferralBonusTriggerSubscription = "first_subscription"
)

// --- 数据结构 ---

// ReferralStats 用户视角的邀请统计
type ReferralStats struct {
	InviteCode         string  `json:"invite_code"`
	InvitedCount       int64   `json:"invited_count"`        // 成功邀请人数（users.invited_by_user_id）
	GrossCommission    float64 `json:"gross_commission"`     // 累计账面佣金
	ReleasedCommission float64 `json:"released_commission"`  // 累计已释放
	PendingCommission  float64 `json:"pending_commission"`   // gross − released
	CommissionRate     float64 `json:"commission_rate"`      // 当前比例（snapshot 设置）
	RefereeBonusAmount float64 `json:"referee_bonus_amount"` // 被邀请人首次付费赠金额度
}

// CommissionLog 返佣明细行
type CommissionLog struct {
	ID                  int64     `json:"id"`
	RefereeID           int64     `json:"referee_id"`
	RefereeEmailMasked  string    `json:"referee_email"`
	SourceType          string    `json:"source_type"` // recharge | subscription
	SourceOrderID       int64     `json:"source_order_id"`
	SourceAmount        float64   `json:"source_amount"`
	CommissionRate      float64   `json:"commission_rate"`
	GrossCommission     float64   `json:"gross_commission"`
	ReleasedCommission  float64   `json:"released_commission"`
	Status              string    `json:"status"`
	SourceValidityDays  *int      `json:"source_validity_days,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// GlobalReferralStats 管理端总览
type GlobalReferralStats struct {
	TotalInvitedUsers    int64   `json:"total_invited_users"`   // 有 invited_by_user_id 的用户数
	TotalReleased        float64 `json:"total_released"`        // SUM(released_commission)
	TotalPending         float64 `json:"total_pending"`         // SUM(gross − released)
	TotalGrossCommission float64 `json:"total_gross_commission"`
	RefereeBonusGranted  int64   `json:"referee_bonus_granted"` // 已发放的 pending_bonus 数
	RefereeBonusPending  int64   `json:"referee_bonus_pending"`
	CommissionRate       float64 `json:"commission_rate"`
	RefereeBonusAmount   float64 `json:"referee_bonus_amount"`
	Enabled              bool    `json:"enabled"`
}

// ReferrerRank 管理端 top referrer 排行项
type ReferrerRank struct {
	UserID             int64   `json:"user_id"`
	Email              string  `json:"email"`
	Username           string  `json:"username"`
	InvitedCount       int64   `json:"invited_count"`
	GrossCommission    float64 `json:"gross_commission"`
	ReleasedCommission float64 `json:"released_commission"`
}

// RefereeDrilldown 管理端下钻单个邀请人的被邀请人列表
type RefereeDrilldown struct {
	RefereeID     int64     `json:"referee_id"`
	Email         string    `json:"email"`
	Username      string    `json:"username"`
	JoinedAt      time.Time `json:"joined_at"`
	Gross         float64   `json:"gross_commission"`
	Released      float64   `json:"released_commission"`
	OrderCount    int64     `json:"order_count"`
	BonusGranted  bool      `json:"bonus_granted"`
}

// --- Service ---

// ReferralService 邀请返佣服务
type ReferralService struct {
	entClient            *dbent.Client
	sqlDB                *sql.DB
	userRepo             UserRepository
	settingService       *SettingService
	billingCacheService  *BillingCacheService
	authCacheInvalidator APIKeyAuthCacheInvalidator
}

// NewReferralService 创建邀请返佣服务实例
func NewReferralService(
	entClient *dbent.Client,
	sqlDB *sql.DB,
	userRepo UserRepository,
	settingService *SettingService,
	billingCacheService *BillingCacheService,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
) *ReferralService {
	return &ReferralService{
		entClient:            entClient,
		sqlDB:                sqlDB,
		userRepo:             userRepo,
		settingService:       settingService,
		billingCacheService:  billingCacheService,
		authCacheInvalidator: authCacheInvalidator,
	}
}

// --- 邀请码管理 ---

// EnsureInviteCode 幂等地确保用户存在可用邀请码；返回该邀请码。
//
// 老用户（在 referral 上线前注册）可能没有 invite_code，调用此方法会按需生成。
// 邀请码一旦生成不会变更（作为 URL 一部分需保持稳定）。
func (s *ReferralService) EnsureInviteCode(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", ErrUserNotFound
	}
	u, err := s.entClient.User.Query().Where(user.IDEQ(userID)).Only(ctx)
	if err != nil {
		return "", translateUserErr(err)
	}
	if u.InviteCode != nil && *u.InviteCode != "" {
		return *u.InviteCode, nil
	}

	// 生成唯一邀请码（最多重试 N 次）
	for i := 0; i < inviteCodeMaxRetry; i++ {
		code, gerr := generateInviteCode()
		if gerr != nil {
			return "", ErrReferralInviteGenLimit
		}
		// 用 Update 的 unique 约束检测冲突
		err = s.entClient.User.UpdateOneID(userID).SetInviteCode(code).Exec(ctx)
		if err == nil {
			return code, nil
		}
		// 冲突则重试；非冲突直接返回
		if !isUniqueViolation(err) {
			return "", fmt.Errorf("set invite_code: %w", err)
		}
	}
	return "", ErrReferralInviteGenLimit
}

// ResolveReferrerByCode 根据邀请码查邀请人。空码返回 nil, nil（不报错）。
func (s *ReferralService) ResolveReferrerByCode(ctx context.Context, code string) (*User, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil
	}
	u, err := s.entClient.User.Query().
		Where(user.InviteCodeEQ(code)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, ErrReferrerNotFound
		}
		return nil, fmt.Errorf("query referrer: %w", err)
	}
	if u.Status != StatusActive {
		return nil, ErrReferrerNotFound
	}
	out := &User{
		ID:       u.ID,
		Email:    u.Email,
		Username: u.Username,
		Status:   u.Status,
	}
	return out, nil
}

// BindReferrer 注册成功后绑定邀请关系；同时根据设置创建 pending 赠金。
//
// 宽松策略（同 promo_code）：
//   - 若 referral 功能未启用，静默返回 nil
//   - 若邀请码无效 / 指向自己，返回错误但调用方通常只记日志不阻塞
//   - 若被邀请人已绑定其他邀请人，返回 ErrReferralAlreadyBound
func (s *ReferralService) BindReferrer(ctx context.Context, refereeID int64, referrerCode string) error {
	referrerCode = strings.TrimSpace(referrerCode)
	if referrerCode == "" {
		return nil
	}
	if !s.settingService.IsReferralEnabled(ctx) {
		return nil
	}

	referrer, err := s.ResolveReferrerByCode(ctx, referrerCode)
	if err != nil {
		return err
	}
	if referrer == nil {
		return nil
	}
	if referrer.ID == refereeID {
		return ErrReferralSelfReferrer
	}

	bonusAmount := s.settingService.GetReferralRefereeBonusAmount(ctx)

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	// 锁用户行，检查是否已绑定过
	refereeEnt, err := client.User.Query().Where(user.IDEQ(refereeID)).ForUpdate().Only(txCtx)
	if err != nil {
		return translateUserErr(err)
	}
	if refereeEnt.InvitedByUserID != nil && *refereeEnt.InvitedByUserID > 0 {
		return ErrReferralAlreadyBound
	}
	if _, err := client.User.UpdateOneID(refereeID).SetInvitedByUserID(referrer.ID).Save(txCtx); err != nil {
		return fmt.Errorf("bind referrer: %w", err)
	}

	// 建立 pending 赠金（bonus=0 则不建记录）
	if bonusAmount > 0 {
		_, err := client.ReferralPendingBonus.Create().
			SetRefereeID(refereeID).
			SetReferrerID(referrer.ID).
			SetBonusAmount(bonusAmount).
			SetStatus(ReferralBonusPending).
			Save(txCtx)
		if err != nil && !isUniqueViolation(err) {
			return fmt.Errorf("create pending bonus: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	slog.Info("[Referral] bound referrer",
		"referee_id", refereeID, "referrer_id", referrer.ID,
		"bonus_amount", bonusAmount)
	return nil
}

// --- 返佣账务：充值 ---

// AccrueCommissionOnRecharge 在被邀请人充值履约成功后调用。
//
// 幂等（依赖 unique (source_order_id, source_type)）：重复调用不会重复入账。
// 若被邀请人没有邀请人或返佣关闭，直接返回 nil。
func (s *ReferralService) AccrueCommissionOnRecharge(ctx context.Context, order *dbent.PaymentOrder) error {
	if order == nil || order.OrderType != payment.OrderTypeBalance {
		return nil
	}
	if !s.settingService.IsReferralEnabled(ctx) {
		return nil
	}
	referrerID, err := s.referrerOf(ctx, order.UserID)
	if err != nil || referrerID == 0 {
		return err
	}

	rate := s.settingService.GetReferralCommissionRate(ctx)
	gross := roundMoney(order.Amount * rate)
	if gross <= 0 {
		return nil
	}

	_, err = s.entClient.ReferralCommission.Create().
		SetReferrerID(referrerID).
		SetRefereeID(order.UserID).
		SetSourceType(ReferralSourceRecharge).
		SetSourceOrderID(order.ID).
		SetSourceAmount(order.Amount).
		SetCommissionRate(rate).
		SetGrossCommission(gross).
		SetReleasedCommission(0).
		SetConsumedAttributed(0).
		SetStatus(ReferralStatusAccruing).
		Save(ctx)
	if err != nil {
		if isUniqueViolation(err) {
			return nil
		}
		return fmt.Errorf("create commission: %w", err)
	}
	slog.Info("[Referral] accrued recharge commission",
		"referee", order.UserID, "referrer", referrerID,
		"order", order.ID, "amount", order.Amount, "gross", gross)
	return nil
}

// --- 返佣账务：订阅 ---

// AccrueCommissionOnSubscription 在被邀请人订阅履约成功后调用。
//
// 订阅型佣金按天数线性释放，因此需要快照 validity_days 与 starts_at。
// 订阅 ID 可以通过最新活跃订阅获取（fulfillment 刚完成，必然存在）。
func (s *ReferralService) AccrueCommissionOnSubscription(
	ctx context.Context, order *dbent.PaymentOrder, subscriptionID int64, startsAt time.Time,
) error {
	if order == nil || order.OrderType != payment.OrderTypeSubscription {
		return nil
	}
	if !s.settingService.IsReferralEnabled(ctx) {
		return nil
	}
	if order.SubscriptionDays == nil || *order.SubscriptionDays <= 0 {
		return nil
	}
	referrerID, err := s.referrerOf(ctx, order.UserID)
	if err != nil || referrerID == 0 {
		return err
	}
	rate := s.settingService.GetReferralCommissionRate(ctx)
	gross := roundMoney(order.Amount * rate)
	if gross <= 0 {
		return nil
	}

	create := s.entClient.ReferralCommission.Create().
		SetReferrerID(referrerID).
		SetRefereeID(order.UserID).
		SetSourceType(ReferralSourceSubscription).
		SetSourceOrderID(order.ID).
		SetSourceAmount(order.Amount).
		SetCommissionRate(rate).
		SetGrossCommission(gross).
		SetReleasedCommission(0).
		SetConsumedAttributed(0).
		SetStatus(ReferralStatusAccruing).
		SetSourceValidityDays(*order.SubscriptionDays).
		SetSourceStartsAt(startsAt)
	if subscriptionID > 0 {
		create = create.SetSourceSubscriptionID(subscriptionID)
	}
	if _, err := create.Save(ctx); err != nil {
		if isUniqueViolation(err) {
			return nil
		}
		return fmt.Errorf("create subscription commission: %w", err)
	}

	// 刚创建的订阅型 commission 立即按当前时间点做一次释放（即使只释放 1 天的比例）。
	// 这符合 plan 中"不满 1 天按 1 天算"的规则，也让邀请人仪表盘立即看到数字。
	_ = s.releaseOneSubscriptionCommission(ctx, order.ID, referrerID)
	slog.Info("[Referral] accrued subscription commission",
		"referee", order.UserID, "referrer", referrerID,
		"order", order.ID, "amount", order.Amount, "gross", gross,
		"validity_days", *order.SubscriptionDays)
	return nil
}

// --- 返佣账务：充值消费释放 ---

// ReleaseCommissionForRechargeConsumption 在被邀请人余额扣费后异步调用。
//
// 按 FIFO 将 amountConsumed 分配给被邀请人最早尚未充分归因的充值型 commissions，
// 对每单更新 consumed_attributed 并按比例计算本单应释放的 released。
// 差额（应释放 − 已释放）入邀请人 balance。
func (s *ReferralService) ReleaseCommissionForRechargeConsumption(
	ctx context.Context, refereeID int64, amountConsumed float64,
) error {
	if amountConsumed <= 0 {
		return nil
	}
	// 即使 referral_enabled 被关掉，历史产生的 commission 仍继续释放。
	referrerID, err := s.referrerOf(ctx, refereeID)
	if err != nil || referrerID == 0 {
		return err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	// 锁取所有未充分归因的 recharge commission（FIFO）
	list, err := client.ReferralCommission.Query().
		Where(
			referralcommission.RefereeIDEQ(refereeID),
			referralcommission.SourceTypeEQ(ReferralSourceRecharge),
			referralcommission.StatusIn(ReferralStatusAccruing, ReferralStatusPartialReversed),
		).
		Order(dbent.Asc(referralcommission.FieldCreatedAt)).
		ForUpdate().
		All(txCtx)
	if err != nil {
		return fmt.Errorf("query commissions for release: %w", err)
	}

	remaining := amountConsumed
	totalNewReleased := 0.0

	for _, c := range list {
		if remaining <= 0 {
			break
		}
		// 每单可再归因空间 = source_amount − consumed_attributed
		capAmount := c.SourceAmount - c.ConsumedAttributed
		if capAmount <= 0 {
			continue
		}
		alloc := math.Min(remaining, capAmount)
		newAttributed := c.ConsumedAttributed + alloc
		// 释放比例（按 source_amount 占比；reversed 场景 gross 可能减小但比例仍按原 source_amount）
		var ratio float64
		if c.SourceAmount > 0 {
			ratio = newAttributed / c.SourceAmount
		}
		if ratio > 1 {
			ratio = 1
		}
		shouldRelease := roundMoney(c.GrossCommission * ratio)
		if shouldRelease < c.ReleasedCommission {
			shouldRelease = c.ReleasedCommission
		}
		delta := shouldRelease - c.ReleasedCommission

		upd := client.ReferralCommission.UpdateOneID(c.ID).
			SetConsumedAttributed(newAttributed).
			SetReleasedCommission(shouldRelease)

		newStatus := c.Status
		// 归因占满 + 全释放 → fully_released
		if newAttributed+1e-8 >= c.SourceAmount && shouldRelease+1e-8 >= c.GrossCommission {
			newStatus = ReferralStatusFullyReleased
			upd = upd.SetStatus(ReferralStatusFullyReleased)
		}
		if _, err := upd.Save(txCtx); err != nil {
			return fmt.Errorf("update commission: %w", err)
		}

		totalNewReleased += delta
		remaining -= alloc
		_ = newStatus
	}

	if totalNewReleased > 0 {
		if err := s.userRepo.UpdateBalance(txCtx, referrerID, totalNewReleased); err != nil {
			return fmt.Errorf("credit referrer balance: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if totalNewReleased > 0 {
		s.invalidateReferrerCache(referrerID, totalNewReleased)
		slog.Info("[Referral] released recharge commission",
			"referee", refereeID, "referrer", referrerID,
			"released_delta", totalNewReleased, "consumed", amountConsumed)
	}
	return nil
}

// --- 返佣账务：订阅定时释放 ---

// RecalcSubscriptionCommissions 定时任务：重算所有活跃订阅型 commission 的 released。
//
// 释放规则：
//   days_elapsed = ceil(hours / 24)（不满 1 天按 1 天）
//   released_ratio = min(1, days_elapsed / source_validity_days)
//   released = source_amount × rate × ratio （即 gross × ratio）
func (s *ReferralService) RecalcSubscriptionCommissions(ctx context.Context) error {
	list, err := s.entClient.ReferralCommission.Query().
		Where(
			referralcommission.SourceTypeEQ(ReferralSourceSubscription),
			referralcommission.StatusIn(ReferralStatusAccruing, ReferralStatusPartialReversed),
		).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list subscription commissions: %w", err)
	}
	now := time.Now()
	for _, c := range list {
		if c.SourceValidityDays == nil || *c.SourceValidityDays <= 0 || c.SourceStartsAt == nil {
			continue
		}
		// 单行事务（避免一整批失败）
		if err := s.releaseOneSubscriptionCommissionWith(ctx, c, now); err != nil {
			slog.Warn("[Referral] recalc one subscription commission failed",
				"id", c.ID, "error", err)
		}
	}
	return nil
}

// releaseOneSubscriptionCommission 封装：通过 order_id 查一条并释放。
func (s *ReferralService) releaseOneSubscriptionCommission(ctx context.Context, orderID int64, referrerID int64) error {
	c, err := s.entClient.ReferralCommission.Query().
		Where(
			referralcommission.SourceOrderIDEQ(orderID),
			referralcommission.SourceTypeEQ(ReferralSourceSubscription),
		).
		Only(ctx)
	if err != nil {
		return err
	}
	return s.releaseOneSubscriptionCommissionWith(ctx, c, time.Now())
}

// releaseOneSubscriptionCommissionWith 核心释放函数（单行事务 + 行锁）。
func (s *ReferralService) releaseOneSubscriptionCommissionWith(
	ctx context.Context, cSnapshot *dbent.ReferralCommission, now time.Time,
) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	c, err := client.ReferralCommission.Query().
		Where(referralcommission.IDEQ(cSnapshot.ID)).
		ForUpdate().Only(txCtx)
	if err != nil {
		return err
	}
	if c.SourceValidityDays == nil || c.SourceStartsAt == nil {
		return nil
	}
	// referrer 已被硬删（FK SET NULL）时跳过释放：没有可入账的对象。
	if c.ReferrerID == nil {
		return nil
	}
	totalDays := *c.SourceValidityDays
	if totalDays <= 0 {
		return nil
	}
	elapsedHours := now.Sub(*c.SourceStartsAt).Hours()
	if elapsedHours < 0 {
		elapsedHours = 0
	}
	// "不满 1 天按 1 天，超过 n-1 天但不满 n 天按 n 天"
	daysElapsed := int(math.Ceil(elapsedHours / 24))
	if daysElapsed < 1 {
		daysElapsed = 1
	}
	if daysElapsed > totalDays {
		daysElapsed = totalDays
	}
	ratio := float64(daysElapsed) / float64(totalDays)
	shouldRelease := roundMoney(c.GrossCommission * ratio)
	if shouldRelease < c.ReleasedCommission {
		shouldRelease = c.ReleasedCommission
	}
	delta := shouldRelease - c.ReleasedCommission
	if delta <= 0 && daysElapsed < totalDays {
		return nil
	}

	upd := client.ReferralCommission.UpdateOneID(c.ID).
		SetReleasedCommission(shouldRelease)
	if daysElapsed >= totalDays && shouldRelease+1e-8 >= c.GrossCommission {
		upd = upd.SetStatus(ReferralStatusFullyReleased)
	}
	if _, err := upd.Save(txCtx); err != nil {
		return err
	}

	referrerID := *c.ReferrerID
	if delta > 0 {
		if err := s.userRepo.UpdateBalance(txCtx, referrerID, delta); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if delta > 0 {
		s.invalidateReferrerCache(referrerID, delta)
		slog.Info("[Referral] released subscription commission",
			"id", c.ID, "referrer", referrerID, "days", daysElapsed,
			"total_days", totalDays, "delta", delta)
	}
	return nil
}

// --- 退款回收 ---

// ReverseCommissionOnRefund 退款完成后重算 gross。
//
// 已释放部分 (released) 不回收（合理收益），但 gross 必须下调到"实际保留收益"。
//   - 充值型：新 gross = (order.Amount − refund_amount) × rate
//   - 订阅型：新 gross = order.Amount × rate × (保留天数 / validity_days)
//
// 若 released > 新 gross，视为 partial_reversed；否则 reversed / fully_released。
func (s *ReferralService) ReverseCommissionOnRefund(ctx context.Context, order *dbent.PaymentOrder) error {
	if order == nil {
		return nil
	}
	var srcType string
	switch order.OrderType {
	case payment.OrderTypeBalance:
		srcType = ReferralSourceRecharge
	case payment.OrderTypeSubscription:
		srcType = ReferralSourceSubscription
	default:
		return nil
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	c, err := client.ReferralCommission.Query().
		Where(
			referralcommission.SourceOrderIDEQ(order.ID),
			referralcommission.SourceTypeEQ(srcType),
		).
		ForUpdate().
		Only(txCtx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("query commission: %w", err)
	}

	var newGross float64
	switch srcType {
	case ReferralSourceRecharge:
		retained := order.Amount - order.RefundAmount
		if retained < 0 {
			retained = 0
		}
		newGross = roundMoney(retained * c.CommissionRate)
	case ReferralSourceSubscription:
		if c.SourceValidityDays == nil || *c.SourceValidityDays <= 0 || c.SourceStartsAt == nil {
			newGross = c.GrossCommission
		} else {
			// 保留天数：如果订单有 refund_at，则到退款那一刻；否则到现在
			end := time.Now()
			if order.RefundAt != nil {
				end = *order.RefundAt
			}
			hours := end.Sub(*c.SourceStartsAt).Hours()
			if hours < 0 {
				hours = 0
			}
			daysRetained := int(math.Ceil(hours / 24))
			if daysRetained < 1 {
				daysRetained = 1
			}
			if daysRetained > *c.SourceValidityDays {
				daysRetained = *c.SourceValidityDays
			}
			newGross = roundMoney(order.Amount * c.CommissionRate * float64(daysRetained) / float64(*c.SourceValidityDays))
		}
	}

	// 新 gross 不能低于 released（已释放部分不回收）
	effectiveGross := math.Max(newGross, c.ReleasedCommission)
	var newStatus string
	switch {
	case effectiveGross <= 1e-8:
		newStatus = ReferralStatusReversed
	case effectiveGross < c.GrossCommission:
		newStatus = ReferralStatusPartialReversed
	default:
		newStatus = c.Status
	}

	// source_amount 也同步更新为退款后金额（方便仪表盘展示真实值）
	newSourceAmount := order.Amount - order.RefundAmount
	if newSourceAmount < 0 {
		newSourceAmount = 0
	}

	if _, err := client.ReferralCommission.UpdateOneID(c.ID).
		SetGrossCommission(effectiveGross).
		SetSourceAmount(newSourceAmount).
		SetStatus(newStatus).
		Save(txCtx); err != nil {
		return fmt.Errorf("update commission on refund: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	slog.Info("[Referral] reversed commission on refund",
		"id", c.ID, "order", order.ID, "new_gross", effectiveGross, "new_status", newStatus)
	return nil
}

// --- 待发赠金触发 ---

// TryGrantPendingBonus 被邀请人首次付费（充值或订阅）履约成功后调用。
//
// 幂等：只在 pending 状态时发放，已 granted 直接返回 nil。
// 若无 pending 记录（用户不是被邀请来的 / 赠金金额 = 0），直接返回 nil。
func (s *ReferralService) TryGrantPendingBonus(
	ctx context.Context, refereeID int64, trigger string, orderID int64,
) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	b, err := client.ReferralPendingBonus.Query().
		Where(
			referralpendingbonus.RefereeIDEQ(refereeID),
			referralpendingbonus.StatusEQ(ReferralBonusPending),
		).
		ForUpdate().
		Only(txCtx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("query pending bonus: %w", err)
	}
	if b.BonusAmount <= 0 {
		// 0 金额直接标记为已发放
		if _, err := client.ReferralPendingBonus.UpdateOneID(b.ID).
			SetStatus(ReferralBonusGranted).
			SetGrantedAt(time.Now()).
			SetGrantedTrigger(trigger).
			SetGrantedOrderID(orderID).
			Save(txCtx); err != nil {
			return err
		}
		return tx.Commit()
	}

	if err := s.userRepo.UpdateBalance(txCtx, refereeID, b.BonusAmount); err != nil {
		return fmt.Errorf("credit referee bonus: %w", err)
	}
	if _, err := client.ReferralPendingBonus.UpdateOneID(b.ID).
		SetStatus(ReferralBonusGranted).
		SetGrantedAt(time.Now()).
		SetGrantedTrigger(trigger).
		SetGrantedOrderID(orderID).
		Save(txCtx); err != nil {
		return fmt.Errorf("mark bonus granted: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidateReferrerCache(refereeID, b.BonusAmount)
	slog.Info("[Referral] granted referee bonus",
		"referee", refereeID, "bonus", b.BonusAmount, "trigger", trigger, "order", orderID)
	return nil
}

// --- 用户视角查询 ---

// GetMyReferralStats 返回某用户的邀请总览
func (s *ReferralService) GetMyReferralStats(ctx context.Context, userID int64) (*ReferralStats, error) {
	code, err := s.EnsureInviteCode(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 邀请人数
	invitedCount, err := s.entClient.User.Query().
		Where(user.InvitedByUserIDEQ(userID)).
		Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count invited: %w", err)
	}

	// 聚合佣金
	var gross, released sql.NullFloat64
	row := s.sqlDB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(gross_commission), 0), COALESCE(SUM(released_commission), 0)
		FROM referral_commissions WHERE referrer_id = $1`, userID)
	if err := row.Scan(&gross, &released); err != nil {
		return nil, fmt.Errorf("sum commission: %w", err)
	}

	return &ReferralStats{
		InviteCode:         code,
		InvitedCount:       int64(invitedCount),
		GrossCommission:    gross.Float64,
		ReleasedCommission: released.Float64,
		PendingCommission:  math.Max(0, gross.Float64-released.Float64),
		CommissionRate:     s.settingService.GetReferralCommissionRate(ctx),
		RefereeBonusAmount: s.settingService.GetReferralRefereeBonusAmount(ctx),
	}, nil
}

// ListMyCommissionLogs 返回某邀请人的返佣明细（分页）
func (s *ReferralService) ListMyCommissionLogs(
	ctx context.Context, userID int64, page, size int,
) ([]*CommissionLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}

	total, err := s.entClient.ReferralCommission.Query().
		Where(referralcommission.ReferrerIDEQ(userID)).
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.entClient.ReferralCommission.Query().
		Where(referralcommission.ReferrerIDEQ(userID)).
		Order(dbent.Desc(referralcommission.FieldCreatedAt)).
		Offset((page - 1) * size).
		Limit(size).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	// 批量查 referee email（referee 已硬删时 RefereeID 为 nil，跳过查询占位显示）
	refereeIDs := make([]int64, 0, len(rows))
	seen := make(map[int64]struct{})
	for _, r := range rows {
		if r.RefereeID == nil {
			continue
		}
		if _, ok := seen[*r.RefereeID]; !ok {
			refereeIDs = append(refereeIDs, *r.RefereeID)
			seen[*r.RefereeID] = struct{}{}
		}
	}
	emailMap, _ := s.fetchEmailsByIDs(ctx, refereeIDs)

	out := make([]*CommissionLog, 0, len(rows))
	for _, r := range rows {
		var refereeID int64
		var refereeEmail string
		if r.RefereeID != nil {
			refereeID = *r.RefereeID
			refereeEmail = emailMap[*r.RefereeID]
		}
		out = append(out, &CommissionLog{
			ID:                 int64(r.ID),
			RefereeID:          refereeID,
			RefereeEmailMasked: maskEmail(refereeEmail),
			SourceType:         r.SourceType,
			SourceOrderID:      r.SourceOrderID,
			SourceAmount:       r.SourceAmount,
			CommissionRate:     r.CommissionRate,
			GrossCommission:    r.GrossCommission,
			ReleasedCommission: r.ReleasedCommission,
			Status:             r.Status,
			SourceValidityDays: r.SourceValidityDays,
			CreatedAt:          r.CreatedAt,
			UpdatedAt:          r.UpdatedAt,
		})
	}
	return out, int64(total), nil
}

// --- 管理端查询 ---

// GetGlobalReferralStats 全局总览
func (s *ReferralService) GetGlobalReferralStats(ctx context.Context) (*GlobalReferralStats, error) {
	stats := &GlobalReferralStats{
		CommissionRate:     s.settingService.GetReferralCommissionRate(ctx),
		RefereeBonusAmount: s.settingService.GetReferralRefereeBonusAmount(ctx),
		Enabled:            s.settingService.IsReferralEnabled(ctx),
	}

	invitedCount, err := s.entClient.User.Query().
		Where(user.InvitedByUserIDNotNil()).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	stats.TotalInvitedUsers = int64(invitedCount)

	row := s.sqlDB.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(gross_commission), 0),
			COALESCE(SUM(released_commission), 0)
		FROM referral_commissions`)
	if err := row.Scan(&stats.TotalGrossCommission, &stats.TotalReleased); err != nil {
		return nil, fmt.Errorf("sum global commission: %w", err)
	}
	stats.TotalPending = math.Max(0, stats.TotalGrossCommission-stats.TotalReleased)

	grantedCount, _ := s.entClient.ReferralPendingBonus.Query().
		Where(referralpendingbonus.StatusEQ(ReferralBonusGranted)).
		Count(ctx)
	stats.RefereeBonusGranted = int64(grantedCount)
	pendingCount, _ := s.entClient.ReferralPendingBonus.Query().
		Where(referralpendingbonus.StatusEQ(ReferralBonusPending)).
		Count(ctx)
	stats.RefereeBonusPending = int64(pendingCount)

	return stats, nil
}

// ListTopReferrers 按 "总佣金" 或 "邀请人数" 排行
func (s *ReferralService) ListTopReferrers(
	ctx context.Context, sortBy string, limit int,
) ([]*ReferrerRank, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	orderClause := "COALESCE(SUM(rc.gross_commission), 0) DESC"
	if sortBy == "count" {
		orderClause = "invited_count DESC, COALESCE(SUM(rc.gross_commission), 0) DESC"
	}

	q := fmt.Sprintf(`
		SELECT
			u.id,
			u.email,
			u.username,
			COALESCE((SELECT COUNT(*) FROM users u2 WHERE u2.invited_by_user_id = u.id AND u2.deleted_at IS NULL), 0) AS invited_count,
			COALESCE(SUM(rc.gross_commission), 0) AS gross_sum,
			COALESCE(SUM(rc.released_commission), 0) AS released_sum
		FROM users u
		LEFT JOIN referral_commissions rc ON rc.referrer_id = u.id
		WHERE u.deleted_at IS NULL
		GROUP BY u.id, u.email, u.username
		HAVING COALESCE((SELECT COUNT(*) FROM users u2 WHERE u2.invited_by_user_id = u.id AND u2.deleted_at IS NULL), 0) > 0
		ORDER BY %s
		LIMIT $1`, orderClause)

	rows, err := s.sqlDB.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("query top referrers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*ReferrerRank, 0, limit)
	for rows.Next() {
		r := &ReferrerRank{}
		if err := rows.Scan(&r.UserID, &r.Email, &r.Username,
			&r.InvitedCount, &r.GrossCommission, &r.ReleasedCommission); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetReferrerDrilldown 下钻单个邀请人的被邀请人列表（分页）
// page 从 1 起，size 为每页条数；返回本页记录、总数。
func (s *ReferralService) GetReferrerDrilldown(
	ctx context.Context, referrerID int64, page, size int,
) ([]*RefereeDrilldown, int64, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}

	// 先查总数，和列表分两条 SQL。避免为了拿 total 再做窗口函数 count(*) over()，
	// 简单 count 在 users(invited_by_user_id) 索引上代价很低。
	var total int64
	if err := s.sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users
		WHERE invited_by_user_id = $1 AND deleted_at IS NULL`, referrerID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count drilldown: %w", err)
	}
	if total == 0 {
		return []*RefereeDrilldown{}, 0, nil
	}

	offset := (page - 1) * size
	q := `
		SELECT
			u.id,
			u.email,
			u.username,
			u.created_at,
			COALESCE(agg.gross, 0),
			COALESCE(agg.released, 0),
			COALESCE(agg.cnt, 0),
			CASE WHEN rb.status = 'granted' THEN TRUE ELSE FALSE END AS bonus_granted
		FROM users u
		LEFT JOIN (
			SELECT referee_id, SUM(gross_commission) AS gross, SUM(released_commission) AS released, COUNT(*) AS cnt
			FROM referral_commissions WHERE referrer_id = $1
			GROUP BY referee_id
		) agg ON agg.referee_id = u.id
		LEFT JOIN referral_pending_bonuses rb ON rb.referee_id = u.id AND rb.referrer_id = $1
		WHERE u.invited_by_user_id = $1 AND u.deleted_at IS NULL
		ORDER BY u.created_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := s.sqlDB.QueryContext(ctx, q, referrerID, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]*RefereeDrilldown, 0, size)
	for rows.Next() {
		d := &RefereeDrilldown{}
		if err := rows.Scan(&d.RefereeID, &d.Email, &d.Username,
			&d.JoinedAt, &d.Gross, &d.Released, &d.OrderCount, &d.BonusGranted); err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// --- 内部辅助 ---

// referrerOf 返回被邀请人的邀请人 ID；未被邀请返回 0。
func (s *ReferralService) referrerOf(ctx context.Context, refereeID int64) (int64, error) {
	u, err := s.entClient.User.Query().
		Where(user.IDEQ(refereeID)).
		Only(ctx)
	if err != nil {
		return 0, translateUserErr(err)
	}
	if u.InvitedByUserID == nil || *u.InvitedByUserID <= 0 {
		return 0, nil
	}
	return *u.InvitedByUserID, nil
}

// fetchEmailsByIDs 批量查用户 email（用于列表展示脱敏）
func (s *ReferralService) fetchEmailsByIDs(ctx context.Context, ids []int64) (map[int64]string, error) {
	result := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	users, err := s.entClient.User.Query().
		Where(user.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return result, err
	}
	for _, u := range users {
		result[u.ID] = u.Email
	}
	return result, nil
}

// invalidateReferrerCache 当 balance 变动时，失效邀请人的缓存。
func (s *ReferralService) invalidateReferrerCache(userID int64, _ float64) {
	if s.billingCacheService != nil {
		go func() {
			cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateUserBalance(cctx, userID)
		}()
	}
	if s.authCacheInvalidator != nil {
		bg := context.Background()
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(bg, userID)
	}
}

// generateInviteCode 生成 8 位随机邀请码
func generateInviteCode() (string, error) {
	b := make([]byte, inviteCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, inviteCodeLen)
	for i, v := range b {
		out[i] = inviteCodeCharset[int(v)%len(inviteCodeCharset)]
	}
	return string(out), nil
}

// roundMoney 保留 8 位小数（与 DB decimal(20,8) 对齐），避免浮点漂移累积。
func roundMoney(v float64) float64 {
	const scale = 1e8
	return math.Round(v*scale) / scale
}

// maskEmail 脱敏邮箱：abc@example.com -> a**@example.com
func maskEmail(email string) string {
	if email == "" {
		return ""
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 1 {
		return local + "**" + domain
	}
	return string(local[0]) + strings.Repeat("*", min2(3, len(local)-1)) + domain
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isUniqueViolation 检测 unique 约束冲突（ent 包装后的错误）。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if dbent.IsConstraintError(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "UNIQUE constraint")
}

// translateUserErr 将 ent NotFound 翻译为领域错误。
func translateUserErr(err error) error {
	if err == nil {
		return nil
	}
	if dbent.IsNotFound(err) {
		return ErrUserNotFound
	}
	var nfErr *dbent.NotFoundError
	if errors.As(err, &nfErr) {
		return ErrUserNotFound
	}
	return err
}
