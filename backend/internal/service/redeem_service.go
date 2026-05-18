package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrRedeemCodeNotFound      = infraerrors.NotFound("REDEEM_CODE_NOT_FOUND", "redeem code not found")
	ErrRedeemCodeUsed          = infraerrors.Conflict("REDEEM_CODE_USED", "redeem code already used")
	ErrInsufficientBalance     = infraerrors.BadRequest("INSUFFICIENT_BALANCE", "insufficient balance")
	ErrRedeemRateLimited       = infraerrors.TooManyRequests("REDEEM_RATE_LIMITED", "too many failed attempts, please try again later")
	ErrRedeemCodeLocked        = infraerrors.Conflict("REDEEM_CODE_LOCKED", "redeem code is being processed, please try again")
	ErrRedeemRenewRequiresSub  = infraerrors.BadRequest("REDEEM_RENEW_REQUIRES_SUB_ID", "renew intent requires renew_subscription_id")
	ErrRedeemIntentNotApplicable = infraerrors.BadRequest("REDEEM_INTENT_NOT_APPLICABLE", "purchase_intent is only valid for subscription redeem codes")
)

// RedeemPurchaseIntent enumerates how a subscription-type redeem code should be applied
// when the user already owns an active subscription of the same group.
//   - "" / RedeemIntentAuto  : legacy behavior (AssignOrExtend), kept for backward compat with old frontends.
//   - "renew"                : extend an existing subscription's expires_at; quota window NOT reset.
//   - "new"                  : create a brand-new parallel subscription row; quota window starts now.
const (
	RedeemIntentAuto  = ""
	RedeemIntentRenew = "renew"
	RedeemIntentNew   = "new"
)

// RedeemOptions carries per-call hints for Redeem(). Empty struct = legacy behavior.
type RedeemOptions struct {
	PurchaseIntent      string // "" / "renew" / "new"
	RenewSubscriptionID int64  // required when PurchaseIntent == "renew"
}

const (
	redeemMaxErrorsPerHour  = 20
	redeemRateLimitDuration = time.Hour
	redeemLockDuration      = 10 * time.Second // 锁超时时间，防止死锁
)

// RedeemCache defines cache operations for redeem service
type RedeemCache interface {
	GetRedeemAttemptCount(ctx context.Context, userID int64) (int, error)
	IncrementRedeemAttemptCount(ctx context.Context, userID int64) error

	AcquireRedeemLock(ctx context.Context, code string, ttl time.Duration) (bool, error)
	ReleaseRedeemLock(ctx context.Context, code string) error
}

type RedeemCodeRepository interface {
	Create(ctx context.Context, code *RedeemCode) error
	CreateBatch(ctx context.Context, codes []RedeemCode) error
	GetByID(ctx context.Context, id int64) (*RedeemCode, error)
	GetByCode(ctx context.Context, code string) (*RedeemCode, error)
	Update(ctx context.Context, code *RedeemCode) error
	Delete(ctx context.Context, id int64) error
	Use(ctx context.Context, id, userID int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]RedeemCode, *pagination.PaginationResult, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]RedeemCode, error)
	// ListByUserPaginated returns paginated balance/concurrency history for a specific user.
	// codeType filter is optional - pass empty string to return all types.
	ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error)
	// SumPositiveBalanceByUser returns the total recharged amount (sum of positive balance values) for a user.
	SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error)
}

// GenerateCodesRequest 生成兑换码请求
type GenerateCodesRequest struct {
	Count int     `json:"count"`
	Value float64 `json:"value"`
	Type  string  `json:"type"`
}

// RedeemCodeResponse 兑换码响应
type RedeemCodeResponse struct {
	Code      string    `json:"code"`
	Value     float64   `json:"value"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// RedeemService 兑换码服务
type RedeemService struct {
	redeemRepo           RedeemCodeRepository
	userRepo             UserRepository
	subscriptionService  *SubscriptionService
	cache                RedeemCache
	billingCacheService  *BillingCacheService
	entClient            *dbent.Client
	authCacheInvalidator APIKeyAuthCacheInvalidator
}

// NewRedeemService 创建兑换码服务实例
func NewRedeemService(
	redeemRepo RedeemCodeRepository,
	userRepo UserRepository,
	subscriptionService *SubscriptionService,
	cache RedeemCache,
	billingCacheService *BillingCacheService,
	entClient *dbent.Client,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
) *RedeemService {
	return &RedeemService{
		redeemRepo:           redeemRepo,
		userRepo:             userRepo,
		subscriptionService:  subscriptionService,
		cache:                cache,
		billingCacheService:  billingCacheService,
		entClient:            entClient,
		authCacheInvalidator: authCacheInvalidator,
	}
}

// GenerateRandomCode 生成随机兑换码
func (s *RedeemService) GenerateRandomCode() (string, error) {
	// 生成16字节随机数据
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	// 转换为十六进制字符串
	code := hex.EncodeToString(bytes)

	// 格式化为 XXXX-XXXX-XXXX-XXXX 格式
	parts := []string{
		strings.ToUpper(code[0:8]),
		strings.ToUpper(code[8:16]),
		strings.ToUpper(code[16:24]),
		strings.ToUpper(code[24:32]),
	}

	return strings.Join(parts, "-"), nil
}

// GenerateCodes 批量生成兑换码
func (s *RedeemService) GenerateCodes(ctx context.Context, req GenerateCodesRequest) ([]RedeemCode, error) {
	if req.Count <= 0 {
		return nil, errors.New("count must be greater than 0")
	}

	// 邀请码类型不需要数值，其他类型需要非零值（支持负数用于退款）
	if req.Type != RedeemTypeInvitation && req.Value == 0 {
		return nil, errors.New("value must not be zero")
	}

	if req.Count > 1000 {
		return nil, errors.New("cannot generate more than 1000 codes at once")
	}

	codeType := req.Type
	if codeType == "" {
		codeType = RedeemTypeBalance
	}

	// 邀请码类型的 value 设为 0
	value := req.Value
	if codeType == RedeemTypeInvitation {
		value = 0
	}

	codes := make([]RedeemCode, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		code, err := s.GenerateRandomCode()
		if err != nil {
			return nil, fmt.Errorf("generate code: %w", err)
		}

		codes = append(codes, RedeemCode{
			Code:   code,
			Type:   codeType,
			Value:  value,
			Status: StatusUnused,
		})
	}

	// 批量插入
	if err := s.redeemRepo.CreateBatch(ctx, codes); err != nil {
		return nil, fmt.Errorf("create batch codes: %w", err)
	}

	return codes, nil
}

// CreateCode creates a redeem code with caller-provided code value.
// It is primarily used by admin integrations that require an external order ID
// to be mapped to a deterministic redeem code.
func (s *RedeemService) CreateCode(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return errors.New("redeem code is required")
	}
	code.Code = strings.TrimSpace(code.Code)
	if code.Code == "" {
		return errors.New("code is required")
	}
	if code.Type == "" {
		code.Type = RedeemTypeBalance
	}
	if code.Type != RedeemTypeInvitation && code.Value == 0 {
		return errors.New("value must not be zero")
	}
	if code.Status == "" {
		code.Status = StatusUnused
	}

	if err := s.redeemRepo.Create(ctx, code); err != nil {
		return fmt.Errorf("create redeem code: %w", err)
	}
	return nil
}

// checkRedeemRateLimit 检查用户兑换错误次数是否超限
func (s *RedeemService) checkRedeemRateLimit(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
	}

	count, err := s.cache.GetRedeemAttemptCount(ctx, userID)
	if err != nil {
		// Redis 出错时不阻止用户操作
		return nil
	}

	if count >= redeemMaxErrorsPerHour {
		return ErrRedeemRateLimited
	}

	return nil
}

// incrementRedeemErrorCount 增加用户兑换错误计数
func (s *RedeemService) incrementRedeemErrorCount(ctx context.Context, userID int64) {
	if s.cache == nil {
		return
	}

	_ = s.cache.IncrementRedeemAttemptCount(ctx, userID)
}

// acquireRedeemLock 尝试获取兑换码的分布式锁
// 返回 true 表示获取成功，false 表示锁已被占用
func (s *RedeemService) acquireRedeemLock(ctx context.Context, code string) bool {
	if s.cache == nil {
		return true // 无 Redis 时降级为不加锁
	}

	ok, err := s.cache.AcquireRedeemLock(ctx, code, redeemLockDuration)
	if err != nil {
		// Redis 出错时不阻止操作，依赖数据库层面的状态检查
		return true
	}
	return ok
}

// releaseRedeemLock 释放兑换码的分布式锁
func (s *RedeemService) releaseRedeemLock(ctx context.Context, code string) {
	if s.cache == nil {
		return
	}

	_ = s.cache.ReleaseRedeemLock(ctx, code)
}

// Redeem 使用兑换码（向后兼容入口，opt 取零值 = 老行为）
func (s *RedeemService) Redeem(ctx context.Context, userID int64, code string) (*RedeemCode, error) {
	return s.RedeemWithOptions(ctx, userID, code, RedeemOptions{})
}

// RedeemWithOptions 使用兑换码，opt.PurchaseIntent 决定 subscription 类型的处理路径：
//   - "" (RedeemIntentAuto)：调用 AssignOrExtendSubscription（旧行为，向后兼容）
//   - "renew"：对 opt.RenewSubscriptionID 指定的现有订阅纯延长 expires_at
//   - "new"：新建一行独立订阅（叠加），配额窗口从今天起算
//
// validityDays < 0 的"缩减/取消"路径忽略 intent。
// balance / concurrency / invitation 类型传入 PurchaseIntent != "" 会被拒绝（ErrRedeemIntentNotApplicable），
// 防止调用方误用。
func (s *RedeemService) RedeemWithOptions(ctx context.Context, userID int64, code string, opt RedeemOptions) (*RedeemCode, error) {
	// fail-fast：非法 intent 直接拒绝，避免无谓的限流计数 / 锁 / 事务开销。
	switch opt.PurchaseIntent {
	case RedeemIntentAuto, RedeemIntentRenew, RedeemIntentNew:
	default:
		return nil, infraerrors.BadRequest("REDEEM_INTENT_INVALID", "unsupported purchase_intent: "+opt.PurchaseIntent)
	}

	// 检查限流
	if err := s.checkRedeemRateLimit(ctx, userID); err != nil {
		return nil, err
	}

	// 获取分布式锁，防止同一兑换码并发使用
	if !s.acquireRedeemLock(ctx, code) {
		return nil, ErrRedeemCodeLocked
	}
	defer s.releaseRedeemLock(ctx, code)

	// 查找兑换码
	redeemCode, err := s.redeemRepo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrRedeemCodeNotFound) {
			s.incrementRedeemErrorCount(ctx, userID)
			return nil, ErrRedeemCodeNotFound
		}
		return nil, fmt.Errorf("get redeem code: %w", err)
	}

	// 检查兑换码状态
	if !redeemCode.CanUse() {
		s.incrementRedeemErrorCount(ctx, userID)
		return nil, ErrRedeemCodeUsed
	}

	// 验证兑换码类型的前置条件
	if redeemCode.Type == RedeemTypeSubscription && redeemCode.GroupID == nil {
		return nil, infraerrors.BadRequest("REDEEM_CODE_INVALID", "invalid subscription redeem code: missing group_id")
	}

	// intent 仅对 subscription 类型有意义；其他类型若显式带 intent 则拒绝，防止上层误用。
	if opt.PurchaseIntent != RedeemIntentAuto && redeemCode.Type != RedeemTypeSubscription {
		return nil, ErrRedeemIntentNotApplicable
	}

	// 获取用户信息
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	// 使用数据库事务保证兑换码标记与权益发放的原子性
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 将事务放入 context，使 repository 方法能够使用同一事务
	txCtx := dbent.NewTxContext(ctx, tx)

	// 【关键】先标记兑换码为已使用，确保并发安全
	// 利用数据库乐观锁（WHERE status = 'unused'）保证原子性
	if err := s.redeemRepo.Use(txCtx, redeemCode.ID, userID); err != nil {
		if errors.Is(err, ErrRedeemCodeNotFound) || errors.Is(err, ErrRedeemCodeUsed) {
			return nil, ErrRedeemCodeUsed
		}
		return nil, fmt.Errorf("mark code as used: %w", err)
	}

	// 执行兑换逻辑（兑换码已被锁定，此时可安全操作）
	switch redeemCode.Type {
	case RedeemTypeBalance:
		amount := redeemCode.Value
		// 负数为退款扣减，余额最低为 0
		if amount < 0 && user.Balance+amount < 0 {
			amount = -user.Balance
		}
		if err := s.userRepo.UpdateBalance(txCtx, userID, amount); err != nil {
			return nil, fmt.Errorf("update user balance: %w", err)
		}

	case RedeemTypeConcurrency:
		delta := int(redeemCode.Value)
		// 负数为退款扣减，并发数最低为 0
		if delta < 0 && user.Concurrency+delta < 0 {
			delta = -user.Concurrency
		}
		if err := s.userRepo.UpdateConcurrency(txCtx, userID, delta); err != nil {
			return nil, fmt.Errorf("update user concurrency: %w", err)
		}

	case RedeemTypeSubscription:
		validityDays := redeemCode.ValidityDays
		if validityDays < 0 {
			// 负数天数：缩短订阅，减到 0 则取消订阅。忽略 intent（用户拿"扣减码"没法选续期/再买）。
			if err := s.reduceOrCancelSubscription(txCtx, userID, *redeemCode.GroupID, -validityDays, redeemCode.Code); err != nil {
				return nil, fmt.Errorf("reduce or cancel subscription: %w", err)
			}
		} else {
			if validityDays == 0 {
				validityDays = 30
			}
			switch opt.PurchaseIntent {
			case RedeemIntentRenew:
				if opt.RenewSubscriptionID <= 0 {
					return nil, ErrRedeemRenewRequiresSub
				}
				// 复用支付路径的所有权 + 分组归属校验（友好错误码）
				if err := s.subscriptionService.ValidateRenewTarget(txCtx, userID, opt.RenewSubscriptionID, *redeemCode.GroupID); err != nil {
					return nil, err
				}
				if _, err := s.subscriptionService.RenewSubscription(
					txCtx, userID, opt.RenewSubscriptionID, *redeemCode.GroupID, validityDays,
				); err != nil {
					return nil, fmt.Errorf("renew subscription: %w", err)
				}
			case RedeemIntentNew:
				if _, err := s.subscriptionService.CreateNewSubscription(txCtx, &AssignSubscriptionInput{
					UserID:       userID,
					GroupID:      *redeemCode.GroupID,
					ValidityDays: validityDays,
					AssignedBy:   0,
					Notes:        fmt.Sprintf("通过兑换码 %s 兑换（再买一张）", redeemCode.Code),
				}); err != nil {
					// 触底叠加上限时 CreateNewSubscription 返回 ErrTooManyStackedSubscriptions（已是友好错误码）
					return nil, fmt.Errorf("create new subscription: %w", err)
				}
			default: // RedeemIntentAuto（"" 缺省）：fail-fast 已经确保 intent 必属 {auto, renew, new}
				if _, _, err := s.subscriptionService.AssignOrExtendSubscription(txCtx, &AssignSubscriptionInput{
					UserID:       userID,
					GroupID:      *redeemCode.GroupID,
					ValidityDays: validityDays,
					AssignedBy:   0,
					Notes:        fmt.Sprintf("通过兑换码 %s 兑换", redeemCode.Code),
				}); err != nil {
					return nil, fmt.Errorf("assign or extend subscription: %w", err)
				}
			}
		}

	default:
		return nil, fmt.Errorf("unsupported redeem type: %s", redeemCode.Type)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// 事务提交成功后失效缓存
	s.invalidateRedeemCaches(ctx, userID, redeemCode)

	// 重新获取更新后的兑换码
	redeemCode, err = s.redeemRepo.GetByID(ctx, redeemCode.ID)
	if err != nil {
		return nil, fmt.Errorf("get updated redeem code: %w", err)
	}

	return redeemCode, nil
}

// invalidateRedeemCaches 失效兑换相关的缓存
func (s *RedeemService) invalidateRedeemCaches(ctx context.Context, userID int64, redeemCode *RedeemCode) {
	switch redeemCode.Type {
	case RedeemTypeBalance:
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		if s.billingCacheService == nil {
			return
		}
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
		}()
	case RedeemTypeConcurrency:
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		if s.billingCacheService == nil {
			return
		}
	case RedeemTypeSubscription:
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		if s.billingCacheService == nil {
			return
		}
		if redeemCode.GroupID != nil {
			groupID := *redeemCode.GroupID
			go func() {
				cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID)
			}()
		}
	}
}

// RedeemPreviewSubInfo 用于 PreviewRedeem 返回"用户已持有的同 group 活跃订阅"快照。
type RedeemPreviewSubInfo struct {
	ID        int64     `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RedeemPreview 是 PreviewRedeem 的返回结构。前端拿到后判断是否需要弹"续期 / 再买一张"二选一弹窗。
//
// 不弹窗的场景（前端直接走 Redeem 即可）：
//   - Type != "subscription"
//   - IsReduce == true（扣减/取消，validityDays<0）
//   - len(ExistingActive) == 0（用户该 group 当前无未过期订阅）
type RedeemPreview struct {
	Type           string                 `json:"type"`
	Value          float64                `json:"value"`
	GroupID        *int64                 `json:"group_id,omitempty"`
	GroupName      string                 `json:"group_name,omitempty"`
	ValidityDays   int                    `json:"validity_days,omitempty"`
	ExistingActive []RedeemPreviewSubInfo `json:"existing_active_subs,omitempty"`
	StackCap       int                    `json:"stack_cap,omitempty"`
	StackCount     int                    `json:"stack_count,omitempty"`
	IsReduce       bool                   `json:"is_reduce,omitempty"`
}

// PreviewRedeem 只读校验兑换码：不消耗码、不下发权益、不持有 redeem 分布式锁，
// 同时共享 incrementRedeemErrorCount 防止暴力枚举 group 信息。
//
// 给前端兑换页用：在用户点"兑换"时先预览，若该 group 已持有未过期订阅就弹窗让用户选
// "续期" vs "再买一张"，与"我的订阅"页右上角续费按钮的 UX 一致（参考 PR 81ee8105）。
func (s *RedeemService) PreviewRedeem(ctx context.Context, userID int64, code string) (*RedeemPreview, error) {
	if err := s.checkRedeemRateLimit(ctx, userID); err != nil {
		return nil, err
	}

	rc, err := s.redeemRepo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrRedeemCodeNotFound) {
			s.incrementRedeemErrorCount(ctx, userID)
			return nil, ErrRedeemCodeNotFound
		}
		return nil, fmt.Errorf("get redeem code: %w", err)
	}
	if !rc.CanUse() {
		s.incrementRedeemErrorCount(ctx, userID)
		return nil, ErrRedeemCodeUsed
	}

	out := &RedeemPreview{
		Type:  rc.Type,
		Value: rc.Value,
	}

	if rc.Type != RedeemTypeSubscription {
		return out, nil
	}

	if rc.GroupID == nil {
		return nil, infraerrors.BadRequest("REDEEM_CODE_INVALID", "invalid subscription redeem code: missing group_id")
	}

	out.GroupID = rc.GroupID
	out.ValidityDays = rc.ValidityDays
	out.IsReduce = rc.ValidityDays < 0
	out.StackCap = MaxStackedSubscriptionsPerUserGroup

	// 兑换码绑定的 group 应当始终存在；若取不到（被硬删 / 软删），是数据完整性问题，
	// 直接报错而不是退化为空 group 名 —— 让用户在弹窗里看到 '您当前已拥有套餐""' 体感很差，
	// 也容易让前端的"是否需要弹窗"判断逻辑跑偏。
	group, gerr := s.subscriptionService.GetGroupByID(ctx, *rc.GroupID)
	if gerr != nil || group == nil {
		return nil, infraerrors.NotFound("REDEEM_CODE_GROUP_MISSING", "subscription group referenced by this redeem code no longer exists")
	}
	out.GroupName = group.Name

	// 扣减码不需要弹窗，跳过活跃订阅列表（避免无谓查询）
	if out.IsReduce {
		return out, nil
	}

	list, lerr := s.subscriptionService.ListActiveSubscriptionsForUserGroup(ctx, userID, *rc.GroupID)
	if lerr == nil {
		out.StackCount = len(list)
		out.ExistingActive = make([]RedeemPreviewSubInfo, 0, len(list))
		for i := range list {
			out.ExistingActive = append(out.ExistingActive, RedeemPreviewSubInfo{
				ID:        list[i].ID,
				ExpiresAt: list[i].ExpiresAt,
			})
		}
	}

	return out, nil
}

// GetByID 根据ID获取兑换码
func (s *RedeemService) GetByID(ctx context.Context, id int64) (*RedeemCode, error) {
	code, err := s.redeemRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get redeem code: %w", err)
	}
	return code, nil
}

// GetByCode 根据Code获取兑换码
func (s *RedeemService) GetByCode(ctx context.Context, code string) (*RedeemCode, error) {
	redeemCode, err := s.redeemRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("get redeem code: %w", err)
	}
	return redeemCode, nil
}

// List 获取兑换码列表（管理员功能）
func (s *RedeemService) List(ctx context.Context, params pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	codes, pagination, err := s.redeemRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list redeem codes: %w", err)
	}
	return codes, pagination, nil
}

// Delete 删除兑换码（管理员功能）
func (s *RedeemService) Delete(ctx context.Context, id int64) error {
	// 检查兑换码是否存在
	code, err := s.redeemRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get redeem code: %w", err)
	}

	// 不允许删除已使用的兑换码
	if code.IsUsed() {
		return infraerrors.Conflict("REDEEM_CODE_DELETE_USED", "cannot delete used redeem code")
	}

	if err := s.redeemRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete redeem code: %w", err)
	}

	return nil
}

// GetStats 获取兑换码统计信息
func (s *RedeemService) GetStats(ctx context.Context) (map[string]any, error) {
	// TODO: 实现统计逻辑
	// 统计未使用、已使用的兑换码数量
	// 统计总面值等

	stats := map[string]any{
		"total_codes":  0,
		"unused_codes": 0,
		"used_codes":   0,
		"total_value":  0.0,
	}

	return stats, nil
}

// GetUserHistory 获取用户的兑换历史
func (s *RedeemService) GetUserHistory(ctx context.Context, userID int64, limit int) ([]RedeemCode, error) {
	codes, err := s.redeemRepo.ListByUser(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get user redeem history: %w", err)
	}
	return codes, nil
}

// reduceOrCancelSubscription 缩短订阅天数，剩余天数 <= 0 时取消订阅
func (s *RedeemService) reduceOrCancelSubscription(ctx context.Context, userID, groupID int64, reduceDays int, code string) error {
	sub, err := s.subscriptionService.userSubRepo.GetByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return ErrSubscriptionNotFound
	}

	now := time.Now()
	remaining := int(sub.ExpiresAt.Sub(now).Hours() / 24)
	if remaining < 0 {
		remaining = 0
	}

	notes := fmt.Sprintf("通过兑换码 %s 退款扣减 %d 天", code, reduceDays)

	if remaining <= reduceDays {
		// 剩余天数不足，直接取消订阅
		if err := s.subscriptionService.userSubRepo.UpdateStatus(ctx, sub.ID, SubscriptionStatusExpired); err != nil {
			return fmt.Errorf("cancel subscription: %w", err)
		}
		// 设置过期时间为当前时间
		if err := s.subscriptionService.userSubRepo.ExtendExpiry(ctx, sub.ID, now); err != nil {
			return fmt.Errorf("set subscription expiry: %w", err)
		}
	} else {
		// 缩短天数
		newExpiresAt := sub.ExpiresAt.AddDate(0, 0, -reduceDays)
		if err := s.subscriptionService.userSubRepo.ExtendExpiry(ctx, sub.ID, newExpiresAt); err != nil {
			return fmt.Errorf("reduce subscription: %w", err)
		}
	}

	// 追加备注
	newNotes := sub.Notes
	if newNotes != "" {
		newNotes += "\n"
	}
	newNotes += notes
	if err := s.subscriptionService.userSubRepo.UpdateNotes(ctx, sub.ID, newNotes); err != nil {
		return fmt.Errorf("update subscription notes: %w", err)
	}

	// 失效缓存
	s.subscriptionService.InvalidateSubCache(userID, groupID)

	return nil
}
