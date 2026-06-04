package models

import "gorm.io/gorm"

// MCPServer 记录手动添加并注册的外部 Model Context Protocol (MCP) 服务配置
type MCPServer struct {
	gorm.Model
	Name     string `gorm:"size:150;not null" json:"name"`       // 外部 MCP 服务名称
	BaseURL  string `gorm:"size:255;not null" json:"base_url"`   // 外部 MCP 服务端点地址 (例如 http://localhost:8080/api/mcp/rpc 或 http://localhost:8080/sse)
	Type     string `gorm:"size:20;default:'http'" json:"type"`  // 连接类型: http (JSON-RPC) 或 sse (Server-Sent Events)
	Method   string `gorm:"size:10;default:'POST'" json:"method"` // 请求方法: GET, POST (仅用于 http 类型)
	Headers  string `gorm:"type:text" json:"headers"`            // JSON 序列化的自定义请求头 (例如 {"Authorization": "Bearer xxx"})
	Params   string `gorm:"type:text" json:"params"`             // JSON 序列化的自定义初始化配置参数
	IsActive bool   `gorm:"default:true" json:"is_active"`       // 是否激活状态
	Status   string `gorm:"size:50;default:'offline'" json:"status"` // 连通状态: online, offline
}
