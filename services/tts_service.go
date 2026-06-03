package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// 阿里云百炼 (Model Studio) CosyVoice 常量定义
// ---------------------------------------------------------------------------

const (
	dashscopeWSEndpoint = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"
	DefaultTTSVoice     = "longxiaoxia" // 默认使用百炼 CosyVoice 女声 "龙小夏"
)

// ---------------------------------------------------------------------------
// WebSocket 请求与响应结构体
// ---------------------------------------------------------------------------

type WSHeader struct {
	Action    string `json:"action,omitempty"`    // 请求动作, 如 "run-task", "continue-task", "finish-task"
	Event     string `json:"event,omitempty"`     // 响应事件, 如 "task-started", "result-generated", "task-finished", "task-failed"
	TaskId    string `json:"task_id"`
	Streaming string `json:"streaming,omitempty"` // "duplex"
}

type WSRequest struct {
	Header  WSHeader  `json:"header"`
	Payload WSPayload `json:"payload"`
}

type WSPayload struct {
	TaskGroup  string      `json:"task_group,omitempty"`
	Task       string      `json:"task,omitempty"`
	Function   string      `json:"function,omitempty"`
	Model      string      `json:"model,omitempty"`
	Parameters interface{} `json:"parameters,omitempty"`
	Input      interface{} `json:"input,omitempty"`
}

type WSContinueRequest struct {
	Header  WSHeader          `json:"header"`
	Payload WSContinuePayload `json:"payload"`
}

type WSContinuePayload struct {
	Input WSContinueInput `json:"input"`
}

type WSContinueInput struct {
	Text string `json:"text"`
}

type WSResponse struct {
	Header WSHeader `json:"header"`
}

// ---------------------------------------------------------------------------
// TTSService 文本转语音合成服务
// ---------------------------------------------------------------------------

// TTSService 基于阿里云百炼 CosyVoice 语音合成服务
// 输出格式: PCM 16kHz 16-bit 单声道
type TTSService struct {
	Voice  string // 音色名称，默认 longxiaoxia
	APIKey string // 百炼 API 密钥
}

// NewTTSService 创建 TTS 服务实例
func NewTTSService() *TTSService {
	// 优先从环境变量加载 API Key，如无则使用默认 Key
	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		apiKey = "sk-41cd73b8bae44957aa31542e03f1521e"
	}
	return &TTSService{
		Voice:  DefaultTTSVoice,
		APIKey: apiKey,
	}
}

// Synthesize 将文本合成为 PCM 音频数据 (16kHz, 16-bit, mono, little-endian)
func (t *TTSService) Synthesize(text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+t.APIKey)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(dashscopeWSEndpoint, headers)
	if err != nil {
		return nil, fmt.Errorf("百炼 TTS WebSocket 拨号失败: %v", err)
	}
	defer conn.Close()

	// 生成随机任务 ID
	taskID := uuid.New().String()

	// 1. 发送 run-task 请求以配置引擎
	runTaskReq := WSRequest{
		Header: WSHeader{
			Action:    "run-task",
			TaskId:    taskID,
			Streaming: "duplex",
		},
		Payload: WSPayload{
			TaskGroup: "audio",
			Task:      "tts",
			Function:  "SpeechSynthesizer",
			Model:     "cosyvoice-v1",
			Parameters: map[string]interface{}{
				"text_type":   "PlainText",
				"voice":       t.Voice,
				"format":      "pcm",
				"sample_rate": 16000,
			},
			Input: map[string]interface{}{},
		},
	}

	runTaskJSON, err := json.Marshal(runTaskReq)
	if err != nil {
		return nil, fmt.Errorf("序列化 run-task 失败: %v", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, runTaskJSON)
	if err != nil {
		return nil, fmt.Errorf("发送 run-task 失败: %v", err)
	}

	// 2. 等待 task-started 信号
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("读取 task-started 失败: %v", err)
		}
		if msgType == websocket.TextMessage {
			var resp WSResponse
			if err := json.Unmarshal(data, &resp); err == nil {
				if resp.Header.Event == "task-started" {
					break
				}
				if resp.Header.Event == "task-failed" {
					return nil, fmt.Errorf("任务启动失败: %s", string(data))
				}
			}
		}
	}

	// 3. 发送 continue-task 请求提供文本
	continueReq := WSContinueRequest{
		Header: WSHeader{
			Action:    "continue-task",
			TaskId:    taskID,
			Streaming: "duplex",
		},
		Payload: WSContinuePayload{
			Input: WSContinueInput{
				Text: text,
			},
		},
	}
	continueJSON, err := json.Marshal(continueReq)
	if err != nil {
		return nil, fmt.Errorf("序列化 continue-task 失败: %v", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, continueJSON)
	if err != nil {
		return nil, fmt.Errorf("发送 continue-task 失败: %v", err)
	}

	// 4. 发送 finish-task 标识结束文本流
	finishReq := WSContinueRequest{
		Header: WSHeader{
			Action: "finish-task",
			TaskId: taskID,
		},
	}
	finishJSON, err := json.Marshal(finishReq)
	if err != nil {
		return nil, fmt.Errorf("序列化 finish-task 失败: %v", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, finishJSON)
	if err != nil {
		return nil, fmt.Errorf("发送 finish-task 失败: %v", err)
	}

	// 5. 循环读取音频数据与完成状态
	var audioBuffer bytes.Buffer
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("读取音频数据失败: %v", err)
		}

		if msgType == websocket.BinaryMessage {
			audioBuffer.Write(data)
		} else if msgType == websocket.TextMessage {
			var resp WSResponse
			if err := json.Unmarshal(data, &resp); err == nil {
				if resp.Header.Event == "task-finished" {
					break
				}
				if resp.Header.Event == "task-failed" {
					return nil, fmt.Errorf("语音合成过程中任务失败: %s", string(data))
				}
			}
		}
	}

	pcmBytes := audioBuffer.Bytes()
	if len(pcmBytes) > 0 {
		durationSec := float64(len(pcmBytes)) / (16000.0 * 2.0)
		log.Printf("【百炼 TTS】语音合成完成, 音色: %s, 音频大小: %d 字节 (%.1f 秒)",
			t.Voice, len(pcmBytes), durationSec)
	}

	return pcmBytes, nil
}
