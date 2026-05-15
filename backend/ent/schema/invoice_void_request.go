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

// InvoiceVoidRequest 用户对已开发票发起的作废申请。
//
// 与 RefundRequest 的差异：
//   - 不涉及资金（用户只想原票作废，例如抬头错了想重开）
//   - approve 通过后直接调 AdminVoid，复用现有红冲流水线
//   - void_request 自身只跟踪「工单」状态机，红冲进度看 invoice.provider_state
//
// 状态机：
//
//	pending_review -> approved     （admin 同意 + 同事务调 AdminVoid 进入红冲）
//	pending_review -> rejected     （admin 拒绝，无副作用，用户可再次提交）
type InvoiceVoidRequest struct {
	ent.Schema
}

func (InvoiceVoidRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "invoice_void_requests"},
	}
}

func (InvoiceVoidRequest) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (InvoiceVoidRequest) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Comment("申请人"),
		field.Int64("invoice_id").
			Comment("关联发票（必有）"),
		field.String("status").
			MaxLen(20).
			Default("pending_review").
			Comment("pending_review | approved | rejected"),
		field.String("reason").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default("").
			Comment("用户填写的作废原因"),
		field.Int64("admin_id").
			Optional().
			Nillable().
			Comment("审批管理员"),
		field.String("admin_notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default("").
			Comment("管理员备注（驳回理由）"),
		field.Time("reviewed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("审批时间"),
	}
}

func (InvoiceVoidRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("invoice_id", "status"),
		index.Fields("status", "created_at"),
	}
}
