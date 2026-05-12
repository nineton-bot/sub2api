package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterInvoiceRoutes mounts user-facing and admin invoice endpoints.
//
// User endpoints (jwtAuth + BackendModeUserGuard):
//
//	GET    /invoices/eligible-orders
//	GET    /invoices
//	POST   /invoices
//	GET    /invoices/:id
//	POST   /invoices/:id/cancel
//	GET    /invoices/:id/pdf
//
// Admin endpoints (adminAuth):
//
//	GET    /admin/invoices
//	GET    /admin/invoices/:id
//	POST   /admin/invoices/:id/approve
//	POST   /admin/invoices/:id/reject
//	POST   /admin/invoices/:id/upload-pdf      (multipart)
//	POST   /admin/invoices/:id/replace-pdf     (multipart)
//	POST   /admin/invoices/:id/mark-issued
//	POST   /admin/invoices/:id/void
//	GET    /admin/invoices/:id/pdf
func RegisterInvoiceRoutes(
	v1 *gin.RouterGroup,
	invoiceHandler *handler.InvoiceHandler,
	adminInvoiceHandler *admin.InvoiceHandler,
	jwtAuth middleware.JWTAuthMiddleware,
	adminAuth middleware.AdminAuthMiddleware,
	settingService *service.SettingService,
) {
	// User endpoints
	authenticated := v1.Group("/invoices")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	{
		// 可见性查询：不走 guardVisibility，自身就是 visibility 检查
		authenticated.GET("/eligibility", invoiceHandler.GetEligibility)

		authenticated.GET("/eligible-orders", invoiceHandler.ListEligibleOrders)
		authenticated.GET("", invoiceHandler.ListMyInvoices)
		authenticated.POST("", invoiceHandler.CreateInvoice)
		authenticated.GET("/:id", invoiceHandler.GetInvoiceDetail)
		authenticated.POST("/:id/cancel", invoiceHandler.CancelInvoice)
		authenticated.GET("/:id/pdf", invoiceHandler.DownloadInvoicePDF)
	}

	// Admin endpoints (admin handler 在 C4/C5 实现；这里允许 nil 以便分阶段提交)
	if adminInvoiceHandler == nil {
		return
	}
	adminGroup := v1.Group("/admin/invoices")
	adminGroup.Use(gin.HandlerFunc(adminAuth))
	{
		adminGroup.GET("", adminInvoiceHandler.ListInvoices)
		adminGroup.GET("/:id", adminInvoiceHandler.GetInvoiceDetail)
		adminGroup.POST("/:id/approve", adminInvoiceHandler.ApproveInvoice)
		adminGroup.POST("/:id/reject", adminInvoiceHandler.RejectInvoice)
		adminGroup.POST("/:id/upload-pdf", adminInvoiceHandler.UploadPDF)
		adminGroup.POST("/:id/replace-pdf", adminInvoiceHandler.UploadPDF)
		adminGroup.POST("/:id/mark-issued", adminInvoiceHandler.MarkIssued)
		adminGroup.POST("/:id/void", adminInvoiceHandler.VoidInvoice)
		adminGroup.GET("/:id/pdf", adminInvoiceHandler.DownloadPDF)
	}
}
