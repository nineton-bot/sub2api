package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InvoicePDFStore 抽象 PDF 文件存储。V1 仅提供本地磁盘实现，
// 后续可加 S3InvoicePDFStore（复用 backup 模块的 S3 client）。
type InvoicePDFStore interface {
	// Storage 返回该 store 的标识，用于落库 invoices.pdf_storage 字段（"local" | "s3" 等）。
	Storage() string

	// Put 持久化文件，返回落库的 key（存到 invoices.pdf_path）。
	// invoiceID 仅用于路径命名，非业务依赖。
	Put(ctx context.Context, invoiceID int64, src io.Reader) (key string, size int64, err error)

	// Get 流式读取，调用方负责 Close。
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete 删除文件，文件不存在时不报错（幂等）。
	Delete(ctx context.Context, key string) error
}

// LocalInvoicePDFStore 在本地磁盘存储 PDF。
//
// 路径布局：<root>/<year>/<invoice_id>-<timestampNano>.pdf
// 不直接用 sha256 命名（sha256 在外层算完才知道，先用 timestampNano 保证可写入性，
// 真正的 sha256 由 service 层另算后写到 invoices.pdf_sha256）。
type LocalInvoicePDFStore struct {
	root string
}

// NewLocalInvoicePDFStore 构造本地 PDF 存储。
// root 不存在时尝试创建（service 启动期）；失败也不阻断启动，Put 时会再尝试。
func NewLocalInvoicePDFStore(root string) *LocalInvoicePDFStore {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "./data/invoices"
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	_ = os.MkdirAll(root, 0o755)
	return &LocalInvoicePDFStore{root: root}
}

func (s *LocalInvoicePDFStore) Storage() string { return "local" }

func (s *LocalInvoicePDFStore) Put(ctx context.Context, invoiceID int64, src io.Reader) (string, int64, error) {
	if invoiceID <= 0 {
		return "", 0, errors.New("invoice id is required")
	}
	now := time.Now()
	year := now.Format("2006")
	dir := filepath.Join(s.root, year)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create invoice pdf dir: %w", err)
	}
	name := fmt.Sprintf("%d-%d.pdf", invoiceID, now.UnixNano())
	rel := filepath.Join(year, name)
	abs := filepath.Join(s.root, rel)

	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, fmt.Errorf("open pdf file: %w", err)
	}
	defer func() { _ = f.Close() }()

	n, err := io.Copy(f, src)
	if err != nil {
		_ = os.Remove(abs)
		return "", 0, fmt.Errorf("write pdf file: %w", err)
	}
	return rel, n, nil
}

func (s *LocalInvoicePDFStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	abs, err := s.resolveSafePath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("invoice pdf not found: %w", err)
		}
		return nil, fmt.Errorf("open invoice pdf: %w", err)
	}
	return f, nil
}

func (s *LocalInvoicePDFStore) Delete(ctx context.Context, key string) error {
	abs, err := s.resolveSafePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete invoice pdf: %w", err)
	}
	return nil
}

// resolveSafePath 防止 key 通过 ".." 跨出 root 目录。
func (s *LocalInvoicePDFStore) resolveSafePath(key string) (string, error) {
	if key == "" {
		return "", errors.New("invoice pdf key is empty")
	}
	clean := filepath.Clean(key)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid invoice pdf key: %q", key)
	}
	abs := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid invoice pdf key: %q", key)
	}
	return abs, nil
}
