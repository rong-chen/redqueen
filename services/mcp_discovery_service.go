package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"RedQueenSystem/database"
	"RedQueenSystem/models"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DiscoverMCPServers 遍历数据库中所有注册的外部 MCP 服务，并行执行握手检测并更新在线状态
func DiscoverMCPServers() {
	// 开启后台协程，防止阻塞主服务启动
	go func() {
		log.Println("【服务发现】开始在后台扫描数据库注册的外部 MCP 服务...")

		var servers []models.MCPServer
		err := database.DB.Where("is_active = ?", true).Find(&servers).Error
		if err != nil {
			log.Printf("【服务发现】加载外部 MCP 服务失败: %v", err)
			return
		}

		if len(servers) == 0 {
			log.Println("【服务发现】未检测到任何注册的外部 MCP 服务，跳过初始化官方标准 MCP 服务")
			return
		}

		var wg sync.WaitGroup
		client := &http.Client{Timeout: 3 * time.Second}

		for _, s := range servers {
			wg.Add(1)
			go func(srv models.MCPServer) {
				defer wg.Done()
				status := "offline"

				// 发送 tools/list 标准测试请求
				rpcReq := map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "tools/list",
					"id":      1,
				}
				jsonBytes, _ := json.Marshal(rpcReq)

				req, err := http.NewRequest("POST", srv.BaseURL, bytes.NewBuffer(jsonBytes))
				if err == nil {
					req.Header.Set("Content-Type", "application/json")

					// 附加 Headers
					if srv.Headers != "" {
						var headersMap map[string]string
						if err := json.Unmarshal([]byte(srv.Headers), &headersMap); err == nil {
							for k, v := range headersMap {
								req.Header.Set(k, v)
							}
						}
					}

					resp, err := client.Do(req)
					if err == nil {
						defer resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							// 只要 HTTP 200 即证明端点联通
							status = "online"
						}
					}
				}

				// 更新数据库中的联通状态
				database.DB.Model(&srv).Update("status", status)
				log.Printf("【服务发现】已探测外部 MCP 服务 [%s] (%s) -> 状态: %s", srv.Name, srv.BaseURL, status)
			}(s)
		}

		// 等待所有探测协程执行完毕
		wg.Wait()
		log.Println("【服务发现】所有外部 MCP 服务状态探测完毕！")

		// 构建并实例化全局官方标准规格的 MCP 服务端，合并所有动态工具！
		BuildExposeGlobalMCPServer()
	}()
}

// ExposeGlobalMCPServer 全局共享的符合官方规范的 MCP 服务器实例
var ExposeGlobalMCPServer *server.MCPServer

// InitExposeGlobalMCPServer 同步创建最基础的规范全局 MCP 服务端，防止路由启动时为 nil
func InitExposeGlobalMCPServer() {
	log.Println("【MCP Gateway】正在同步创建全局标准 MCP 服务器实例...")
	ExposeGlobalMCPServer = server.NewMCPServer("RedQueen-MCP-Gateway", "1.0.0")
}

// BuildExposeGlobalMCPServer 在项目启动或配置变更时动态扫描数据库在线服务，清除旧工具，并重新绑定最新外部工具
func BuildExposeGlobalMCPServer() {
	log.Println("【MCP Gateway】正在构建/重装载全局标准 MCP 服务端的动态工具集...")

	if ExposeGlobalMCPServer == nil {
		InitExposeGlobalMCPServer()
	}

	// 1. 清理已注册的工具，防止重复或残留
	var existingTools []string
	for name := range ExposeGlobalMCPServer.ListTools() {
		existingTools = append(existingTools, name)
	}
	if len(existingTools) > 0 {
		ExposeGlobalMCPServer.DeleteTools(existingTools...)
	}

	// 2. 动态获取当前数据库在线的所有外部工具列表
	mcpTools, _, _ := GetExternalMCPTools()

	// 3. 将这些工具动态注册到该标准服务上，并统一绑定 DynamicMCPToolHandler 代理路由器
	for _, t := range mcpTools {
		ExposeGlobalMCPServer.AddTool(t, DynamicMCPToolHandler)
	}

	log.Printf("【MCP Gateway】官方标准 MCP 服务热装载完成，当前共对外暴露了 %d 个标准工具端点！", len(mcpTools))
}

// DynamicMCPToolHandler 是官方标准的 ToolHandlerFunc，负责接收外部的标准 tools/call 请求，并动态向后级真实的外部服务做请求路由与代理转发
func DynamicMCPToolHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName := req.Params.Name
	log.Printf("【MCP Gateway】接收到官方标准 tools/call 调度请求: %s", toolName)

	// 1. 获取动态映射表
	_, _, toolToServerMap := GetExternalMCPTools()
	srv, exists := toolToServerMap[toolName]
	if !exists {
		return mcp.NewToolResultError(fmt.Sprintf("未在后台找到能够执行工具 [%s] 的在线 MCP 服务器配置", toolName)), nil
	}

	// 2. 序列化参数为 JSON 字节
	argsBytes, err := json.Marshal(req.Params.Arguments)
	if err != nil {
		return mcp.NewToolResultError("序列化参数结构失败"), nil
	}

	// 3. 动态发起请求并转发执行
	respText, err := CallExternalMCPTool(srv, toolName, string(argsBytes))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("外部服务处理异常: %v", err)), nil
	}

	// 4. 返回标准文本结果
	return mcp.NewToolResultText(respText), nil
}

// GetExternalMCPTools 动态获取当前所有在线的外部 MCP 服务声明的工具列表
func GetExternalMCPTools() ([]mcp.Tool, []map[string]interface{}, map[string]models.MCPServer) {
	var mcpTools []mcp.Tool
	var openAITools []map[string]interface{}
	toolToServerMap := make(map[string]models.MCPServer)

	var servers []models.MCPServer
	err := database.DB.Where("is_active = ? AND status = ?", true, "online").Find(&servers).Error
	if err != nil {
		log.Printf("【MCP 动态工具提取】读取数据库失败: %v", err)
		return mcpTools, openAITools, toolToServerMap
	}

	client := &http.Client{Timeout: 3 * time.Second}

	for _, srv := range servers {
		var req *http.Request
		var err error

		method := strings.ToUpper(srv.Method)
		if method == "" {
			method = "POST"
		}

		if method == "GET" {
			fullURL := srv.BaseURL + "?jsonrpc=2.0&method=tools/list&id=1"
			req, err = http.NewRequest("GET", fullURL, nil)
		} else {
			rpcReq := map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "tools/list",
				"id":      1,
			}
			jsonBytes, marshalErr := json.Marshal(rpcReq)
			if marshalErr != nil {
				continue
			}
			req, err = http.NewRequest("POST", srv.BaseURL, bytes.NewBuffer(jsonBytes))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
			}
		}

		if err != nil {
			continue
		}

		// 附加自定义 Headers
		if srv.Headers != "" {
			var headersMap map[string]string
			if err := json.Unmarshal([]byte(srv.Headers), &headersMap); err == nil {
				for k, v := range headersMap {
					req.Header.Set(k, v)
				}
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		// 2. 解析返回的工具列表
		var rpcResp struct {
			JsonRPC string `json:"jsonrpc"`
			Result  struct {
				Tools []struct {
					Name        string                 `json:"name"`
					Description string                 `json:"description"`
					InputSchema map[string]interface{} `json:"inputSchema"`
				} `json:"tools"`
			} `json:"result"`
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		if err := json.Unmarshal(bodyBytes, &rpcResp); err != nil {
			continue
		}

		// 3. 动态转换为 mcp-go 的 mcp.Tool 及 OpenAI 兼容格式
		for _, rawTool := range rpcResp.Result.Tools {
			if rawTool.Name == "" {
				continue
			}

			// 反序列化 InputSchema 并封装为 mcp.ToolInputSchema
			var inputSchema mcp.ToolInputSchema
			schemaBytes, _ := json.Marshal(rawTool.InputSchema)
			_ = json.Unmarshal(schemaBytes, &inputSchema)

			mcpTool := mcp.Tool{
				Name:        rawTool.Name,
				Description: rawTool.Description,
				InputSchema: inputSchema,
			}
			mcpTools = append(mcpTools, mcpTool)

			openAITool := map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        rawTool.Name,
					"description": rawTool.Description,
					"parameters":  rawTool.InputSchema,
				},
			}
			openAITools = append(openAITools, openAITool)

			toolToServerMap[rawTool.Name] = srv
			log.Printf("【MCP 动态发现】已成功发现并封装外部工具 [%s] (来自服务: %s)", rawTool.Name, srv.Name)
		}
	}

	return mcpTools, openAITools, toolToServerMap
}

// CallExternalMCPTool 转发工具调用请求至对应的外部 MCP 服务
func CallExternalMCPTool(srv models.MCPServer, toolName string, arguments string) (string, error) {
	log.Printf("【MCP 动态转发】正在将工具 [%s] 转发至外部服务 [%s] (%s)...", toolName, srv.Name, srv.BaseURL)

	// 1. 构建标准的 tools/call JSON-RPC 2.0 请求
	var parsedArgs map[string]interface{}
	_ = json.Unmarshal([]byte(arguments), &parsedArgs)

	var req *http.Request
	var err error

	method := strings.ToUpper(srv.Method)
	if method == "" {
		method = "POST"
	}

	if method == "GET" {
		paramsObj := map[string]interface{}{
			"name":      toolName,
			"arguments": parsedArgs,
		}
		paramsJSON, _ := json.Marshal(paramsObj)
		fullURL := fmt.Sprintf("%s?jsonrpc=2.0&method=tools/call&params=%s&id=1",
			srv.BaseURL, url.QueryEscape(string(paramsJSON)))
		req, err = http.NewRequest("GET", fullURL, nil)
	} else {
		rpcReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      toolName,
				"arguments": parsedArgs,
			},
			"id": 1,
		}
		jsonBytes, marshalErr := json.Marshal(rpcReq)
		if marshalErr != nil {
			return "", fmt.Errorf("构建请求失败: %v", marshalErr)
		}
		req, err = http.NewRequest("POST", srv.BaseURL, bytes.NewBuffer(jsonBytes))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	if err != nil {
		return "", fmt.Errorf("构建请求对象失败: %v", err)
	}

	// 附加 Custom Headers
	if srv.Headers != "" {
		var headersMap map[string]string
		if err := json.Unmarshal([]byte(srv.Headers), &headersMap); err == nil {
			for k, v := range headersMap {
				req.Header.Set(k, v)
			}
		}
	}

	// 2. 发送请求
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("网络连接失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("外部服务返回异常 HTTP 状态码 %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	// 3. 解析标准 MCP Content 返回
	var rpcResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error interface{} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &rpcResp); err != nil {
		return "", fmt.Errorf("反序列化 JSON-RPC 失败: %v", err)
	}

	if rpcResp.Error != nil {
		return "", fmt.Errorf("MCP 服务返回业务错误: %v", rpcResp.Error)
	}

	if len(rpcResp.Result.Content) > 0 {
		// 返回第一条内容文本
		return rpcResp.Result.Content[0].Text, nil
	}

	return "指令已发送，但外部服务未返回提示语", nil
}
