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

// UserReferralConfig 单用户返佣配置覆盖（V2）。
//
// 语义：每行对应一个用户；未写入记录 = 全部跟随全局默认。
// enabled / commission_rate_override / referee_bonus_override 为 nil 时
// 表示该项跟随全局；非 nil 时覆盖全局。
// withdrawal_allowed 是纯 per-user 开关，默认 false（谨慎功能）。
type UserReferralConfig struct {
	ent.Schema
}

func (UserReferralConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_referral_configs"},
	}
}

func (UserReferralConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (UserReferralConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").Unique(),
		field.Bool("enabled").
			Optional().
			Nillable().
			Comment("nil 表示跟随全局默认"),
		field.Float("commission_rate_override").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,6)"}).
			Optional().
			Nillable(),
		field.Float("referee_bonus_override").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Optional().
			Nillable(),
		field.Bool("withdrawal_allowed").
			Default(false),
		field.String("notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
	}
}

func (UserReferralConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
