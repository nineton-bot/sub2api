package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "users"},
	}
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		// 唯一约束通过部分索引实现（WHERE deleted_at IS NULL），支持软删除后重用
		// 见迁移文件 016_soft_delete_partial_unique_indexes.sql
		field.String("email").
			MaxLen(255).
			NotEmpty(),
		field.String("password_hash").
			MaxLen(255).
			NotEmpty(),
		field.String("role").
			MaxLen(20).
			Default(domain.RoleUser),
		field.Float("balance").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Int("concurrency").
			Default(5),
		field.String("status").
			MaxLen(20).
			Default(domain.StatusActive),

		// Optional profile fields (added later; default '' in DB migration)
		field.String("username").
			MaxLen(100).
			Default(""),
		// wechat field migrated to user_attribute_values (see migration 019)
		field.String("notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),

		// TOTP 双因素认证字段
		field.String("totp_secret_encrypted").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Optional().
			Nillable(),
		field.Bool("totp_enabled").
			Default(false),
		field.Time("totp_enabled_at").
			Optional().
			Nillable(),

		// 邀请返佣字段（见迁移 103_add_referral_system.sql）
		// invited_by_user_id 是邀请关系，终身绑定。部分用户未必被邀请，故 Optional+Nillable。
		field.Int64("invited_by_user_id").
			Optional().
			Nillable(),
		// invite_code 每个账户唯一（按需生成），用于生成邀请链接。
		field.String("invite_code").
			MaxLen(16).
			Optional().
			Nillable(),

		// 可使用佣金池（V2），与 balance 解耦。
		// 释放事件写入此字段；用户可主动"转入余额"或"申请提现"。
		// 见迁移 105_referral_usable_and_withdrawals.sql。
		field.Float("referral_usable").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("api_keys", APIKey.Type),
		edge.To("redeem_codes", RedeemCode.Type),
		edge.To("subscriptions", UserSubscription.Type),
		edge.To("assigned_subscriptions", UserSubscription.Type),
		edge.To("announcement_reads", AnnouncementRead.Type),
		edge.To("allowed_groups", Group.Type).
			Through("user_allowed_groups", UserAllowedGroup.Type),
		edge.To("usage_logs", UsageLog.Type),
		edge.To("attribute_values", UserAttributeValue.Type),
		edge.To("promo_code_usages", PromoCodeUsage.Type),
		edge.To("payment_orders", PaymentOrder.Type),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		// email 字段已在 Fields() 中声明 Unique()，无需重复索引
		index.Fields("status"),
		index.Fields("deleted_at"),
		// 邀请返佣索引（唯一约束通过 WHERE invite_code IS NOT NULL 的部分索引在迁移中实现）
		index.Fields("invited_by_user_id"),
	}
}
