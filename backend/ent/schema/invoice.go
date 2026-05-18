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
//	pending  -> approved -> issued                （正常审批 + 自动开票或人工上传 PDF）
//	pending  -> rejected                          （管理员驳回，订单释放）
//	pending  -> voided                            （用户取消，订单释放）
//	approved -> voided                            （管理员作废，订单释放）
//	issued   -> voided                            （红冲成功 / 管理员作废，订单释放，PDF 文件保留作历史）
//
// 子状态 provider_state 跟踪第三方平台异步处理：
//
//	开票方向：queued -> issuing -> success | failed
//	红冲方向：reverse_pending -> reversing -> reverse_success | reverse_failed
//
// 数电票红冲走 reverse_step 子状态机：red_applying -> red_confirmed -> red_issuing -> red_done。
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

		// 申请单号：创建时生成，格式 APP-YYYYMMDD-{id6位}，全状态都有，供 UI 在未开具前展示稳定标识
		field.String("application_no").
			MaxLen(32).
			Default("").
			Comment("内部申请单号，创建后回填，与平台 invoice_no 区分"),

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

		// 购方扩展信息（专票必填）。普票/个人可空。
		field.String("buyer_address").
			MaxLen(200).
			Default("").
			Comment("购方地址（专票必填）"),
		field.String("buyer_phone").
			MaxLen(32).
			Default("").
			Comment("购方电话（专票必填）"),
		field.String("buyer_bank_name").
			MaxLen(64).
			Default("").
			Comment("购方开户行（专票必填）"),
		field.String("buyer_bank_account").
			MaxLen(64).
			Default("").
			Comment("购方银行账号（专票必填）"),

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

		// 票种（v3 新增）
		field.String("invoice_kind").
			MaxLen(10).
			Default("normal").
			Comment("normal 普票 | special 专票"),
		field.String("invoice_type_code").
			MaxLen(4).
			Default("").
			Comment("财云通 InvoiceType 枚举：04/10/01/08/05/06"),

		// 第三方扩展
		field.String("provider").
			MaxLen(30).
			Default("manual").
			Comment("manual | caiyuntong | nuonuo | baiwang | ..."),
		field.String("provider_state").
			MaxLen(20).
			Default("none").
			Comment("none | queued | issuing | success | failed | reverse_pending | reversing | reverse_success | reverse_failed"),
		field.String("provider_trace_id").
			MaxLen(128).
			Default("").
			Comment("蓝票 RequestID"),
		field.String("provider_last_error").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.Int("provider_retry_count").
			Default(0),
		field.JSON("provider_payload", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		// 红冲（v3 新增）
		field.String("reverse_step").
			MaxLen(20).
			Default("").
			Comment("red_applying | red_confirmed | red_issuing | red_done"),
		field.String("red_advice_num").
			MaxLen(100).
			Default("").
			Comment("红字信息单编号（数电 Step 1 产物）"),
		field.String("red_confirm_num").
			MaxLen(60).
			Default("").
			Comment("红字信息单 uuid（数电 Step 2 产物）"),
		field.String("reverse_trace_id").
			MaxLen(128).
			Default("").
			Comment("红票 RequestID"),
		field.String("red_invoice_no").
			MaxLen(64).
			Default("").
			Comment("红票号码"),
		field.String("red_pdf_path").
			MaxLen(512).
			Default("").
			Comment("红票 PDF 存储路径"),

		field.Int64("voided_by").
			Optional().
			Nillable(),

		// 对公转账（v3 后续）：无系统订单，用户手填金额/日期，需管理员确认收款后才可审批
		field.String("source").
			MaxLen(20).
			Default("order").
			Comment("order 订单开票 | bank_transfer 对公转账"),
		field.Time("transfer_date").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("对公转账日期（用户填写）"),
		field.Bool("transfer_confirmed").
			Default(false).
			Comment("对公转账是否已确认收款"),
		field.Time("transfer_confirmed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("确认收款时间"),
		field.Int64("transfer_confirmed_by").
			Optional().
			Nillable().
			Comment("确认收款的管理员 user id"),
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
		index.Fields("application_no").
			Unique().
			Annotations(entsql.IndexWhere("application_no <> ''")),
		index.Fields("provider_trace_id").
			Unique().
			Annotations(entsql.IndexWhere("provider_trace_id <> ''")),
		index.Fields("reverse_trace_id").
			Unique().
			Annotations(entsql.IndexWhere("reverse_trace_id <> ''")),
		index.Fields("provider_state"),
	}
}
