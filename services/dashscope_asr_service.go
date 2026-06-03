package services

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// DashScopeASRStream 管理与百炼 ASR WebSocket 接口的双工通信流
type DashScopeASRStream struct {
	conn      *websocket.Conn
	taskID    string
	writeMu   sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	onResult  func(text string, isFinal bool)
	startChan chan struct{}
	audioBuf  []int16
	audioMu   sync.Mutex
}

// NewDashScopeASRStream 实例化并初始化百炼 Paraformer ASR 二进制流
func NewDashScopeASRStream(apiKey string, onResult func(text string, isFinal bool)) (*DashScopeASRStream, error) {
	wsUrl := "wss://dashscope.aliyuncs.com/api-ws/v1/inference"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsUrl, headers)
	if err != nil {
		return nil, fmt.Errorf("dial DashScope ASR failed: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream := &DashScopeASRStream{
		conn:      conn,
		taskID:    uuid.New().String(),
		ctx:       ctx,
		cancel:    cancel,
		onResult:  onResult,
		startChan: make(chan struct{}),
	}

	// 启动后台事件监听协程
	go stream.readLoop()

	// 1. 发送 run-task 初始化指令
	err = stream.sendRunTask()
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("send run-task failed: %w", err)
	}

	// 2. 等待 task-started 信号以确保握手成功且云端 ASR 已就绪
	select {
	case <-stream.startChan:
		log.Printf("【百炼 ASR】成功建立流式识别任务, TaskID: %s", stream.taskID)
	case <-time.After(5 * time.Second):
		stream.Close()
		return nil, fmt.Errorf("wait for task-started timeout")
	}

	return stream, nil
}

func (s *DashScopeASRStream) sendRunTask() error {
	req := map[string]interface{}{
		"header": map[string]interface{}{
			"action":    "run-task",
			"task_id":   s.taskID,
			"streaming": "duplex",
		},
		"payload": map[string]interface{}{
			"task_group": "audio",
			"task":       "asr",
			"function":   "recognition",
			"model":      "paraformer-realtime-v2",
			"parameters": map[string]interface{}{
				"format":      "pcm",
				"sample_rate": 16000,
			},
			"input": map[string]interface{}{},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// SendAudio 推送 16kHz, 16-bit 单声道 Little-Endian 二进制 PCM 音频块
func (s *DashScopeASRStream) SendAudio(pcmData []byte) error {
	s.audioMu.Lock()
	n := len(pcmData) / 2
	maxSamples := 16000 * 15 // 最多缓存 15 秒音频
	if len(s.audioBuf)+n > maxSamples {
		excess := len(s.audioBuf) + n - maxSamples
		s.audioBuf = s.audioBuf[excess:]
	}
	samplesInt16 := make([]int16, n)
	for i := 0; i < n; i++ {
		samplesInt16[i] = int16(binary.LittleEndian.Uint16(pcmData[i*2 : i*2+2]))
	}
	s.audioBuf = append(s.audioBuf, samplesInt16...)
	s.audioMu.Unlock()

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, pcmData)
}

// GetAndClearAudio 获取当前句子的音频缓存并清空
func (s *DashScopeASRStream) GetAndClearAudio() []int16 {
	s.audioMu.Lock()
	defer s.audioMu.Unlock()
	buf := s.audioBuf
	s.audioBuf = nil
	return buf
}

// Close 发送 finish-task 并优雅关闭 WebSocket 连接
func (s *DashScopeASRStream) Close() {
	s.cancel()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.conn != nil {
		finishReq := map[string]interface{}{
			"header": map[string]interface{}{
				"action":  "finish-task",
				"task_id": s.taskID,
			},
		}
		if data, err := json.Marshal(finishReq); err == nil {
			_ = s.conn.WriteMessage(websocket.TextMessage, data)
		}
		_ = s.conn.Close()
	}
}

func (s *DashScopeASRStream) readLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			msgType, data, err := s.conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage {
				var resp struct {
					Header struct {
						Event  string `json:"event"`
						TaskID string `json:"task_id"`
					} `json:"header"`
					Payload struct {
						Output struct {
							Sentence struct {
								Text        string `json:"text"`
								SentenceEnd bool   `json:"sentence_end"`
							} `json:"sentence"`
						} `json:"output"`
					} `json:"payload"`
				}

				if err := json.Unmarshal(data, &resp); err == nil {
					switch resp.Header.Event {
					case "task-started":
						select {
						case <-s.startChan:
						default:
							close(s.startChan)
						}
					case "result-generated":
						text := resp.Payload.Output.Sentence.Text
						isFinal := resp.Payload.Output.Sentence.SentenceEnd
						if text != "" {
							s.onResult(text, isFinal)
						}
					case "task-finished", "task-failed":
						return
					}
				}
			}
		}
	}
}
