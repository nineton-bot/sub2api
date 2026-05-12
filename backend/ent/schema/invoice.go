package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Invoice 用户开票申请。
//
// 生命周期：
//
//	pending  -> approved -> issued                （正常审批 + 上传 PDF）
//	pending  -> rejected                          （管理员驳回，订单释放）
//	pending  -> voided                            （用户取消，订单释放）
//	approved -> voided                            （管理员作废，订单释放）
//	issued   -> voided                            （管理员作废，订单释放，PDF 文件保留作历史）
//
// rejected/voided 时事务内 hard-delete invoice_items 释放订单的 UNIQUE 约束。
type Invoice struct {
	ent.Schema
}

func (Invoice) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "invoices"},
	}
}

func (Invoice) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Invoice) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("user_email").
			MaxLen(255).
			Comment("申请时邮箱快照"),

		// 抬头
		field.String("title_type").
			MaxLen(20).
			Comment("personal | business"),
		field.String("title").
			MaxLen(200),
		field.String("tax_no").
			MaxLen(64).
			Default("").
			Comment("企业必填，个人空串"),
		field.String("contact_email").
			MaxLen(255).
			Default("").
			Comment("收票邮箱（可选）"),

		// 金额
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Comment("订单 pay_amount 之和，RMB 元"),
		field.String("currency").
			MaxLen(8).
			Default("CNY"),

		// 用户备注
		field.String("notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),

		// 状态机
		field.String("status").
			MaxLen(20).
			Default("pending").
			Comment("pending | approved | issued | rejected | voided"),
		field.Time("submitted_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		// 审核
		field.Time("reviewed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("reviewed_by").
			Optional().
			Nillable().
			Comment("admin user id"),
		field.String("review_notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default("").
			Comment("驳回/作废原因"),

		// 开具
		field.Time("issued_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("invoice_no").
			MaxLen(64).
			Default("").
			Comment("真实发票号（管理员填写）"),

		// PDF 文件
		field.String("pdf_path").
			MaxLen(512).
			Default("").
			Comment("存储路径或 S3 key"),
		field.String("pdf_storage").
			MaxLen(20).
			Default("local").
			Comment("local | s3"),
		field.Int64("pdf_size").
			Default(0),
		field.String("pdf_sha256").
			MaxLen(64).
			Default(""),
		field.String("pdf_original_name").
			MaxLen(255).
			Default(""),

		// 第三方扩展
		field.String("provider").
			MaxLen(30).
			Default("manual").
			Comment("manual | nuonuo | baiwang | ..."),
		field.JSON("provider_payload", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		field.Int64("voided_by").
			Optional().
			Nillable(),
	}
}

func (Invoice) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("invoices").
			Field("user_id").
			Unique().
			Required(),
		edge.To("items", InvoiceItem.Type),
	}
}

func (Invoice) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("status", "submitted_at"),
		index.Fields("invoice_no").
			Unique().
			Annotations(entsql.IndexWhere("invoice_no <> ''")),
	}
}
