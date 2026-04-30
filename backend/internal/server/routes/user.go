package routes

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	rateMW "github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterUserRoutes 注册用户相关路由（需要认证）
func RegisterUserRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	jwtAuth middleware.JWTAuthMiddleware,
	settingService *service.SettingService,
	redisClient *redis.Client,
) {
	// 用户侧敏感接口用的速率限制器（每用户 30/分钟），保护邀请返佣等偶发型查询接口
	// 不被刷。Redis 故障时 fail-open（用户体验优先）。
	rateLimiter := rateMW.NewRateLimiter(redisClient)

	// userIDKey 从 JWT auth 结果中提取 user ID 做限流细分。未登录/异常取不到时回落
	// 到 ClientIP（rate_limiter.go 内部会处理）。
	userIDKey := func(c *gin.Context) string {
		if sub, ok := middleware.GetAuthSubjectFromContext(c); ok && sub.UserID > 0 {
			return "u:" + strconv.FormatInt(sub.UserID, 10)
		}
		return ""
	}

	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	{
		// 用户接口
		user := authenticated.Group("/user")
		{
			user.GET("/profile", h.User.GetProfile)
			user.PUT("/password", h.User.ChangePassword)
			user.PUT("", h.User.UpdateProfile)
			user.GET("/aff", h.User.GetAffiliate)
			user.POST("/aff/transfer", h.User.TransferAffiliateQuota)
			user.POST("/account-bindings/email/send-code", h.User.SendEmailBindingCode)
			user.POST("/account-bindings/email", h.User.BindEmailIdentity)
			user.DELETE("/account-bindings/:provider", h.User.UnbindIdentity)
			user.POST("/auth-identities/bind/start", h.User.StartIdentityBinding)

			// 通知邮箱管理
			notifyEmail := user.Group("/notify-email")
			{
				notifyEmail.POST("/send-code", h.User.SendNotifyEmailCode)
				notifyEmail.POST("/verify", h.User.VerifyNotifyEmail)
				notifyEmail.PUT("/toggle", h.User.ToggleNotifyEmail)
				notifyEmail.DELETE("", h.User.RemoveNotifyEmail)
			}

			// TOTP 双因素认证
			totp := user.Group("/totp")
			{
				totp.GET("/status", h.Totp.GetStatus)
				totp.GET("/verification-method", h.Totp.GetVerificationMethod)
				totp.POST("/send-code", h.Totp.SendVerifyCode)
				totp.POST("/setup", h.Totp.InitiateSetup)
				totp.POST("/enable", h.Totp.Enable)
				totp.POST("/disable", h.Totp.Disable)
			}
		}

		// API Key管理
		keys := authenticated.Group("/keys")
		{
			keys.GET("", h.APIKey.List)
			keys.GET("/:id", h.APIKey.GetByID)
			keys.POST("", h.APIKey.Create)
			keys.PUT("/:id", h.APIKey.Update)
			keys.DELETE("/:id", h.APIKey.Delete)
		}

		// 用户可用分组（非管理员接口）
		groups := authenticated.Group("/groups")
		{
			groups.GET("/available", h.APIKey.GetAvailableGroups)
			groups.GET("/rates", h.APIKey.GetUserGroupRates)
		}

		// 用户可用渠道（非管理员接口）
		channels := authenticated.Group("/channels")
		{
			channels.GET("/available", h.AvailableChannel.List)
		}

		// 使用记录
		usage := authenticated.Group("/usage")
		{
			usage.GET("", h.Usage.List)
			usage.GET("/:id", h.Usage.GetByID)
			usage.GET("/stats", h.Usage.Stats)
			// User dashboard endpoints
			usage.GET("/dashboard/stats", h.Usage.DashboardStats)
			usage.GET("/dashboard/trend", h.Usage.DashboardTrend)
			usage.GET("/dashboard/models", h.Usage.DashboardModels)
			usage.POST("/dashboard/api-keys-usage", h.Usage.DashboardAPIKeysUsage)
		}

		// 公告（用户可见）
		announcements := authenticated.Group("/announcements")
		{
			announcements.GET("", h.Announcement.List)
			announcements.POST("/:id/read", h.Announcement.MarkRead)
		}

		// 卡密兑换
		redeem := authenticated.Group("/redeem")
		{
			redeem.POST("", h.Redeem.Redeem)
			redeem.GET("/history", h.Redeem.GetHistory)
		}

		// 用户订阅
		subscriptions := authenticated.Group("/subscriptions")
		{
			subscriptions.GET("", h.Subscription.List)
			subscriptions.GET("/active", h.Subscription.GetActive)
			subscriptions.GET("/progress", h.Subscription.GetProgress)
			subscriptions.GET("/summary", h.Subscription.GetSummary)
		}

		// 邀请返佣（用户视角）
		// 这组接口虽然是查询型但涉及 DB 连表 + 脱敏邮箱拉取，容易被脚本刷爆；
		// 每用户每分钟 30 次足够正常前端页面使用（刷新+翻页）。
		referral := authenticated.Group("/user/referral")
		referral.Use(rateLimiter.LimitWithKeyFn(
			"user-referral", 30, time.Minute,
			rateMW.RateLimitOptions{FailureMode: rateMW.RateLimitFailOpen},
			userIDKey,
		))
		{
			referral.GET("/eligibility", h.Referral.GetEligibility)
			referral.GET("/overview", h.Referral.GetMyOverview)
			referral.GET("/commissions", h.Referral.ListMyCommissions)
			referral.GET("/release-logs", h.Referral.ListMyReleaseLogsDaily)
			referral.POST("/ensure-code", h.Referral.EnsureInviteCode)

			// 资金动作额外加严限流（10 次/分钟）——防刷扣减
			moneyActions := referral.Group("")
			moneyActions.Use(rateLimiter.LimitWithKeyFn(
				"user-referral-money", 10, time.Minute,
				rateMW.RateLimitOptions{FailureMode: rateMW.RateLimitFailOpen},
				userIDKey,
			))
			{
				moneyActions.POST("/transfer-to-balance", h.Referral.TransferToBalance)
			}
		}

		// 渠道监控（用户只读）
		monitors := authenticated.Group("/channel-monitors")
		{
			monitors.GET("", h.ChannelMonitor.List)
			monitors.GET("/:id/status", h.ChannelMonitor.GetStatus)
		}
	}
}
