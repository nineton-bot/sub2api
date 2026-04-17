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

// ReferralPendingBonus holds the schema definition for the ReferralPendingBonus entity.
//
// 被邀请人延迟赠金：注册时写入 pending 记录，待被邀请人首次完成充值或订阅后
// 才会实际入账（并入 balance）。若从未付费，赠金永远不会发放（防薅羊毛）。
type ReferralPendingBonus struct {
	ent.Schema
}

func (ReferralPendingBonus) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "referral_pending_bonuses"},
	}
}

func (ReferralPendingBonus) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("referee_id").
			Unique(),
		field.Int64("referrer_id"),
		field.Float("bonus_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.String("status").
			MaxLen(20).
			Default("pending").
			Comment("pending | granted"),
		field.Time("granted_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("granted_trigger").
			MaxLen(20).
			Optional().
			Nillable().
			Comment("first_recharge | first_subscription"),
		field.Int64("granted_order_id").
			Optional().
			Nillable(),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ReferralPendingBonus) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("referee_id", "status"),
		index.Fields("referrer_id"),
	}
}
