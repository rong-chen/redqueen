package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"RedQueenSystem/database"
	"RedQueenSystem/models"
)

// NLPService 整合大语言模型 (DeepSeek/OpenAI) 标准 Tool Calling 协议进行语音命令解析
type NLPService struct{}

// NewNLPService 实例化大语言模型语义解析服务
func NewNLPService() *NLPService {
	return &NLPService{}
}

// ParseResult 定义大模型解析返回的标准意图提取结构
type ParseResult struct {
	Intent            string  `json:"intent"`      // 意图
	Confidence        float64 `json:"confidence"`  // 置信度
	ReplyText         string  `json:"reply_text"`  // 大模型生成的温馨口语化回复文字
	IsExternal        bool    `json:"is_external"` // 是否为外部注册 of MCP 工具调用
	ExternalToolName  string  `json:"external_tool_name"`
	ExternalArguments string  `json:"external_arguments"`
	ToolStatus        string  `json:"tool_status,omitempty"` // success / failed
	ToolError         string  `json:"tool_error,omitempty"`  // 工具执行失败时的具体错误
}

// StreamChunk 流式输出的单个数据片段
type StreamChunk struct {
	Token    string        // 新增的文本 token
	Done     bool          // 是否已完成全部输出
	ToolCall *ToolCallInfo // 如果最终是工具调用，完成时携带此信息
	Error    error         // 错误信息
}

// ToolCallInfo 工具调用信息
type ToolCallInfo struct {
	Name      string
	Arguments string
}

// prepareRequest 提取公共的配置加载、URL 构建、系统提示词插值、工具加载逻辑，供 ParseIntent 和 ParseIntentStream 共享
func (s *NLPService) prepareRequest(transcript string) (requestBody map[string]interface{}, apiURL string, apiKey string, err error) {
	// 1. 动态从数据库中获取当前最新的模型配置参数
	var cfg models.ModelConfig
	err = database.DB.First(&cfg).Error
	if err != nil {
		err = fmt.Errorf("获取大模型数据库配置失败: %v", err)
		return
	}

	// 2. 检查 Key 的有效性
	if cfg.ApiKey == "" {
		err = fmt.Errorf("大模型 API Key 尚未配置，请前往后台系统设置进行配置")
		return
	}

	// 3. 端点适配优化，自动追加标准聊天路径
	apiURL = cfg.ApiURL
	if !strings.HasSuffix(apiURL, "/chat/completions") {
		apiURL = strings.TrimSuffix(apiURL, "/")
		apiURL = apiURL + "/chat/completions"
	}

	apiKey = cfg.ApiKey

	log.Printf("【OpenAI Tool Calling】使用模型 %s 开始语义工具选择...", cfg.ModelName)

	// 4. 动态探测数据库内所有在线的外部手动注册的 MCP 服务声明的工具集，并打包作为 API 参数中唯一的 tools 选项
	var tools []map[string]interface{}
	_, externalTools, _ := GetExternalMCPTools()
	if len(externalTools) > 0 {
		tools = append(tools, externalTools...)
	}

	// 5. 动态将 SystemPrompt 中的 {{.SystemRole}} 和 {{.SystemPersonality}} 替换为具体设定
	finalSystemPrompt := cfg.SystemPrompt
	if cfg.SystemRole != "" {
		finalSystemPrompt = strings.ReplaceAll(finalSystemPrompt, "{{.SystemRole}}", cfg.SystemRole)
		finalSystemPrompt = strings.ReplaceAll(finalSystemPrompt, "{{SystemRole}}", cfg.SystemRole)
	}
	if cfg.SystemPersonality != "" {
		finalSystemPrompt = strings.ReplaceAll(finalSystemPrompt, "{{.SystemPersonality}}", cfg.SystemPersonality)
		finalSystemPrompt = strings.ReplaceAll(finalSystemPrompt, "{{SystemPersonality}}", cfg.SystemPersonality)
	}

	// 构建官方标准的 OpenAI 请求体，仅当存在外部工具时才加载 tools 字段，防止空参数报错
	requestBody = map[string]interface{}{
		"model": cfg.ModelName,
		"messages": []map[string]interface{}{
			{"role": "system", "content": finalSystemPrompt},
			{"role": "user", "content": "用户说: " + transcript},
		},
		"temperature": 0.3,
	}

	if len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}

	return
}

// ParseIntent 使用标准 OpenAI Tool Calling 接口进行语音动作提取，并在命中工具时自动调度、喂回真实数据并进行多轮递归重生成回复
func (s *NLPService) ParseIntent(transcript string) (ParseResult, error) {
	requestBody, apiURL, apiKey, err := s.prepareRequest(transcript)
	if err != nil {
		return ParseResult{}, err
	}

	// 统一转换为 []map[string]interface{} 方便动态追加多轮会话消息
	originalMessages := requestBody["messages"].([]map[string]interface{})
	currentMessages := make([]map[string]interface{}, len(originalMessages))
	copy(currentMessages, originalMessages)

	var lastReplyText string
	var finalResult ParseResult
	finalResult.Intent = "conversation"
	finalResult.Confidence = 0.98

	client := &http.Client{Timeout: 30 * time.Second}

	// 最多允许 5 轮递归工具调用，防止死循环
	for turn := 1; turn <= 5; turn++ {
		reqBody := map[string]interface{}{
			"model":       requestBody["model"],
			"messages":    currentMessages,
			"temperature": 0.3,
		}

		// 在每一轮中都带上 tools 定义，以便大模型自主决定是继续调用新工具还是生成最终回复
		if tools, ok := requestBody["tools"]; ok {
			reqBody["tools"] = tools
			reqBody["tool_choice"] = "auto"
		}

		jsonBytes, err := json.Marshal(reqBody)
		if err != nil {
			return ParseResult{}, fmt.Errorf("序列化请求 JSON 结构失败: %v", err)
		}

		req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			return ParseResult{}, fmt.Errorf("构建 HTTP 请求失败: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return ParseResult{}, fmt.Errorf("调用大模型 API 网络请求失败: %v", err)
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return ParseResult{}, fmt.Errorf("读取大模型响应流失败: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			return ParseResult{}, fmt.Errorf("大模型端点返回 HTTP 状态码 %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var apiResult struct {
			Choices []struct {
				Message struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}

		if err := json.Unmarshal(bodyBytes, &apiResult); err != nil || len(apiResult.Choices) == 0 {
			return ParseResult{}, fmt.Errorf("解析大模型响应数据失败: %v", err)
		}

		message := apiResult.Choices[0].Message
		lastReplyText = strings.TrimSpace(message.Content)

		// 检测标准 Tool Call 或 custom DSML tool call
		var toolCallName string
		var toolCallArgs string
		var toolCallID string
		var hasToolCall bool

		if len(message.ToolCalls) > 0 {
			toolCall := message.ToolCalls[0]
			toolCallName = toolCall.Function.Name
			toolCallArgs = toolCall.Function.Arguments
			toolCallID = toolCall.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("call_sync_%d", turn)
			}
			hasToolCall = true
		} else if name, args, ok := ParseDSMLToolCall(message.Content); ok {
			toolCallName = name
			toolCallArgs = args
			toolCallID = fmt.Sprintf("call_sync_dsml_%d", turn)
			hasToolCall = true
		}

		if hasToolCall {
			log.Printf("【OpenAI Tool Calling 第 %d 轮命中工具】: %s | 参数: %s", turn, toolCallName, toolCallArgs)

			finalResult.IsExternal = true
			finalResult.ExternalToolName = toolCallName
			finalResult.ExternalArguments = toolCallArgs
			finalResult.Intent = "external_mcp_call"

			// A. 后端路由并执行外部 MCP 工具，取得真实数据
			_, _, toolToServerMap := GetExternalMCPTools()
			srv, exists := toolToServerMap[toolCallName]
			var toolResult string
			if !exists {
				toolResult = fmt.Sprintf("未在后台找到执行工具 [%s] 的在线 MCP 服务器", toolCallName)
				finalResult.ToolStatus = "failed"
				finalResult.ToolError = toolResult
			} else {
				res, callErr := CallExternalMCPTool(srv, toolCallName, toolCallArgs)
				if callErr != nil {
					toolResult = "工具执行出错: " + callErr.Error()
					finalResult.ToolStatus = "failed"
					finalResult.ToolError = callErr.Error()
				} else {
					toolResult = res
					finalResult.ToolStatus = "success"
				}
			}

			log.Printf("【OpenAI Tool Calling 第 %d 轮执行结果】: %s", turn, toolResult)

			// B. 拼装 assistant 消息及 tool 消息追加至上下文历史
			assistantMsg := map[string]interface{}{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []map[string]interface{}{
					{
						"id":       toolCallID,
						"type":     "function",
						"function": map[string]interface{}{
							"name":      toolCallName,
							"arguments": toolCallArgs,
						},
					},
				},
			}
			if len(message.ToolCalls) == 0 {
				// 对于 DSML 模式下在 content 吐出的 XML 格式
				assistantMsg["content"] = message.Content
			}

			toolMsg := map[string]interface{}{
				"role":         "tool",
				"tool_call_id": toolCallID,
				"name":         toolCallName,
				"content":      toolResult,
			}

			currentMessages = append(currentMessages, assistantMsg, toolMsg)

			// 继续下一轮递归决策
			continue
		}

		// 若没有命中任何工具，说明大模型已经输出了最终的自然回复文本！
		reThink := regexp.MustCompile(`(?s)<think>.*?</think>`)
		finalResult.ReplyText = reThink.ReplaceAllString(lastReplyText, "")
		finalResult.ReplyText = strings.TrimSpace(finalResult.ReplyText)
		break
	}

	// 兜底：如果递归异常失败，直接展现最后一次接收的内容
	if finalResult.ReplyText == "" {
		if lastReplyText != "" {
			reThink := regexp.MustCompile(`(?s)<think>.*?</think>`)
			finalResult.ReplyText = reThink.ReplaceAllString(lastReplyText, "")
			finalResult.ReplyText = strings.TrimSpace(finalResult.ReplyText)
		} else {
			finalResult.ReplyText = "你好，我是红皇后，目前没有匹配到可用的控制工具或意图。"
		}
	}

	// 如果大模型既没有选择工具，也没有产生普通的 content 聊天文本，我们自动补充默认口语化文本并归类为 unknown
	if finalResult.Intent == "conversation" && finalResult.ReplyText == "" {
		finalResult.Intent = "unknown"
		finalResult.ReplyText = "你好，我是红皇后，目前没有匹配到可用的控制工具或意图。"
	}

	return finalResult, nil
}

// ParseIntentStream 使用流式 SSE 模式调用大模型 API，逐 token 返回解析结果
func (s *NLPService) ParseIntentStream(transcript string) (<-chan StreamChunk, error) {
	requestBody, apiURL, apiKey, err := s.prepareRequest(transcript)
	if err != nil {
		return nil, err
	}

	// 启用流式输出
	requestBody["stream"] = true

	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("序列化流式请求 JSON 结构失败: %v", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("构建流式 HTTP 请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 流式请求不设置超时，防止长时间生成过程被强制中断
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用大模型流式 API 网络请求失败: %v", err)
	}

	// 检查 HTTP 状态码，非 200 时直接读取错误并返回
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		var errResponse struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(bodyBytes, &errResponse)
		if errResponse.Error.Message != "" {
			return nil, fmt.Errorf("大模型服务商报错: %s", errResponse.Error.Message)
		}
		return nil, fmt.Errorf("大模型端点返回 HTTP 状态码 %d: %s", resp.StatusCode, string(bodyBytes))
	}

	ch := make(chan StreamChunk, 32)

	// 启动后台 goroutine 逐行读取 SSE 流并发送 StreamChunk
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		// 用于累积工具调用的名称和参数（流式模式下 tool_calls 是分片到达的）
		var toolCallName strings.Builder
		var toolCallArgs strings.Builder
		hasToolCall := false

		// 用于处理 DSML 工具调用流式拦截与看门狗缓存
		var contentAccumulator strings.Builder
		isBuffering := false
		isDSMLToolCall := false

		// 用于过滤 <think> ... </think> 思考链的变量
		inThinkBlock := false
		var thinkBuffer strings.Builder

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// SSE 格式：忽略空行和非 data: 前缀的行
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			// 去除 "data: " 前缀获取 JSON 数据部分
			data := strings.TrimPrefix(line, "data: ")

			// 检测流式结束标志
			if data == "[DONE]" {
				log.Println("【流式输出】收到 [DONE] 信号，流式传输完成")

				// 冲刷常规缓冲
				if isBuffering && contentAccumulator.Len() > 0 {
					ch <- StreamChunk{Token: contentAccumulator.String()}
				}

				finalChunk := StreamChunk{Done: true}
				if isDSMLToolCall {
					fullContent := contentAccumulator.String()
					if name, args, ok := ParseDSMLToolCall(fullContent); ok {
						finalChunk.ToolCall = &ToolCallInfo{
							Name:      name,
							Arguments: args,
						}
						log.Printf("【流式 DSML Tool Calling 命中工具】: %s | 参数: %s", name, args)
					} else {
						// 如果解析失败，退回作为普通文本输出给用户
						log.Printf("【流式 DSML Tool Calling】解析失败，退回常规文本")
						ch <- StreamChunk{Token: fullContent}
					}
				} else if hasToolCall {
					finalChunk.ToolCall = &ToolCallInfo{
						Name:      toolCallName.String(),
						Arguments: toolCallArgs.String(),
					}
					log.Printf("【流式 Tool Calling 命中工具】: %s | 参数: %s", toolCallName.String(), toolCallArgs.String())
				}
				ch <- finalChunk
				return
			}

			// 解析 SSE 数据块中的 delta 结构
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						ToolCalls []struct {
							Index    int `json:"index"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// 解析失败时跳过当前行，不中断整个流
				log.Printf("【流式输出】解析 SSE 数据块失败，跳过: %v", err)
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			// 处理文本内容 delta (带有 DSML 流拦截看门狗 & <think> 过滤)
			if delta.Content != "" {
				// 1. 如果在 think 块中，累积并检查结束标志
				if inThinkBlock {
					thinkBuffer.WriteString(delta.Content)
					acc := thinkBuffer.String()
					if idx := strings.Index(acc, "</think>"); idx != -1 {
						inThinkBlock = false
						delta.Content = acc[idx+8:] // 跳过 </think>
						thinkBuffer.Reset()
					} else {
						// 还在 think 中，忽略该 token
						continue
					}
				}

				// 2. 检测是否进入 think 块
				if !inThinkBlock && strings.Contains(delta.Content, "<think>") {
					idx := strings.Index(delta.Content, "<think>")
					before := delta.Content[:idx]
					after := delta.Content[idx+7:] // 跳过 <think>
					
					inThinkBlock = true
					thinkBuffer.Reset()
					thinkBuffer.WriteString(after)
					
					// 检查当前 token 是否就已经包含了 </think> 结束符
					acc := thinkBuffer.String()
					if idxEnd := strings.Index(acc, "</think>"); idxEnd != -1 {
						inThinkBlock = false
						delta.Content = before + acc[idxEnd+8:]
						thinkBuffer.Reset()
					} else {
						if before != "" {
							delta.Content = before
						} else {
							continue
						}
					}
				}

				if delta.Content != "" {
					if !isBuffering && !isDSMLToolCall {
						// 发现以 "<" 或 "<｜" 开头，开启前缀预判缓冲模式
						trimmed := strings.TrimSpace(delta.Content)
						if strings.HasPrefix(trimmed, "<") {
							isBuffering = true
						}
					}

					if isBuffering {
						contentAccumulator.WriteString(delta.Content)
						accStr := contentAccumulator.String()

						// 判定是否符合 DSML 前缀
						if strings.Contains(accStr, "<｜｜DSML｜｜tool_calls>") {
							isDSMLToolCall = true
							isBuffering = false
						} else if len(accStr) >= 40 {
							// 累积足够长还不包含特征词，判定为普通文本，冲刷并关闭缓冲
							ch <- StreamChunk{Token: accStr}
							contentAccumulator.Reset()
							isBuffering = false
						}
					} else if isDSMLToolCall {
						// 静默累积全部工具调用文本，不发给前端，也不进行 TTS 发声
						contentAccumulator.WriteString(delta.Content)
					} else {
						ch <- StreamChunk{Token: delta.Content}
					}
				}
			}

			// 处理工具调用 delta（标准流式模式下分片累积）
			if len(delta.ToolCalls) > 0 {
				hasToolCall = true
				for _, tc := range delta.ToolCalls {
					if tc.Function.Name != "" {
						toolCallName.WriteString(tc.Function.Name)
					}
					if tc.Function.Arguments != "" {
						toolCallArgs.WriteString(tc.Function.Arguments)
					}
				}
			}
		}

		// 扫描器遇到错误时通过 channel 发送错误信息
		if err := scanner.Err(); err != nil {
			log.Printf("【流式输出】读取 SSE 响应流异常: %v", err)
			ch <- StreamChunk{Error: fmt.Errorf("读取流式响应数据失败: %v", err)}
			return
		}

		// 流异常结束时的兜底处理
		if isBuffering && contentAccumulator.Len() > 0 {
			ch <- StreamChunk{Token: contentAccumulator.String()}
		}

		finalChunk := StreamChunk{Done: true}
		if isDSMLToolCall {
			fullContent := contentAccumulator.String()
			if name, args, ok := ParseDSMLToolCall(fullContent); ok {
				finalChunk.ToolCall = &ToolCallInfo{
					Name:      name,
					Arguments: args,
				}
			} else {
				ch <- StreamChunk{Token: fullContent}
			}
		} else if hasToolCall {
			finalChunk.ToolCall = &ToolCallInfo{
				Name:      toolCallName.String(),
				Arguments: toolCallArgs.String(),
			}
		}
		ch <- finalChunk
	}()

	return ch, nil
}

// GenerateStreamingReply 使用流式 SSE 模式发送给定的多轮消息历史，并逐 token 返回解析结果
func (s *NLPService) GenerateStreamingReply(messages []map[string]interface{}) (<-chan StreamChunk, error) {
	// 1. 动态从数据库中获取当前最新的模型配置参数
	var cfg models.ModelConfig
	err := database.DB.First(&cfg).Error
	if err != nil {
		return nil, fmt.Errorf("获取大模型数据库配置失败: %v", err)
	}

	// 2. 检查 Key 的有效性
	if cfg.ApiKey == "" {
		return nil, fmt.Errorf("大模型 API Key 尚未配置，请前往后台系统设置进行配置")
	}

	// 3. 端点适配优化，自动追加标准聊天路径
	apiURL := cfg.ApiURL
	if !strings.HasSuffix(apiURL, "/chat/completions") {
		apiURL = strings.TrimSuffix(apiURL, "/")
		apiURL = apiURL + "/chat/completions"
	}

	apiKey := cfg.ApiKey

	log.Printf("【流式性格重生成】使用模型 %s 开始第二轮性格响应渲染...", cfg.ModelName)

	// 构建官方标准的 OpenAI 流式请求体，不携带 tools 字段，专心文本渲染
	requestBody := map[string]interface{}{
		"model":       cfg.ModelName,
		"messages":    messages,
		"temperature": 0.3,
		"stream":      true,
	}

	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("序列化流式请求 JSON 结构失败: %v", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("构建流式 HTTP 请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 流式请求不设置超时，防止长时间生成过程被强制中断
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用大模型流式 API 网络请求失败: %v", err)
	}

	// 检查 HTTP 状态码，非 200 时直接读取错误并返回
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		var errResponse struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(bodyBytes, &errResponse)
		if errResponse.Error.Message != "" {
			return nil, fmt.Errorf("大模型服务商报错: %s", errResponse.Error.Message)
		}
		return nil, fmt.Errorf("大模型端点返回 HTTP 状态码 %d: %s", resp.StatusCode, string(bodyBytes))
	}

	ch := make(chan StreamChunk, 32)

	// 启动后台 goroutine 逐行读取 SSE 流并发送 StreamChunk
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		// 用于过滤 <think> ... </think> 思考链的变量
		inThinkBlock := false
		var thinkBuffer strings.Builder

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// SSE 格式：忽略空行和非 data: 前缀的行
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			// 去除 "data: " 前缀获取 JSON 数据部分
			data := strings.TrimPrefix(line, "data: ")

			// 检测流式结束标志
			if data == "[DONE]" {
				ch <- StreamChunk{Done: true}
				return
			}

			// 解析 SSE 数据块中的 delta 结构
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				// 解析失败时跳过当前行，不中断整个流
				log.Printf("【流式输出】解析 SSE 数据块失败，跳过: %v", err)
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			// 处理文本内容 delta (含 <think> 过滤)
			if delta.Content != "" {
				// 1. 如果在 think 块中，累积并检查结束标志
				if inThinkBlock {
					thinkBuffer.WriteString(delta.Content)
					acc := thinkBuffer.String()
					if idx := strings.Index(acc, "</think>"); idx != -1 {
						inThinkBlock = false
						delta.Content = acc[idx+8:] // 跳过 </think>
						thinkBuffer.Reset()
					} else {
						// 还在 think 中，忽略该 token
						continue
					}
				}

				// 2. 检测是否进入 think 块
				if !inThinkBlock && strings.Contains(delta.Content, "<think>") {
					idx := strings.Index(delta.Content, "<think>")
					before := delta.Content[:idx]
					after := delta.Content[idx+7:] // 跳过 <think>
					
					inThinkBlock = true
					thinkBuffer.Reset()
					thinkBuffer.WriteString(after)
					
					// 检查当前 token 是否就已经包含了 </think> 结束符
					acc := thinkBuffer.String()
					if idxEnd := strings.Index(acc, "</think>"); idxEnd != -1 {
						inThinkBlock = false
						delta.Content = before + acc[idxEnd+8:]
						thinkBuffer.Reset()
					} else {
						if before != "" {
							delta.Content = before
						} else {
							continue
						}
					}
				}

				if delta.Content != "" {
					ch <- StreamChunk{Token: delta.Content}
				}
			}
		}

		// 扫描器遇到错误时通过 channel 发送错误信息
		if err := scanner.Err(); err != nil {
			log.Printf("【流式输出】读取 SSE 响应流异常: %v", err)
			ch <- StreamChunk{Error: fmt.Errorf("读取流式响应数据失败: %v", err)}
			return
		}

		ch <- StreamChunk{Done: true}
	}()

	return ch, nil
}

// GenerateStreamingToolReply 在 WebSocket 命中工具并执行完后，自动组装多轮历史并调用流式回复
func (s *NLPService) GenerateStreamingToolReply(command string, toolName string, toolArgs string, toolResult string) (<-chan StreamChunk, error) {
	// 1. 获取配置并插值 SystemPrompt
	var cfg models.ModelConfig
	err := database.DB.First(&cfg).Error
	if err != nil {
		return nil, fmt.Errorf("获取大模型数据库配置失败: %v", err)
	}

	finalSystemPrompt := cfg.SystemPrompt
	if cfg.SystemRole != "" {
		finalSystemPrompt = strings.ReplaceAll(finalSystemPrompt, "{{.SystemRole}}", cfg.SystemRole)
		finalSystemPrompt = strings.ReplaceAll(finalSystemPrompt, "{{SystemRole}}", cfg.SystemRole)
	}
	if cfg.SystemPersonality != "" {
		finalSystemPrompt = strings.ReplaceAll(finalSystemPrompt, "{{.SystemPersonality}}", cfg.SystemPersonality)
		finalSystemPrompt = strings.ReplaceAll(finalSystemPrompt, "{{SystemPersonality}}", cfg.SystemPersonality)
	}

	// 2. 构建多轮 Messages 历史
	messages := []map[string]interface{}{
		{"role": "system", "content": finalSystemPrompt},
		{"role": "user", "content": "用户说: " + command},
		{"role": "assistant", "content": nil, "tool_calls": []map[string]interface{}{
			{
				"id":       "call_stream_1",
				"type":     "function",
				"function": map[string]interface{}{
					"name":      toolName,
					"arguments": toolArgs,
				},
			},
		}},
		{"role": "tool", "tool_call_id": "call_stream_1", "name": toolName, "content": toolResult},
	}

	// 3. 调用底层的流式回复生成器
	return s.GenerateStreamingReply(messages)
}

// ParseDSMLToolCall 解析 DeepSeek 特有的 DSML 工具调用格式
func ParseDSMLToolCall(content string) (name string, arguments string, ok bool) {
	if !strings.Contains(content, "<｜｜DSML｜｜tool_calls>") {
		return "", "", false
	}

	// 提取 tool name
	nameRx := regexp.MustCompile(`<｜｜DSML｜｜invoke\s+name="([^"]+)"`)
	nameMatch := nameRx.FindStringSubmatch(content)
	if len(nameMatch) < 2 {
		return "", "", false
	}
	name = nameMatch[1]

	// 提取 parameters (支持任意参数属性，如 type="int" 或 string="true")
	paramRx := regexp.MustCompile(`<｜｜DSML｜｜parameter\s+name="([^"]+)"[^>]*>([^<]+)</｜｜DSML｜｜parameter>`)
	matches := paramRx.FindAllStringSubmatch(content, -1)

	argMap := make(map[string]interface{})
	for _, m := range matches {
		if len(m) >= 3 {
			key := m[1]
			val := strings.TrimSpace(m[2])
			// 尝试解析 val 的类型，避免都是 string 导致 MCP 调用参数类型报错
			if isDigits(val) {
				if iv, err := strconv.Atoi(val); err == nil {
					argMap[key] = iv
					continue
				}
			}
			if val == "true" {
				argMap[key] = true
				continue
			}
			if val == "false" {
				argMap[key] = false
				continue
			}
			argMap[key] = val
		}
	}

	argBytes, err := json.Marshal(argMap)
	if err != nil {
		return name, "{}", true
	}

	return name, string(argBytes), true
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

