package api

import (
	"github.com/fyerfyer/doc-QA-system/api/handler"
	"github.com/fyerfyer/doc-QA-system/api/middleware"
	"github.com/gin-gonic/gin"
)

// SetupRouter 设置API路由
// 配置所有的API端点并应用中间件
func SetupRouter(
	docHandler *handler.DocumentHandler,
	qaHandler *handler.QAHandler,
) *gin.Engine {
	// 创建默认的Gin路由引擎
	router := gin.Default()

	// 应用全局中间件
	router.Use(middleware.Logger())
	router.Use(middleware.ErrorHandler())
	router.Use(middleware.SetTraceID())

	// 在调试模式下记录请求体和响应体
	if gin.Mode() == gin.DebugMode {
		router.Use(middleware.RequestLogger())
	}

	// 创建API分组
	api := router.Group("/api")
	{
		// 文档管理API
		docGroup := api.Group("/documents")
		{
			// 上传文档 - POST /api/documents
			docGroup.POST("", docHandler.UploadDocument)

			// 获取文档状态 - GET /api/documents/:id/status
			docGroup.GET("/:id/status", docHandler.GetDocumentStatus)

			// 获取文档列表 - GET /api/documents
			docGroup.GET("", docHandler.ListDocuments)

			// 删除文档 - DELETE /api/documents/:id
			docGroup.DELETE("/:id", docHandler.DeleteDocument)
		}

		// 问答API
		qaGroup := api.Group("/qa")
		{
			// 回答问题 - POST /api/qa
			qaGroup.POST("", qaHandler.AnswerQuestion)
		}

		// 健康检查API
		api.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status": "ok",
			})
		})
	}

	return router
}

// RegisterSwagger 注册Swagger文档路由
// TODO: 当集成Swagger文档后实现此函数
func RegisterSwagger(router *gin.Engine) {
	// 待实现：集成Swagger API文档
}

// RegisterWebUI 注册Web UI路由
// TODO: 当前端页面准备好后实现此函数
func RegisterWebUI(router *gin.Engine) {
	// 待实现：集成前端页面
	// 示例：router.StaticFile("/", "./web/dist/index.html")
	// 示例：router.Static("/static", "./web/dist/static")
}

// RegisterRateLimiter 注册限流器
// TODO: 当需要限流功能时实现此函数
func RegisterRateLimiter(router *gin.Engine) {
	// 待实现：添加API请求限流功能
}

// Cors 跨域资源共享中间件
// 如果需要支持跨域请求，可以启用此中间件
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Trace-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
