package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// InvoiceItem 发票与订单的关联行。
//
// 唯一约束：每个 payment_order_id 在该表中至多出现一次。
// 当发票进入 rejected/voided 时，service 在事务内 hard-delete 对应 invoice_items 行，
// 释放该订单的占用，使其可被新的发票申请绑定。
type InvoiceItem struct {
	ent.Schema
}

func (InvoiceItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "invoice_items"},
	}
}

func (InvoiceItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("invoice_id"),
		field.Int64("payment_order_id"),

		// 订单快照
		field.String("order_no").
			MaxLen(64).
			Comment("冗余 payment_orders.out_trade_no"),
		field.String("product_name").
			MaxLen(200).
			Default("").
			Comment("订单产品名快照，例如 1000刀API余额充值"),
		field.String("order_type").
			MaxLen(20).
			Comment("balance | subscription"),
		field.Float("pay_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Comment("订单实付金额（RMB 元），快照"),
		field.Time("paid_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (InvoiceItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("invoice", Invoice.Type).
			Ref("items").
			Field("invoice_id").
			Unique().
			Required(),
	}
}

func (InvoiceItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invoice_id"),
		// 防重复：同一订单只能存在于一张活跃发票里。
		// service 在 reject/void 时 hard-delete 对应行以释放约束。
		index.Fields("payment_order_id").Unique(),
	}
}
