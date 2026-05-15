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

// RefundRequest 用户对已开票订单发起的退款请求。
//
// 当订单已开发票时，RequestRefund 不再直接 409，而是创建一条 RefundRequest
// 并触发 invoice 的红冲流程；红冲成功后由系统驱动执行实际的资金退款。
//
// 状态机：
//
//	awaiting_reverse  -> reversing -> refunding -> done
//	awaiting_reverse  -> blocked              （红冲失败，等管理员介入）
//	awaiting_reverse  -> rejected             （管理员拒绝退款）
type RefundRequest struct {
	ent.Schema
}

func (RefundRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "refund_requests"},
	}
}

func (RefundRequest) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (RefundRequest) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("payment_order_id").
			MaxLen(64).
			Comment("关联订单 out_trade_no"),
		field.Int64("invoice_id").
			Optional().
			Nillable().
			Comment("关联发票（已开票时存在）"),

		field.String("status").
			MaxLen(20).
			Default("awaiting_reverse").
			Comment("awaiting_reverse | reversing | refunding | done | blocked | rejected"),

		field.String("reason").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default("").
			Comment("用户填写退款原因"),

		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Comment("退款金额，RMB 元"),

		field.Int64("admin_id").
			Optional().
			Nillable().
			Comment("失败时处理的管理员"),

		field.String("admin_notes").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),

		field.String("last_error").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
	}
}

func (RefundRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("payment_order_id"),
		index.Fields("invoice_id"),
		index.Fields("status", "created_at"),
	}
}
