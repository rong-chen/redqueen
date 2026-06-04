package main

import (
	"log"

	"RedQueenSystem/config"
	"RedQueenSystem/database"
	"RedQueenSystem/routers"
	"RedQueenSystem/services"
)

func main() {
	log.Println("正在启动 RedQueenSystem 语音与硬件交互系统服务...")

	// 1. 加载系统配置参数
	cfg := config.LoadConfig()

	// 2. 初始化 PostgreSQL 数据库连接及自动迁移
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	// 同步初始化全局官方规范的 MCP 服务器，确保路由成功挂载端点
	services.InitExposeGlobalMCPServer()

	// 自动探测并握手数据库已注册的外部 MCP 服务
	services.DiscoverMCPServers()

	// 避免定义未使用的变量提示
	_ = db

	// 4. 构建并配置 Gin 路由规则
	router := routers.SetupRouter(cfg)

	// 5. 监听端口并启动 HTTP Web 服务
	serverAddress := ":" + cfg.Port
	log.Printf("HTTP 服务已成功开启，正在监听地址: %s", serverAddress)
	if err := router.Run(serverAddress); err != nil {
		log.Fatalf("HTTP 服务启动异常: %v", err)
	}
}
