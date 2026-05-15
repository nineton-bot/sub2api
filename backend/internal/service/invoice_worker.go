package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/invoice"
	"github.com/Wei-Shaw/sub2api/ent/invoiceitem"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/service/provider/caiyuntong"
)

// StartInvoiceWorkers 启动后台发票 worker。重复调用 no-op。
//
// 当前实现两个独立 goroutine：
//   - dispatchWorker：扫 invoices.provider_state IN (queued, reverse_pending)，调用 provider.Issue() 或推进红冲第一步
//   - pollWorker：扫 invoices.provider_state IN (issuing, reversing)，调 Query 拉取最终结果或推进多步红冲
//
// 间隔由 SettingService 控制（invoice_poller_interval_seconds / invoice_reverse_poller_interval_seconds）。
func (s *InvoiceService) StartInvoiceWorkers() {
	if s == nil || s.entClient == nil || s.settingService == nil {
		return
	}
	if s.workerStarted {
		return
	}
	s.workerStarted = true

	s.workerWG.Add(2)
	go func() {
		defer s.workerWG.Done()
		s.runDispatchWorker()
	}()
	go func() {
		defer s.workerWG.Done()
		s.runPollWorker()
	}()
	slog.Info("invoice_workers_started")
}

// StopInvoiceWorkers 通知 worker 停止并等待退出。app shutdown 调用。
func (s *InvoiceService) StopInvoiceWorkers() {
	if s == nil {
		return
	}
	s.workerStopOnce.Do(func() {
		close(s.workerStop)
	})
	s.workerWG.Wait()
}

// --------------------------------------------------------------------------
// dispatch worker：把 queued 状态推进到 issuing
// --------------------------------------------------------------------------

func (s *InvoiceService) runDispatchWorker() {
	const fallbackInterval = 30 * time.Second
	for {
		s.tickDispatch()
		interval := s.dispatchTickInterval(fallbackInterval)
		select {
		case <-time.After(interval):
		case <-s.workerStop:
			return
		}
	}
}

func (s *InvoiceService) dispatchTickInterval(fallback time.Duration) time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil || settings == nil {
		return fallback
	}
	v := settings.InvoicePoller.IntervalSeconds
	if v <= 0 {
		return fallback
	}
	return time.Duration(v) * time.Second
}

func (s *InvoiceService) tickDispatch() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil || settings == nil {
		slog.Warn("invoice_dispatch_load_settings_failed", "error", err)
		return
	}
	cfg := buildCaiyuntongConfig(settings)
	if cfg.Endpoint == "" || cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" {
		// 没配置就跳过；让后续设置生效后再处理
		return
	}
	provider := caiyuntong.New(cfg, slogLogger{})

	rows, err := s.entClient.Invoice.Query().
		Where(
			invoice.ProviderEQ(InvoiceProviderCaiyuntong),
			invoice.ProviderStateIn(ProviderStateQueued, ProviderStateReversePending),
		).
		Order(dbent.Asc(invoice.FieldUpdatedAt)).
		Limit(20).
		All(ctx)
	if err != nil {
		slog.Warn("invoice_dispatch_query_failed", "error", err)
		return
	}
	for _, inv := range rows {
		select {
		case <-s.workerStop:
			return
		default:
		}

		if inv.ProviderState == ProviderStateQueued {
			s.dispatchIssue(ctx, inv, provider, &cfg)
		} else if inv.ProviderState == ProviderStateReversePending {
			s.dispatchReverseFirstStep(ctx, inv, provider)
		}
	}

	// 同 tick 内推进 reversing 中所有「需要本地决策」的步骤（red_applying / red_confirmed）；
	// red_issuing 步骤由 poll_worker 调 Query 推进。
	s.tickReverseSteps(ctx, provider)
}

// dispatchReverseFirstStep 接管 reverse_pending → reversing 的首步：
//   - 数电：apply 红字信息单 → reverse_step=red_applying
//   - 税控：直接开红票 → reverse_step=red_issuing
func (s *InvoiceService) dispatchReverseFirstStep(ctx context.Context, inv *dbent.Invoice, provider *caiyuntong.Provider) {
	params := s.buildReverseParams(inv, "")
	res, err := provider.Reverse(ctx, params)
	if err != nil {
		s.markReverseFailure(ctx, inv, err)
		return
	}
	upd := s.entClient.Invoice.UpdateOneID(inv.ID).
		SetProviderState(ProviderStateReversing).
		SetReverseStep(res.NextStep).
		SetProviderLastError("")
	if res.RedAdviceNum != "" {
		upd = upd.SetRedAdviceNum(res.RedAdviceNum)
	}
	if res.RedConfirmNum != "" {
		upd = upd.SetRedConfirmNum(res.RedConfirmNum)
	}
	if res.TraceID != "" {
		upd = upd.SetReverseTraceID(res.TraceID)
	}
	// 把红票 BillNo 持久化到 provider_payload，让后续 pollWorker 的 extractBillNo
	// 能查询红票数据（否则会 fallback 到蓝票 BillNo，财云通对已红冲的蓝票查询时
	// 返回的是关联的红票数据，引起 invoice_no 字段污染）。
	payload := mergePayload(inv.ProviderPayload, "red_bill_no", params.BillNoRed)
	upd = upd.SetProviderPayload(payload)

	if _, err := upd.Save(ctx); err != nil {
		slog.Warn("invoice_reverse_first_step_persist_failed", "invoice_id", inv.ID, "error", err)
		return
	}
	slog.Info("invoice_reverse_first_step",
		"invoice_id", inv.ID,
		"next_step", res.NextStep,
		"red_bill_no", params.BillNoRed,
	)
}

// tickReverseSteps 扫描 reverse_step 处于 red_applying / red_confirmed 的发票，推进到下一步。
// 不处理 red_issuing（由 pollWorker 调 Query 拿到红票号），不处理 red_done（终态）。
func (s *InvoiceService) tickReverseSteps(ctx context.Context, provider *caiyuntong.Provider) {
	rows, err := s.entClient.Invoice.Query().
		Where(
			invoice.ProviderEQ(InvoiceProviderCaiyuntong),
			invoice.ProviderStateEQ(ProviderStateReversing),
			invoice.ReverseStepIn(ReverseStepRedApplying, ReverseStepRedConfirmed),
		).
		Order(dbent.Asc(invoice.FieldUpdatedAt)).
		Limit(20).
		All(ctx)
	if err != nil {
		slog.Warn("invoice_reverse_step_query_failed", "error", err)
		return
	}
	for _, inv := range rows {
		select {
		case <-s.workerStop:
			return
		default:
		}
		s.advanceReverseStep(ctx, inv, provider)
	}
}

// advanceReverseStep 把单张发票从 red_applying/red_confirmed 推进到下一步。
func (s *InvoiceService) advanceReverseStep(ctx context.Context, inv *dbent.Invoice, provider *caiyuntong.Provider) {
	params := s.buildReverseParams(inv, inv.ReverseStep)
	res, err := provider.Reverse(ctx, params)
	if err != nil {
		s.markReverseFailure(ctx, inv, err)
		return
	}
	if res.NextStep == inv.ReverseStep {
		// 状态未变（如数电 red_applying 等待买方确认）。仅更新 updated_at 让排序生效。
		_, _ = s.entClient.Invoice.UpdateOneID(inv.ID).
			SetProviderState(ProviderStateReversing).
			Save(ctx)
		return
	}
	upd := s.entClient.Invoice.UpdateOneID(inv.ID).
		SetReverseStep(res.NextStep).
		SetProviderLastError("")
	if res.RedAdviceNum != "" {
		upd = upd.SetRedAdviceNum(res.RedAdviceNum)
	}
	if res.RedConfirmNum != "" {
		upd = upd.SetRedConfirmNum(res.RedConfirmNum)
	}
	if res.TraceID != "" {
		upd = upd.SetReverseTraceID(res.TraceID)
		// 进入 red_issuing 的那次调用一定带 TraceID（即刚下单红票）。
		// 把红票 BillNo 持久化到 provider_payload，让 poll worker 用同一个 BillNo 查询。
		payload := mergePayload(inv.ProviderPayload, "red_bill_no", deriveBillNoFromTrace(res.TraceID))
		upd = upd.SetProviderPayload(payload)
	}
	if _, err := upd.Save(ctx); err != nil {
		slog.Warn("invoice_reverse_step_persist_failed", "invoice_id", inv.ID, "error", err)
		return
	}
	slog.Info("invoice_reverse_step_advanced",
		"invoice_id", inv.ID,
		"from_step", inv.ReverseStep,
		"to_step", res.NextStep,
	)
}

// deriveBillNoFromTrace 财云通 requestID 格式 "{billNo}_{ms}"，反推出 billNo。
func deriveBillNoFromTrace(traceID string) string {
	idx := strings.LastIndex(traceID, "_")
	if idx <= 0 {
		return traceID
	}
	return traceID[:idx]
}

// mergePayload 复制 provider_payload 并设置 key。
func mergePayload(prev map[string]any, key string, value any) map[string]any {
	out := make(map[string]any, len(prev)+1)
	for k, v := range prev {
		out[k] = v
	}
	out[key] = value
	return out
}

// markReverseFailure 累计重试次数，超过 3 次转 reverse_failed。
func (s *InvoiceService) markReverseFailure(ctx context.Context, inv *dbent.Invoice, err error) {
	retry := inv.ProviderRetryCount + 1
	state := ProviderStateReversing
	if retry >= maxIssueRetries {
		state = ProviderStateReverseFailed
	}
	_, _ = s.entClient.Invoice.UpdateOneID(inv.ID).
		SetProviderState(state).
		SetProviderRetryCount(retry).
		SetProviderLastError(truncErr(err.Error())).
		Save(ctx)
	slog.Warn("invoice_reverse_step_failed",
		"invoice_id", inv.ID,
		"retry", retry,
		"final_state", state,
		"step", inv.ReverseStep,
		"error", err,
	)
}

// buildReverseParams 把 invoice 行 + 配置打成 caiyuntong.ReverseParams。
//
// 红票 BillNo 在首次提交红票（step=red_confirmed/red_pending 进入 red_issuing 的那次调用）
// 时生成并持久化到 provider_payload["red_bill_no"]，后续 poll 时复用，确保 poll 能查到同一笔红票。
func (s *InvoiceService) buildReverseParams(inv *dbent.Invoice, step string) caiyuntong.ReverseParams {
	billNoRed := ""
	if inv.ProviderPayload != nil {
		if v, ok := inv.ProviderPayload["red_bill_no"].(string); ok {
			billNoRed = v
		}
	}
	if billNoRed == "" {
		billNoRed = fmt.Sprintf("REV-%d-%d", inv.ID, time.Now().UnixNano())
	}
	requestID := fmt.Sprintf("%s_%d", billNoRed, time.Now().UnixMilli())

	// 蓝票发票日期（yyyyMMdd）取自 issued_at
	blueDate := ""
	if inv.IssuedAt != nil {
		blueDate = inv.IssuedAt.In(time.FixedZone("CST+8", 8*3600)).Format("20060102")
	}

	// 从 provider_payload 取原 BillNo
	blueBillNo := ""
	if inv.ProviderPayload != nil {
		if v, ok := inv.ProviderPayload["bill_no"].(string); ok {
			blueBillNo = v
		}
	}

	// 行明细：复用 invoice_items
	ctx := context.Background()
	items, _ := loadInvoiceItemsForProvider(ctx, s.entClient, inv.ID)

	return caiyuntong.ReverseParams{
		BlueBillNo:       blueBillNo,
		BlueInvoiceNo:    inv.InvoiceNo,
		BlueInvoiceDate:  blueDate,
		InvoiceTypeCode:  inv.InvoiceTypeCode,
		Title:            inv.Title,
		TaxNo:            inv.TaxNo,
		TitleType:        inv.TitleType,
		ContactEmail:     fallbackContactEmail(inv),
		Amount:           inv.Amount,
		Items:            items,
		Reason:           "01",
		Step:             step,
		RedAdviceNum:     inv.RedAdviceNum,
		RedConfirmNum:    inv.RedConfirmNum,
		BillNoRed:        billNoRed,
		RequestID:        requestID,
		BuyerAddress:     inv.BuyerAddress,
		BuyerPhone:       inv.BuyerPhone,
		BuyerBankName:    inv.BuyerBankName,
		BuyerBankAccount: inv.BuyerBankAccount,
	}
}

// dispatchIssue 调 provider.Issue() 把单张发票从 queued 推到 issuing。
func (s *InvoiceService) dispatchIssue(ctx context.Context, inv *dbent.Invoice, provider *caiyuntong.Provider, cfg *caiyuntong.Config) {
	// 加载明细
	items, err := loadInvoiceItemsForProvider(ctx, s.entClient, inv.ID)
	if err != nil {
		slog.Warn("invoice_dispatch_load_items_failed", "invoice_id", inv.ID, "error", err)
		return
	}

	billNo := fmt.Sprintf("INV-%d-%d", inv.ID, time.Now().UnixNano())
	requestID := fmt.Sprintf("%s_%d", billNo, time.Now().UnixMilli())

	invoiceTypeCode := inv.InvoiceTypeCode
	if invoiceTypeCode == "" {
		invoiceTypeCode = cfg.InvoiceTypeFor(inv.InvoiceKind)
	}

	traceID, issueErr := provider.Issue(ctx, caiyuntong.IssueParams{
		BillNo:           billNo,
		RequestID:        requestID,
		TitleType:        inv.TitleType,
		Title:            inv.Title,
		TaxNo:            inv.TaxNo,
		ContactEmail:     fallbackContactEmail(inv),
		Amount:           inv.Amount,
		InvoiceTypeCode:  invoiceTypeCode,
		Items:            items,
		BuyerAddress:     inv.BuyerAddress,
		BuyerPhone:       inv.BuyerPhone,
		BuyerBankName:    inv.BuyerBankName,
		BuyerBankAccount: inv.BuyerBankAccount,
	})
	if issueErr != nil {
		retry := inv.ProviderRetryCount + 1
		state := ProviderStateQueued
		if retry >= maxIssueRetries {
			state = ProviderStateFailed
		}
		_, _ = s.entClient.Invoice.UpdateOneID(inv.ID).
			SetProviderState(state).
			SetProviderLastError(truncErr(issueErr.Error())).
			SetProviderRetryCount(retry).
			Save(ctx)
		slog.Warn("invoice_issue_failed",
			"invoice_id", inv.ID,
			"retry", retry,
			"final_state", state,
			"error", issueErr,
		)
		return
	}

	if _, err := s.entClient.Invoice.UpdateOneID(inv.ID).
		SetProviderState(ProviderStateIssuing).
		SetProviderTraceID(traceID).
		SetInvoiceTypeCode(invoiceTypeCode).
		SetProviderLastError("").
		SetProviderPayload(map[string]any{"bill_no": billNo, "request_id": requestID}).
		Save(ctx); err != nil {
		slog.Warn("invoice_issue_state_persist_failed", "invoice_id", inv.ID, "error", err)
		return
	}
	slog.Info("invoice_issue_submitted",
		"invoice_id", inv.ID,
		"trace_id", traceID,
		"bill_no", billNo,
	)
}

// --------------------------------------------------------------------------
// poll worker：把 issuing/reversing 推进到终态
// --------------------------------------------------------------------------

func (s *InvoiceService) runPollWorker() {
	const fallbackInterval = 30 * time.Second
	for {
		s.tickPoll()
		select {
		case <-time.After(s.pollTickInterval(fallbackInterval)):
		case <-s.workerStop:
			return
		}
	}
}

func (s *InvoiceService) pollTickInterval(fallback time.Duration) time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil || settings == nil {
		return fallback
	}
	v := settings.InvoicePoller.IntervalSeconds
	if v <= 0 {
		return fallback
	}
	return time.Duration(v) * time.Second
}

func (s *InvoiceService) tickPoll() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil || settings == nil {
		return
	}
	cfg := buildCaiyuntongConfig(settings)
	if cfg.Endpoint == "" || cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" {
		return
	}
	provider := caiyuntong.New(cfg, slogLogger{})

	rows, err := s.entClient.Invoice.Query().
		Where(
			invoice.ProviderEQ(InvoiceProviderCaiyuntong),
			invoice.Or(
				invoice.ProviderStateEQ(ProviderStateIssuing),
				invoice.And(
					invoice.ProviderStateEQ(ProviderStateReversing),
					invoice.ReverseStepEQ(ReverseStepRedIssuing),
				),
			),
		).
		Order(dbent.Asc(invoice.FieldUpdatedAt)).
		Limit(20).
		All(ctx)
	if err != nil {
		slog.Warn("invoice_poll_query_failed", "error", err)
		return
	}

	for _, inv := range rows {
		select {
		case <-s.workerStop:
			return
		default:
		}
		s.pollOne(ctx, inv, provider)
	}
}

func (s *InvoiceService) pollOne(ctx context.Context, inv *dbent.Invoice, provider *caiyuntong.Provider) {
	billNo := extractBillNo(inv)
	if billNo == "" {
		// payload 里没有 bill_no（旧数据/异常），跳过
		return
	}
	isRed := inv.ProviderState == ProviderStateReversing
	res, err := provider.Query(ctx, caiyuntong.QueryParams{
		BillNo:          billNo,
		InvoiceTypeCode: inv.InvoiceTypeCode,
		IsRed:           isRed,
	})
	if err != nil {
		slog.Warn("invoice_query_failed", "invoice_id", inv.ID, "error", err)
		return
	}

	switch res.Stage {
	case "issued":
		if !isRed {
			s.handleIssueSuccess(ctx, inv, res)
		} else {
			s.handleReverseSuccess(ctx, inv, res)
		}
	case "failed":
		state := ProviderStateFailed
		if isRed {
			state = ProviderStateReverseFailed
		}
		_, _ = s.entClient.Invoice.UpdateOneID(inv.ID).
			SetProviderState(state).
			SetProviderLastError(truncErr(res.Reason)).
			Save(ctx)
		slog.Warn("invoice_provider_reported_failure",
			"invoice_id", inv.ID,
			"is_red", isRed,
			"reason", res.Reason,
		)
	default:
		// pending: 检查是否超时
		s.maybeMarkTimeout(ctx, inv)
	}
}

func (s *InvoiceService) handleIssueSuccess(ctx context.Context, inv *dbent.Invoice, res *caiyuntong.QueryResult) {
	if s.pdfStore == nil {
		slog.Warn("invoice_pdf_store_unavailable", "invoice_id", inv.ID)
		return
	}
	// 优先用响应内嵌的 base64 PDF — 测试环境的 DownloadUrl 是 HTML 预览页，下载会得 HTML
	var pdfBytes []byte
	if len(res.PDFBytes) > 0 {
		pdfBytes = res.PDFBytes
	} else if res.PDFURL != "" {
		b, err := fetchPDF(ctx, res.PDFURL)
		if err != nil {
			slog.Warn("invoice_pdf_download_failed", "invoice_id", inv.ID, "url", res.PDFURL, "error", err)
			return
		}
		pdfBytes = b
	} else {
		slog.Warn("invoice_pdf_no_source", "invoice_id", inv.ID)
		return
	}
	if !looksLikePDF(pdfBytes) {
		slog.Warn("invoice_pdf_invalid_format",
			"invoice_id", inv.ID,
			"size", len(pdfBytes),
			"head", string(pdfBytes[:min(len(pdfBytes), 32)]),
		)
		return
	}
	pdfName := fmt.Sprintf("%s.pdf", res.InvoiceNo)
	key, size, err := s.pdfStore.Put(ctx, inv.ID, bytesReader(pdfBytes))
	if err != nil {
		slog.Warn("invoice_pdf_store_failed", "invoice_id", inv.ID, "error", err)
		return
	}
	now := time.Now()
	if err := s.markIssuedAfterAutoOpen(ctx, inv.ID, res.InvoiceNo, key, size, pdfName, now); err != nil {
		slog.Warn("invoice_mark_issued_failed", "invoice_id", inv.ID, "error", err)
		return
	}
	slog.Info("invoice_issued_auto",
		"invoice_id", inv.ID,
		"invoice_no", res.InvoiceNo,
	)
	if err := s.sendInvoiceIssuedEmail(ctx, inv.ID); err != nil {
		slog.Warn("invoice_issued_email_failed", "invoice_id", inv.ID, "error", err)
	}
}

func (s *InvoiceService) handleReverseSuccess(ctx context.Context, inv *dbent.Invoice, res *caiyuntong.QueryResult) {
	// 下载红票 PDF（可选）
	var pdfPath string
	if res.PDFURL != "" && s.pdfStore != nil {
		if pdfBytes, err := fetchPDF(ctx, res.PDFURL); err == nil {
			if key, _, perr := s.pdfStore.Put(ctx, inv.ID, bytesReader(pdfBytes)); perr == nil {
				pdfPath = key
			}
		}
	}
	// 标记 voided + 释放订单
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()

	upd := tx.Invoice.UpdateOneID(inv.ID).
		SetStatus(InvoiceStatusVoided).
		SetProviderState(ProviderStateReverseSuccess).
		SetRedInvoiceNo(res.InvoiceNo).
		SetReverseStep(ReverseStepRedDone)
	if pdfPath != "" {
		upd = upd.SetRedPdfPath(pdfPath)
	}
	if _, err := upd.Save(ctx); err != nil {
		slog.Warn("invoice_reverse_persist_failed", "invoice_id", inv.ID, "error", err)
		return
	}
	if err := releaseInvoiceOrders(ctx, tx, inv.ID); err != nil {
		slog.Warn("invoice_reverse_release_orders_failed", "invoice_id", inv.ID, "error", err)
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Warn("invoice_reverse_commit_failed", "invoice_id", inv.ID, "error", err)
		return
	}
	slog.Info("invoice_reverse_completed",
		"invoice_id", inv.ID,
		"red_invoice_no", res.InvoiceNo,
	)
	// 红冲成功后驱动实际资金退款
	if s.refundExecutor != nil {
		if err := s.refundExecutor.FinalizeRefundAfterReverse(ctx, inv.ID); err != nil {
			slog.Warn("invoice_finalize_refund_failed", "invoice_id", inv.ID, "error", err)
		}
	}
	// 通知用户
	if err := s.sendInvoiceReversedEmail(ctx, inv.ID); err != nil {
		slog.Warn("invoice_reversed_email_failed", "invoice_id", inv.ID, "error", err)
	}
}

// sendInvoiceReversedEmail 给用户发送「红冲完成 + 退款已发起」通知。
func (s *InvoiceService) sendInvoiceReversedEmail(ctx context.Context, invoiceID int64) error {
	if s == nil || s.emailService == nil {
		return nil
	}
	inv, err := s.entClient.Invoice.Get(ctx, invoiceID)
	if err != nil {
		return err
	}
	to := strings.TrimSpace(inv.ContactEmail)
	if to == "" {
		to = strings.TrimSpace(inv.UserEmail)
	}
	if to == "" {
		return nil
	}
	siteName := "Sub2API"
	if s.settingService != nil {
		if ps, err := s.settingService.GetPublicSettings(ctx); err == nil && ps != nil && ps.SiteName != "" {
			siteName = ps.SiteName
		}
	}
	subject := fmt.Sprintf("[%s] 发票已红冲，退款已发起", siteName)
	body := fmt.Sprintf(`<!DOCTYPE html>
<html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f7f7f7;padding:24px;">
<div style="max-width:560px;margin:0 auto;background:#fff;border-radius:12px;padding:32px;border:1px solid #e5e5e5;">
  <h2 style="margin:0 0 16px;color:#1c282e;font-size:20px;">您的发票已红冲</h2>
  <p style="color:#555;line-height:1.6;">您好，您申请退款的订单对应发票已红冲完成，退款已发起，预计 1-3 个工作日内到账。</p>
  <table style="width:100%%;margin:20px 0;border-collapse:collapse;font-size:14px;background:#fafafa;border-radius:8px;overflow:hidden;">
    <tr><td style="padding:6px 12px;color:#888;width:90px;">原蓝票号</td><td style="padding:6px 12px;">%s</td></tr>
    <tr><td style="padding:6px 12px;color:#888;">红票号</td><td style="padding:6px 12px;font-weight:500;">%s</td></tr>
    <tr><td style="padding:6px 12px;color:#888;">金额</td><td style="padding:6px 12px;font-weight:500;">¥ %.2f</td></tr>
  </table>
  <p style="color:#999;font-size:12px;line-height:1.6;margin-top:24px;">本邮件由系统自动发送，请勿直接回复。</p>
</div>
</body></html>`,
		inv.InvoiceNo, inv.RedInvoiceNo, inv.Amount,
	)
	return s.emailService.SendEmail(ctx, to, subject, body)
}

// markIssuedAfterAutoOpen 写入 PDF 元信息并把 status / payment_orders 同步到 issued。
// 复用 AdminUploadPDF 的 invariants：status=issued, invoice_no, pdf_path 等。
func (s *InvoiceService) markIssuedAfterAutoOpen(
	ctx context.Context,
	invoiceID int64,
	invoiceNo string,
	pdfKey string,
	pdfSize int64,
	pdfName string,
	now time.Time,
) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Invoice.UpdateOneID(invoiceID).
		SetStatus(InvoiceStatusIssued).
		SetIssuedAt(now).
		SetInvoiceNo(invoiceNo).
		SetPdfPath(pdfKey).
		SetPdfStorage(s.pdfStore.Storage()).
		SetPdfSize(pdfSize).
		SetPdfOriginalName(pdfName).
		SetProviderState(ProviderStateSuccess).
		SetProviderLastError("").
		Save(ctx)
	if err != nil {
		return err
	}
	if err := markInvoiceOrdersIssued(ctx, tx, invoiceID); err != nil {
		return err
	}
	return tx.Commit()
}

// maybeMarkTimeout 把卡在 issuing/reversing 太久的发票转为 failed，防止永久 pending。
func (s *InvoiceService) maybeMarkTimeout(ctx context.Context, inv *dbent.Invoice) {
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil || settings == nil {
		return
	}
	var timeout time.Duration
	if inv.ProviderState == ProviderStateReversing {
		timeout = time.Duration(settings.InvoiceReverse.TimeoutMinutes) * time.Minute
	} else {
		timeout = time.Duration(settings.InvoicePoller.TimeoutMinutes) * time.Minute
	}
	if timeout <= 0 {
		return
	}
	if time.Since(inv.UpdatedAt) < timeout {
		return
	}
	finalState := ProviderStateFailed
	if inv.ProviderState == ProviderStateReversing {
		finalState = ProviderStateReverseFailed
	}
	_, _ = s.entClient.Invoice.UpdateOneID(inv.ID).
		SetProviderState(finalState).
		SetProviderLastError(fmt.Sprintf("财云通处理超时（%s 内未返回出票结果），可能是平台积压或销方账号被限流；请稍后重试或联系开票平台技术", timeout)).
		Save(ctx)
	slog.Warn("invoice_provider_timeout",
		"invoice_id", inv.ID,
		"prev_state", inv.ProviderState,
		"final_state", finalState,
		"timeout", timeout,
	)
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

const maxIssueRetries = 3

func buildCaiyuntongConfig(s *SystemSettings) caiyuntong.Config {
	if s == nil {
		return caiyuntong.Config{}
	}
	c := s.InvoiceCaiyuntong
	return caiyuntong.Config{
		Endpoint:         c.Endpoint,
		AccessKeyID:      c.AccessKeyID,
		AccessKeySecret:  c.AccessKeySecret,
		SellerTaxNum:     c.SellerTaxNum,
		SellerName:       c.SellerName,
		SellerAddress:    c.SellerAddress,
		SellerPhone:      c.SellerPhone,
		SellerBankName:   c.SellerBankName,
		SellerBankAcc:    c.SellerBankAcc,
		Drawer:           c.Drawer,
		Payee:            c.Payee,
		Reviewer:         c.Reviewer,
		TypeForNormal:    c.TypeForNormal,
		TypeForSpecial:   c.TypeForSpecial,
		GoodsCodeDefault: c.GoodsCodeDefault,
		DefaultTaxRate:   c.DefaultTaxRate,
	}
}

func loadInvoiceItemsForProvider(ctx context.Context, entClient *dbent.Client, invoiceID int64) ([]caiyuntong.LineItem, error) {
	items, err := entClient.Invoice.Query().
		Where(invoice.IDEQ(invoiceID)).
		QueryItems().
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]caiyuntong.LineItem, 0, len(items))
	for _, it := range items {
		out = append(out, caiyuntong.LineItem{
			Name:      truncForInvoiceName(it.ProductName),
			PayAmount: it.PayAmount,
		})
	}
	return out, nil
}

func truncForInvoiceName(s string) string {
	const maxBytes = 90 // 数电限制 100 字节，留余量
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}

func fallbackContactEmail(inv *dbent.Invoice) string {
	if inv == nil {
		return ""
	}
	if strings.TrimSpace(inv.ContactEmail) != "" {
		return inv.ContactEmail
	}
	return inv.UserEmail
}

// extractBillNo 根据当前发票状态选择对应的财云通 BillNo。
//   - reversing：取 red_bill_no（红票）
//   - 其它：取 bill_no（蓝票）
func extractBillNo(inv *dbent.Invoice) string {
	if inv.ProviderPayload == nil {
		return ""
	}
	if inv.ProviderState == ProviderStateReversing {
		if v, ok := inv.ProviderPayload["red_bill_no"].(string); ok && v != "" {
			return v
		}
	}
	if v, ok := inv.ProviderPayload["bill_no"].(string); ok {
		return v
	}
	return ""
}

func truncErr(s string) string {
	if len(s) <= 1024 {
		return s
	}
	return s[:1024]
}

func bytesReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}

// looksLikePDF 检查字节流是否以 "%PDF-" 魔数开头。防止把 HTML 预览页或错误响应当 PDF 存起来。
func looksLikePDF(b []byte) bool {
	return len(b) >= 5 && string(b[:5]) == "%PDF-"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func fetchPDF(ctx context.Context, url string) ([]byte, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("empty pdf url")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pdf download http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// sendInvoiceIssuedEmail 向用户发送「发票已开具」通知。
//
// 邮件包含发票号、抬头、金额，并附带 PDF 下载链接（指向用户登录后的下载入口 /api/v1/invoices/:id/pdf）。
// 若 EmailService 未注入或 SMTP 未配置，仅落 log 跳过——避免阻塞开票主路径。
func (s *InvoiceService) sendInvoiceIssuedEmail(ctx context.Context, invoiceID int64) error {
	if s == nil || s.emailService == nil {
		return nil
	}
	inv, err := s.entClient.Invoice.Get(ctx, invoiceID)
	if err != nil {
		return err
	}
	to := strings.TrimSpace(inv.ContactEmail)
	if to == "" {
		to = strings.TrimSpace(inv.UserEmail)
	}
	if to == "" {
		slog.Warn("invoice_issued_email_skipped_no_recipient", "invoice_id", invoiceID)
		return nil
	}

	siteName := "Sub2API"
	apiBase := ""
	if s.settingService != nil {
		if ps, err := s.settingService.GetPublicSettings(ctx); err == nil && ps != nil {
			if ps.SiteName != "" {
				siteName = ps.SiteName
			}
			apiBase = strings.TrimRight(ps.APIBaseURL, "/")
		}
	}
	downloadURL := fmt.Sprintf("%s/user/invoices?highlight=%d", apiBase, inv.ID)

	subject := fmt.Sprintf("[%s] 您的发票已开具：%s", siteName, inv.InvoiceNo)
	body := buildInvoiceIssuedEmailBody(siteName, inv, downloadURL)
	return s.emailService.SendEmail(ctx, to, subject, body)
}

// buildInvoiceIssuedEmailBody 生成开票完成通知邮件正文（HTML）。
func buildInvoiceIssuedEmailBody(siteName string, inv *dbent.Invoice, downloadURL string) string {
	titleType := "个人"
	if inv.TitleType == InvoiceTitleTypeBusiness {
		titleType = "企业"
	}
	taxNoLine := ""
	if strings.TrimSpace(inv.TaxNo) != "" {
		taxNoLine = fmt.Sprintf(`<tr><td style="padding:6px 12px;color:#888;">税号</td><td style="padding:6px 12px;">%s</td></tr>`, inv.TaxNo)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f7f7f7;padding:24px;">
<div style="max-width:560px;margin:0 auto;background:#fff;border-radius:12px;padding:32px;border:1px solid #e5e5e5;">
  <h2 style="margin:0 0 16px;color:#1c282e;font-size:20px;">您的发票已开具</h2>
  <p style="color:#555;line-height:1.6;">您好，您在 %s 的开票申请已自动开具完成，请登录系统下载 PDF 版式文件。</p>
  <table style="width:100%%;margin:20px 0;border-collapse:collapse;font-size:14px;background:#fafafa;border-radius:8px;overflow:hidden;">
    <tr><td style="padding:6px 12px;color:#888;width:90px;">发票号</td><td style="padding:6px 12px;font-weight:500;">%s</td></tr>
    <tr><td style="padding:6px 12px;color:#888;">抬头</td><td style="padding:6px 12px;">%s（%s）</td></tr>
    %s
    <tr><td style="padding:6px 12px;color:#888;">金额</td><td style="padding:6px 12px;font-weight:500;">¥ %.2f</td></tr>
  </table>
  <div style="text-align:center;margin:24px 0;">
    <a href="%s" style="display:inline-block;background:#1c282e;color:#fff;text-decoration:none;padding:10px 28px;border-radius:8px;font-size:14px;">登录系统下载 PDF</a>
  </div>
  <p style="color:#999;font-size:12px;line-height:1.6;margin-top:24px;">如需协助，请联系平台客服。本邮件由系统自动发送，请勿直接回复。</p>
</div>
</body></html>`,
		siteName,
		inv.InvoiceNo,
		inv.Title, titleType,
		taxNoLine,
		inv.Amount,
		downloadURL,
	)
}

// slogLogger 适配 caiyuntong.Logger 接口。
type slogLogger struct{}

func (slogLogger) Debug(msg string, args ...any) { slog.Debug(msg, args...) }
func (slogLogger) Info(msg string, args ...any)  { slog.Info(msg, args...) }
func (slogLogger) Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func (slogLogger) Error(msg string, args ...any) { slog.Error(msg, args...) }

// markInvoiceOrdersIssued 把 invoice_items 关联的 payment_orders.invoice_status 标为 issued。
// 与既有 AdminUploadPDF 流程对齐：通过 invoice_id 反查关联订单，整批 UPDATE。
func markInvoiceOrdersIssued(ctx context.Context, tx *dbent.Tx, invoiceID int64) error {
	_, err := tx.PaymentOrder.Update().
		Where(paymentorder.InvoiceIDEQ(invoiceID)).
		SetInvoiceStatus(orderInvoiceStatusIssued).
		Save(ctx)
	return err
}

var _ = invoiceitem.InvoiceIDEQ // 显式引用，预留给后续 reverse worker 使用
var _ = strconv.Itoa