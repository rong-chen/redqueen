package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	IsExternal        bool    `json:"is_external"` // 是否为外部注册的 MCP 工具调用
	ExternalToolName  string  `json:"external_tool_name"`
	ExternalArguments string  `json:"external_arguments"`
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
		"messages": []map[string]string{
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

// ParseIntent 使用标准 OpenAI Tool Calling 接口进行语音动作提取
func (s *NLPService) ParseIntent(transcript string) (ParseResult, error) {
	requestBody, apiURL, apiKey, err := s.prepareRequest(transcript)
	if err != nil {
		return ParseResult{}, err
	}

	jsonBytes, err := json.Marshal(requestBody)
	if err != nil {
		return ParseResult{}, fmt.Errorf("序列化请求 JSON 结构失败: %v", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return ParseResult{}, fmt.Errorf("构建 HTTP 请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second} // 加长超时时间至 30 秒，防止大模型高并发排队或网络波动导致超时
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
		// 提取出错响应，尝试获取 JSON 中的错误信息
		var errResponse struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(bodyBytes, &errResponse)
		if errResponse.Error.Message != "" {
			return ParseResult{}, fmt.Errorf("大模型服务商报错: %s", errResponse.Error.Message)
		}
		return ParseResult{}, fmt.Errorf("大模型端点返回 HTTP 状态码 %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// 6. 解析标准 OpenAI/DeepSeek 兼容 of Tool-Calls 及 Content 返回结构
	var apiResult struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"` // 普通聊天/未匹配到工具时的文本回复
				ToolCalls []struct {
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

	result := ParseResult{
		Intent:     "conversation",
		Confidence: 0.98,
		ReplyText:  strings.TrimSpace(message.Content),
	}

	// 7. 处理大模型决策命中的工具
	if len(message.ToolCalls) > 0 {
		toolCall := message.ToolCalls[0]
		log.Printf("【OpenAI Tool Calling 命中工具】: %s | 参数: %s", toolCall.Function.Name, toolCall.Function.Arguments)

		// 8. 命中外部手动注册的 MCP 服务工具，封装返回并做标记，待后端服务层进行转发执行
		result.IsExternal = true
		result.ExternalToolName = toolCall.Function.Name
		result.ExternalArguments = toolCall.Function.Arguments
		result.Intent = "external_mcp_call"
		result.ReplyText = fmt.Sprintf("正在通过外部 MCP 协议执行工具 [%s]...", toolCall.Function.Name)
	}

	// 如果大模型既没有选择工具，也没有产生普通的 content 聊天文本，我们自动补充默认口语化文本并归类为 unknown
	if result.Intent == "conversation" && result.ReplyText == "" {
		result.Intent = "unknown"
		result.ReplyText = "你好，我是红皇后，目前没有匹配到可用的控制工具或意图。"
	}

	return result, nil
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
				finalChunk := StreamChunk{Done: true}
				if hasToolCall {
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

			// 处理文本内容 delta
			if delta.Content != "" {
				ch <- StreamChunk{Token: delta.Content}
			}

			// 处理工具调用 delta（流式模式下分片累积）
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

		// 如果流正常结束但未收到 [DONE]，仍然发送完成信号
		finalChunk := StreamChunk{Done: true}
		if hasToolCall {
			finalChunk.ToolCall = &ToolCallInfo{
				Name:      toolCallName.String(),
				Arguments: toolCallArgs.String(),
			}
		}
		ch <- finalChunk
	}()

	return ch, nil
}
