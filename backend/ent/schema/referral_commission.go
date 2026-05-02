package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ReferralCommission holds the schema definition for the ReferralCommission entity.
//
// 邀请返佣台账：每笔被邀请人充值/订阅订单对应一条记录。
//
// 释放模型：
//   - 充值型：按 FIFO 与被邀请人实际消费挂钩释放，释放金额并入邀请人 balance。
//   - 订阅型：按订阅生效天数线性释放（不满 1 天按 1 天算）。
//
// 退款处理：订单状态变为 REFUNDED/PARTIALLY_REFUNDED 时重算 gross_commission，
// 已释放部分不回收（合理收益）。
type ReferralCommission struct {
	ent.Schema
}

func (ReferralCommission) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "referral_commissions"},
	}
}

func (ReferralCommission) Fields() []ent.Field {
	return []ent.Field{
		// referrer_id / referee_id 在 users 被硬删除时通过 FK ON DELETE SET NULL
		// 置空（见迁移 135_referral_commissions_set_null_fk.sql，原 104），以保留佣金台账
		// 作为审计证据。业务代码写入时仍传非空值。
		field.Int64("referrer_id").
			Optional().
			Nillable(),
		field.Int64("referee_id").
			Optional().
			Nillable(),
		field.String("source_type").
			MaxLen(20).
			Comment("recharge | subscription"),
		field.Int64("source_order_id"),
		field.Float("source_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Int64("source_subscription_id").
			Optional().
			Nillable(),
		field.Int("source_validity_days").
			Optional().
			Nillable(),
		field.Time("source_starts_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Float("commission_rate").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,6)"}),
		field.Float("gross_commission").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("released_commission").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("consumed_attributed").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.String("status").
			MaxLen(20).
			Default("accruing").
			Comment("accruing | fully_released | reversed | partial_reversed"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ReferralCommission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("referrer_id", "status"),
		index.Fields("referee_id"),
		index.Fields("source_type", "status"),
		// 一单一条（防重复入账）
		index.Fields("source_order_id", "source_type").Unique(),
	}
}
