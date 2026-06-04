package routers

import (
	"net/http"

	"RedQueenSystem/config"
	"RedQueenSystem/controllers"
	"RedQueenSystem/middleware"
	"RedQueenSystem/services"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/server"
)

// SetupRouter 初始化 Gin 路由引擎并配置全局路由规则与中间件
func SetupRouter(cfg config.Config) *gin.Engine {
	r := gin.Default()

	// 全局跨域 (CORS) 中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", cfg.AllowOrigins)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// 初始化各层控制器
	authCtrl := controllers.NewAuthController()
	voiceCtrl := controllers.NewVoiceController()
	voiceWSCtrl := controllers.NewVoiceWSController(cfg.WakeWord, cfg.SessionTimeout) // 实时语音 WebSocket 控制器
	configCtrl := controllers.NewConfigController()
	mcpServerCtrl := controllers.NewMCPServerController()     // 新增：外部 MCP 服务控制器

	// 健康检查测试端点
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"system":  "RedQueenSystem 语音与硬件交互系统",
			"status":  "running",
			"version": "1.0.0",
		})
	})

	// API 路由分流组
	api := r.Group("/api")
	{
		// 1. 公开的认证接口组
		auth := api.Group("/auth")
		{
			auth.POST("/login", authCtrl.Login) // 管理员登录接口
		}

		// 2. 受保护的语音功能接口组
		voice := api.Group("/voice")
		voice.Use(middleware.AuthRequired()) // 接入鉴权拦截器
		{
			voice.POST("/command", voiceCtrl.CreateCommand) // 保存并解析语音命令
			voice.GET("/history", voiceCtrl.GetHistory)    // 获取历史命令列表
		}

		// 5. 实时语音 WebSocket 端点（采用 Token 鉴权保护）
		api.GET("/voice/ws", middleware.AuthRequired(), voiceWSCtrl.HandleWebSocket)

		// 【新增】符合官方标准的标准 Model Context Protocol (MCP) SSE 服务器端点
		// 允许任何外部 MCP 客户端（如 Cursor、Claude Desktop 等）连接，并自动发现和调用我们在数据库注册的所有外部工具！
		if services.ExposeGlobalMCPServer != nil {
			sseServer := server.NewSSEServer(
				services.ExposeGlobalMCPServer,
				server.WithSSEEndpoint("/api/mcp/sse"),
				server.WithMessageEndpoint("/api/mcp/messages"),
			)
			api.GET("/mcp/sse", gin.WrapH(sseServer))
			api.POST("/mcp/messages", gin.WrapH(sseServer))
		}

		// 3. 受保护的 MCP 外部服务管理接口组
		mcp := api.Group("/mcp")
		mcp.Use(middleware.AuthRequired()) // 接入鉴权拦截器
		{
			// 外部手动注册的 MCP 服务管理接口组
			mcp.POST("/servers", mcpServerCtrl.RegisterServer)       // 注册外部服务配置
			mcp.GET("/servers", mcpServerCtrl.ListServers)           // 获取外部服务列表
			mcp.POST("/servers/test", mcpServerCtrl.TestServerHandshake) // 实时连接测试与同步握手
			mcp.DELETE("/servers/:id", mcpServerCtrl.DeleteServer)   // 删除服务配置
		}

		// 4. 受保护的系统与模型配置接口组
		sysConfig := api.Group("/config")
		sysConfig.Use(middleware.AuthRequired()) // 接入鉴权拦截器
		{
			sysConfig.GET("/model", configCtrl.GetModelConfig)    // 获取当前大模型配置
			sysConfig.POST("/model", configCtrl.UpdateModelConfig) // 更新大模型配置参数
		}
	}

	return r
}
