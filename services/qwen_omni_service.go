package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// QwenOmniSession represents a single real-time end-to-end speech session with Qwen-Omni
type QwenOmniSession struct {
	conn              *websocket.Conn
	writeMu           sync.Mutex
	ctx               context.Context
	cancel            context.CancelFunc
	onAudioDelta      func([]byte)
	onTranscriptDelta func(string, bool) // text, isFinal
	onSpeechStarted   func()             // VAD interruption signal
	apiKey            string
	model             string
	systemPrompt      string
	voice             string
	sessionID         string
}

// NewQwenOmniSession initializes a connection to Qwen-Omni Realtime WebSocket
func NewQwenOmniSession(
	systemPrompt string,
	voice string,
	onAudioDelta func([]byte),
	onTranscriptDelta func(string, bool),
	onSpeechStarted func(),
) (*QwenOmniSession, error) {
	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		apiKey = "sk-41cd73b8bae44957aa31542e03f1521e"
	}

	model := "qwen3.5-omni-plus-realtime"

	wsURL := fmt.Sprintf("wss://dashscope.aliyuncs.com/api-ws/v1/realtime?model=%s", model)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("dial Qwen-Omni Realtime WebSocket failed: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	session := &QwenOmniSession{
		conn:              conn,
		ctx:               ctx,
		cancel:            cancel,
		onAudioDelta:      onAudioDelta,
		onTranscriptDelta: onTranscriptDelta,
		onSpeechStarted:   onSpeechStarted,
		apiKey:            apiKey,
		model:             model,
		systemPrompt:      systemPrompt,
		voice:             voice,
		sessionID:         uuid.New().String(),
	}

	// Start reading server events
	go session.readLoop()

	// Initialize the session configurations
	if err := session.sendSessionUpdate(); err != nil {
		session.Close()
		return nil, fmt.Errorf("initial session.update failed: %w", err)
	}

	log.Printf("【Qwen-Omni】已成功创建实时语音会话, SessionID: %s, 音色: %s", session.sessionID, voice)
	return session, nil
}

// sendSessionUpdate configures Qwen-Omni modalities, voice, prompt, and tools
func (s *QwenOmniSession) sendSessionUpdate() error {
	// Retrieve MCP tools currently registered and online
	_, openAITools, _ := GetExternalMCPTools()

	// Build tools payload if available
	var tools interface{}
	if len(openAITools) > 0 {
		tools = openAITools
	}

	// Prepare instructions/prompts
	instructions := s.systemPrompt
	if instructions == "" {
		instructions = "你是一个智能硬件助手红皇后，说话简短冷酷。"
	}

	voiceName := s.voice
	if voiceName == "" {
		voiceName = "Tina"
	}

	updatePayload := map[string]interface{}{
		"type": "session.update",
		"session": map[string]interface{}{
			"modalities":          []string{"text", "audio"},
			"voice":               voiceName, // Custom voice
			"instructions":        instructions,
			"input_audio_format":  "pcm16",
			"output_audio_format": "pcm16",
			"turn_detection": map[string]interface{}{
				"type":                "server_vad",
				"threshold":           0.5,
				"prefix_padding_ms":   300,
				"silence_duration_ms": 600,
			},
		},
	}

	if tools != nil {
		updatePayload["session"].(map[string]interface{})["tools"] = tools
	}

	data, err := json.Marshal(updatePayload)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// SendAudioChunk base64-encodes raw 16kHz PCM data and streams it to the model
func (s *QwenOmniSession) SendAudioChunk(pcmData []byte) error {
	if len(pcmData) == 0 {
		return nil
	}

	encoded := base64.StdEncoding.EncodeToString(pcmData)
	payload := map[string]interface{}{
		"type":  "input_audio_buffer.append",
		"audio": encoded,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// Close gracefully closes the WebSocket connection to DashScope
func (s *QwenOmniSession) Close() {
	s.cancel()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

// readLoop continuously reads events from Qwen-Omni Realtime connection
func (s *QwenOmniSession) readLoop() {
	defer s.Close()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			msgType, data, err := s.conn.ReadMessage()
			if err != nil {
				log.Printf("【Qwen-Omni】读取服务端事件出错: %v", err)
				return
			}

			if msgType == websocket.TextMessage {
				s.handleServerEvent(data)
			}
		}
	}
}

// handleServerEvent processes incoming JSON messages from Qwen-Omni
func (s *QwenOmniSession) handleServerEvent(data []byte) {
	var generic struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &generic); err != nil {
		return
	}

	switch generic.Type {
	case "session.updated":
		log.Println("【Qwen-Omni】会话配置更新成功")

	case "input_audio_buffer.speech_started":
		log.Println("【Qwen-Omni】检测到用户说话起音，触发打断回调...")
		if s.onSpeechStarted != nil {
			s.onSpeechStarted()
		}

	case "response.audio.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(data, &ev); err == nil && ev.Delta != "" {
			audioBytes, err := base64.StdEncoding.DecodeString(ev.Delta)
			if err == nil && s.onAudioDelta != nil {
				s.onAudioDelta(audioBytes)
			}
		}

	case "response.audio_transcript.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(data, &ev); err == nil && ev.Delta != "" {
			if s.onTranscriptDelta != nil {
				s.onTranscriptDelta(ev.Delta, false)
			}
		}

	case "response.done":
		log.Println("【Qwen-Omni】模型回复生成完毕")
		if s.onTranscriptDelta != nil {
			s.onTranscriptDelta("", true) // Signal final
		}

	case "response.function_call_arguments.done":
		var ev struct {
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := json.Unmarshal(data, &ev); err == nil && ev.Name != "" {
			log.Printf("【Qwen-Omni】接收到工具调用请求: %s | 参数: %s", ev.Name, ev.Arguments)
			go s.handleToolCall(ev.CallID, ev.Name, ev.Arguments)
		}

	case "error":
		var ev struct {
			Error map[string]interface{} `json:"error"`
		}
		_ = json.Unmarshal(data, &ev)
		log.Printf("【Qwen-Omni】收到服务端报错事件: %v", ev.Error)
	}
}

// handleToolCall executes the corresponding MCP tool and feeds the result back to Qwen-Omni
func (s *QwenOmniSession) handleToolCall(callID, name, arguments string) {
	_, _, toolToServerMap := GetExternalMCPTools()
	srv, exists := toolToServerMap[name]

	var resultText string
	var err error

	if !exists {
		resultText = fmt.Sprintf("Error: Cannot find online MCP server for tool [%s]", name)
	} else {
		resultText, err = CallExternalMCPTool(srv, name, arguments)
		if err != nil {
			resultText = fmt.Sprintf("Error executing tool: %v", err)
		}
	}

	log.Printf("【Qwen-Omni】工具 [%s] 执行结果: %s", name, resultText)

	// Send tool result event back to Qwen-Omni
	toolResultPayload := map[string]interface{}{
		"type": "conversation.item.create",
		"item": map[string]interface{}{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  resultText,
		},
	}

	data, err := json.Marshal(toolResultPayload)
	if err == nil {
		s.writeMu.Lock()
		_ = s.conn.WriteMessage(websocket.TextMessage, data)
		s.writeMu.Unlock()
	}

	// Trigger response generation after tool result submission
	responseCreatePayload := map[string]interface{}{
		"type": "response.create",
	}
	data2, err2 := json.Marshal(responseCreatePayload)
	if err2 == nil {
		s.writeMu.Lock()
		_ = s.conn.WriteMessage(websocket.TextMessage, data2)
		s.writeMu.Unlock()
	}
}

// SendTextCommand sends a text instruction to the model and triggers response generation
func (s *QwenOmniSession) SendTextCommand(text string) error {
	if text == "" {
		return nil
	}

	// 1. Create a user message item in conversation
	itemPayload := map[string]interface{}{
		"type": "conversation.item.create",
		"item": map[string]interface{}{
			"type": "message",
			"role": "user",
			"content": []map[string]interface{}{
				{
					"type": "input_text",
					"text": text,
				},
			},
		},
	}
	data, err := json.Marshal(itemPayload)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	err = s.conn.WriteMessage(websocket.TextMessage, data)
	s.writeMu.Unlock()
	if err != nil {
		return err
	}

	// 2. Trigger response generation
	responsePayload := map[string]interface{}{
		"type": "response.create",
	}
	data2, err2 := json.Marshal(responsePayload)
	if err2 != nil {
		return err2
	}

	s.writeMu.Lock()
	err = s.conn.WriteMessage(websocket.TextMessage, data2)
	s.writeMu.Unlock()
	return err
}

// CancelResponse cancels the current active model generation response
func (s *QwenOmniSession) CancelResponse() error {
	payload := map[string]interface{}{
		"type": "response.cancel",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	err = s.conn.WriteMessage(websocket.TextMessage, data)
	s.writeMu.Unlock()
	return err
}
