package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"RedQueenSystem/database"
	"RedQueenSystem/models"
	"RedQueenSystem/services"

	"github.com/gin-gonic/gin"
)

type MCPServerController struct{}

func NewMCPServerController() *MCPServerController {
	return &MCPServerController{}
}

// TestRequest 定义测试连接的请求体
type TestRequest struct {
	BaseURL string `json:"base_url" binding:"required"`
	Headers string `json:"headers"` // JSON string
	Params  string `json:"params"`  // JSON string
}

// TestServerHandshake 执行真实的 MCP JSON-RPC 2.0 握手测试
func (ctrl *MCPServerController) TestServerHandshake(c *gin.Context) {
	var req TestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数格式错误: " + err.Error()})
		return
	}

	// 1. 发送 tools/list 标准测试请求体
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"id":      1,
	}

	jsonBytes, err := json.Marshal(rpcReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "无法构建 JSON-RPC 请求"})
		return
	}

	httpReq, err := http.NewRequest("POST", req.BaseURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的 Base URL 地址: " + err.Error()})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// 2. 解析并附加自定义 Headers
	if req.Headers != "" {
		var headersMap map[string]string
		if err := json.Unmarshal([]byte(req.Headers), &headersMap); err == nil {
			for k, v := range headersMap {
				httpReq.Header.Set(k, v)
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Headers JSON 格式解析失败，请检查语法"})
			return
		}
	}

	// 3. 执行请求（设置 3 秒超时）
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"status":  "offline",
			"message": fmt.Sprintf("连接失败: %v (请检查服务地址或网络)", err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{
			"code":    resp.StatusCode,
			"status":  "offline",
			"message": fmt.Sprintf("服务握手失败: HTTP 状态码 %d", resp.StatusCode),
		})
		return
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"status":  "offline",
			"message": "无法读取响应内容",
		})
		return
	}

	// 4. 验证响应是否为标准的 MCP JSON-RPC 2.0
	var rpcResp struct {
		JsonRPC string      `json:"jsonrpc"`
		Result  interface{} `json:"result"`
		Error   interface{} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &rpcResp); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"status":  "offline",
			"message": "接口测试通畅，但返回内容不符合标准 MCP JSON-RPC 2.0 规范，请检查外部服务实现",
		})
		return
	}

	if rpcResp.JsonRPC != "2.0" {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"status":  "offline",
			"message": "返回的 jsonrpc 版本不为 2.0",
		})
		return
	}

	if rpcResp.Error != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"status":  "offline",
			"message": fmt.Sprintf("MCP 接口返回错误: %v", rpcResp.Error),
		})
		return
	}

	// 成功握手且符合规范！
	go services.DiscoverMCPServers()

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"status":  "online",
		"message": "测试连接成功！已检测到外部 MCP 服务处于在线状态，并成功握手同步工具集。",
	})
}

// RegisterServer 注册添加一个新的外部 MCP 配置到数据库
func (ctrl *MCPServerController) RegisterServer(c *gin.Context) {
	var req models.MCPServer
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数解析失败: " + err.Error()})
		return
	}

	// 验证必填字段
	if req.Name == "" || req.BaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "服务名称和 Base URL 不能为空"})
		return
	}

	// 校验 JSON 格式
	if req.Headers != "" {
		var temp map[string]interface{}
		if err := json.Unmarshal([]byte(req.Headers), &temp); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Headers 不是合法的 JSON 格式"})
			return
		}
	}
	if req.Params != "" {
		var temp map[string]interface{}
		if err := json.Unmarshal([]byte(req.Params), &temp); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "Params 不是合法的 JSON 格式"})
			return
		}
	}

	// 默认状态设置为激活，状态等待发现
	req.IsActive = true
	req.Status = "offline"

	if err := database.DB.Create(&req).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "写入数据库失败: " + err.Error()})
		return
	}

	go services.DiscoverMCPServers()

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "外部 MCP 服务注册成功！后端已加入服务自动发现列表。",
		"data":    req,
	})
}

// ListServers 列出所有的外部 MCP 服务
func (ctrl *MCPServerController) ListServers(c *gin.Context) {
	var servers []models.MCPServer
	if err := database.DB.Order("id desc").Find(&servers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": servers,
	})
}

// DeleteServer 删除外部注册的 MCP 服务
func (ctrl *MCPServerController) DeleteServer(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的服务 ID"})
		return
	}

	if err := database.DB.Delete(&models.MCPServer{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "删除失败: " + err.Error()})
		return
	}

	go services.DiscoverMCPServers()

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "服务配置删除成功",
	})
}
