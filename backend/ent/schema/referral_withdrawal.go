package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ReferralWithdrawal 提现申请（V2）。
//
// 生命周期：
//
//	pending   -> approved -> completed   （正常审批流）
//	pending   -> rejected                （拒绝，退回 referral_usable）
//	pending   -> cancelled               （用户主动取消，退回 referral_usable）
//
// 金额在 pending 时已从 users.referral_usable 扣减，rejected/cancelled 时退回。
// approved/completed 不再变动 referral_usable（线下打款）。
type ReferralWithdrawal struct {
	ent.Schema
}

func (ReferralWithdrawal) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "referral_withdrawals"},
	}
}

func (ReferralWithdrawal) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ReferralWithdrawal) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.String("payout_method").
			MaxLen(20).
			Comment("wechat | alipay | bank | other"),
		field.String("payout_account").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("status").
			MaxLen(20).
			Default("pending").
			Comment("pending | approved | rejected | completed | cancelled"),
		field.Time("requested_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("reviewed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("completed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("reviewed_by").
			Optional().
			Nillable(),
		field.String("review_notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
	}
}

func (ReferralWithdrawal) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("status", "requested_at"),
	}
}
