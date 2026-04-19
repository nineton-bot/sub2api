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

// ReferralCommissionReleaseLog 释放日志（V2），append-only。
//
// 每次佣金释放事件写一行：
//   - 充值消费触发（recharge_consumed）
//   - 订阅定时触发（subscription_daily）
//   - 管理员手动触发（manual_admin）
//   - 退款反转（refund_reversal，amount 可为负）
//
// 用户可在"我的推广"页点击 commission 行展开看到完整释放时间线。
// computation_detail 存 JSONB，不同 trigger_type 存不同结构（见 service 层）。
type ReferralCommissionReleaseLog struct {
	ent.Schema
}

func (ReferralCommissionReleaseLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "referral_commission_release_logs"},
	}
}

func (ReferralCommissionReleaseLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("commission_id"),
		field.Int64("user_id"),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.String("trigger_type").
			MaxLen(30).
			Comment("recharge_consumed | subscription_daily | manual_admin | refund_reversal"),
		field.Float("rate_snapshot").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,6)"}),
		field.String("computation_detail").
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Default("{}"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ReferralCommissionReleaseLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("commission_id"),
		index.Fields("user_id", "created_at"),
	}
}
