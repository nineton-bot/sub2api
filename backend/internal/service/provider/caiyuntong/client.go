package caiyuntong

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config 财云通对接所需的运行时配置。
//
// 来源于 SettingService（key 前缀 invoice.caiyuntong.*）。
// AccessKeySecret 应在 SettingService 层用现有加密通道保存，传到这里时已是明文。
type Config struct {
	Endpoint        string // e.g. https://api-dataservice.bigfintax.com/
	AccessKeyID     string
	AccessKeySecret string
	SellerTaxNum    string
	SellerName      string
	SellerAddress   string
	SellerPhone     string
	SellerBankName  string
	SellerBankAcc   string
	Drawer          string
	Payee           string
	Reviewer        string
	// 票种代码映射（财云通 InvoiceType）。
	//   普票（normal）默认 06 数电普
	//   专票（special）默认 05 数电专
	TypeForNormal      string
	TypeForSpecial     string
	GoodsCodeDefault   string  // 默认税收分类编码
	DefaultTaxRate     float64 // 默认税率，例 0.06 (6%)；为 0 时 fallback 到 6%
	HTTPTimeoutSeconds int     // 单次请求超时，默认 30
}

// Client 封装财云通 HTTP 调用 + 签名。
type Client struct {
	cfg    Config
	http   *http.Client
	logger Logger
}

// Logger 让外层注入 slog/log，避免 service 包直接依赖。
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// NewClient 创建一个 Client。logger 可为 nil。
func NewClient(cfg Config, logger Logger) *Client {
	to := cfg.HTTPTimeoutSeconds
	if to <= 0 {
		to = 30
	}
	if logger == nil {
		logger = noopLogger{}
	}
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: time.Duration(to) * time.Second},
		logger: logger,
	}
}

// Config 暴露只读视图给上层使用。
func (c *Client) Config() Config { return c.cfg }

// postJSON 给指定 path（如 "/invoice/createInvoice"）发签名请求。
// 返回响应原文（JSON 字节），由调用方解码。
func (c *Client) postJSON(ctx context.Context, path string, body any) ([]byte, error) {
	if c.cfg.Endpoint == "" || c.cfg.AccessKeyID == "" || c.cfg.AccessKeySecret == "" {
		return nil, fmt.Errorf("caiyuntong: missing endpoint / access key configuration")
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("caiyuntong: marshal request: %w", err)
	}

	url := joinEndpoint(c.cfg.Endpoint, path) + "?" + BuildSignedQuery(c.cfg.AccessKeyID, c.cfg.AccessKeySecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("caiyuntong: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	bodyPreview := string(payload)
	if len(bodyPreview) > 1500 {
		bodyPreview = bodyPreview[:1500] + "...(truncated)"
	}
	c.logger.Debug("caiyuntong_request", "path", path, "body_len", len(payload), "body", bodyPreview)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("caiyuntong: http do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("caiyuntong: read body: %w", err)
	}

	respPreview := string(respBody)
	if len(respPreview) > 4000 {
		respPreview = respPreview[:4000] + "...(truncated)"
	}
	c.logger.Debug("caiyuntong_response", "path", path, "status", resp.StatusCode, "body_len", len(respBody), "body", respPreview)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 透传一部分响应体便于排查
		preview := string(respBody)
		if len(preview) > 512 {
			preview = preview[:512]
		}
		return respBody, fmt.Errorf("caiyuntong: http %d: %s", resp.StatusCode, preview)
	}
	return respBody, nil
}

// joinEndpoint 合并 Base URL 与具体 API path，避免常见的 URL 拼接错误：
//
//   - 容忍 Base URL 末尾带或不带 "/"
//   - 容忍管理员在 Base URL 里多填了具体 endpoint 后缀（如 "/invoice/createInvoice"）
//     ——若 Base URL 的 path 已经以待拼接 path 的最后一段（例如 "createInvoice"）结尾，
//     视为重复并剥除，防止 "/invoice/createInvoice/invoice/createInvoice" 这种 404
//   - 容忍 Base URL 里包含 query 串（极少见，安全起见连同 query 一起剥）
//
// 真实的 endpoint 上下文路径（如 "/inv-xx-ports"）仍然保留 — 我们只比对最末一段。
func joinEndpoint(base, path string) string {
	if base == "" {
		return path
	}
	// 砍掉 query
	if i := strings.IndexByte(base, '?'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, "/")
	// 取 path 的最后一段（"/invoice/createInvoice" → "createInvoice"）作为重复判断
	lastSeg := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		lastSeg = path[i+1:]
	}
	if lastSeg != "" && strings.HasSuffix(base, "/"+lastSeg) {
		// e.g. base="https://x.com/inv-xx-ports/invoice/createInvoice", path="/invoice/createInvoice"
		// → 剥到 "https://x.com/inv-xx-ports/invoice"，再拼 "/createInvoice" 仍重复，所以剥两段：
		base = strings.TrimSuffix(base, "/"+lastSeg)
		base = strings.TrimRight(base, "/")
		// 如果 base 还有 "/invoice" 之类相同前缀也剥
		if i := strings.LastIndexByte(path, '/'); i > 0 {
			parent := path[:i]
			if strings.HasSuffix(base, parent) {
				base = strings.TrimSuffix(base, parent)
				base = strings.TrimRight(base, "/")
			}
		}
	}
	return base + path
}

// Ping 简单连通性测试：通过签名 + 一个空 query 调用是否能拿到 401/200 之类的有效响应，
// 不严格依赖后端业务接口的成功语义。供「测试连接」按钮使用。
func (c *Client) Ping(ctx context.Context) error {
	if c.cfg.Endpoint == "" || c.cfg.AccessKeyID == "" || c.cfg.AccessKeySecret == "" {
		return fmt.Errorf("missing config")
	}
	url := strings.TrimRight(c.cfg.Endpoint, "/") + "/invoice/createInvoice?" + BuildSignedQuery(c.cfg.AccessKeyID, c.cfg.AccessKeySecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(`{"Count":0,"RequestID":"ping","Content":[]}`)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	// 任何来自服务端的 HTTP 响应均视为可达（具体业务错误码留给真实开票链路）。
	return nil
}
