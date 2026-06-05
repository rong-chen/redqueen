package controllers

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"RedQueenSystem/database"
	"RedQueenSystem/models"
	"RedQueenSystem/services"
	"RedQueenSystem/utils"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gopkg.in/hraban/opus.v2"
)

// ---------------------------------------------------------------------------
// WebSocket 升级器配置
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ---------------------------------------------------------------------------
// WebSocket 消息协议
// ---------------------------------------------------------------------------

// WSMessage 服务端推送给客户端的 JSON 消息结构
type WSMessage struct {
	Type    string `json:"type"`              // 消息类型: partial, final, wake, result, sleep, error
	Text    string `json:"text,omitempty"`     // 识别出的文字
	Intent  string `json:"intent,omitempty"`   // NLP 解析出的意图
	Status  string `json:"status,omitempty"`   // 指令执行状态: success, failed
	Message string `json:"message,omitempty"`  // 人可读的提示信息
}

// WSClientMessage 客户端发送给服务端的 JSON 消息结构
type WSClientMessage struct {
	Type string `json:"type"`           // 消息类型: command, interrupt, ping
	Text string `json:"text,omitempty"` // 指令文本内容
}

// VoiceprintAuthState 记录每个 WebSocket 连接的声纹鉴权状态
type VoiceprintAuthState struct {
	IsAuthenticated  bool
	AuthBuffer       []int16
	MasterVoiceprint []float32
	HasMaster        bool
}

// calculateVolume 计算一帧 PCM 16-bit 音频数据的平均绝对振幅（音量）
func calculateVolume(pcmData []byte) float64 {
	if len(pcmData) < 2 {
		return 0
	}
	var sum float64
	count := 0
	for i := 0; i < len(pcmData)-1; i += 2 {
		val := int16(pcmData[i]) | (int16(pcmData[i+1]) << 8)
		absVal := val
		if val < 0 {
			if val == -32768 {
				absVal = 32767
			} else {
				absVal = -val
			}
		}
		sum += float64(absVal)
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// ---------------------------------------------------------------------------
// VoiceWSController 实时语音 WebSocket 控制器
// ---------------------------------------------------------------------------

// VoiceWSController 负责处理 WebSocket 语音流连接
type VoiceWSController struct {
	wakeWord       string
	sessionTimeout time.Duration
	activeSessions sync.Map // Map[string]*services.QwenOmniSession (RoomID -> QwenOmniSession)
}

// NewVoiceWSController 实例化 WebSocket 语音控制器
func NewVoiceWSController(wakeWord string, sessionTimeoutSec int) *VoiceWSController {
	timeout := time.Duration(sessionTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &VoiceWSController{
		wakeWord:       wakeWord,
		sessionTimeout: timeout,
	}
}

// interruptPlayback 强行中断指定房间的活跃播放并通知客户端
func (ctrl *VoiceWSController) interruptPlayback(conn *websocket.Conn, writeMu *sync.Mutex, roomID string) {
	// Cancel Qwen-Omni current active generating response
	if val, ok := ctrl.activeSessions.Load(roomID); ok {
		if omniSess, ok := val.(*services.QwenOmniSession); ok {
			_ = omniSess.CancelResponse()
			log.Printf("【打断机制】成功发送取消指令给 Qwen-Omni 会话 [%s]", roomID)
		}
	}
	// 通知客户端播放已被打断
	ctrl.sendMessage(conn, writeMu, WSMessage{
		Type:    "interrupt",
		Message: "playback_interrupted",
	})
}

// HandleWebSocket 处理 GET /api/voice/ws —— 实时语音识别 WebSocket 端点
func (ctrl *VoiceWSController) HandleWebSocket(c *gin.Context) {
	// 1. 升级 HTTP 连接为 WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("【WebSocket】连接升级失败: %v", err)
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex

	room := c.DefaultQuery("room", "")
	if room == "" {
		room = "client_" + time.Now().Format("150405.000")
	}

	codec := c.DefaultQuery("codec", "pcm")
	log.Printf("【WebSocket】新的语音连接已建立, 房间: %s, 编码格式: %s", room, codec)

	// 初始化 Opus 编解码器（仅在选用 opus 时）
	var opusDecoder *opus.Decoder
	var opusEncoder *opus.Encoder
	if codec == "opus" {
		dec, err := opus.NewDecoder(16000, 1)
		if err != nil {
			log.Printf("【WebSocket】初始化 Opus 解码器失败: %v", err)
			ctrl.sendMessage(conn, &writeMu, WSMessage{Type: "error", Message: "Opus decoder init failed"})
			return
		}
		opusDecoder = dec

		enc, err := opus.NewEncoder(16000, 1, opus.AppVoIP)
		if err != nil {
			log.Printf("【WebSocket】初始化 Opus 编码器失败: %v", err)
			ctrl.sendMessage(conn, &writeMu, WSMessage{Type: "error", Message: "Opus encoder init failed"})
			return
		}
		_ = enc.SetComplexity(1)
		opusEncoder = enc
	}

	// 创建本地会话状态管理
	session := services.NewSession(ctrl.wakeWord, ctrl.sessionTimeout, room)
	defer ctrl.stopQwenOmniSession(room)

	var authState VoiceprintAuthState
	// 从数据库加载 MasterVoiceprint
	var admin models.User
	if err := database.DB.Where("role = ?", "admin").First(&admin).Error; err == nil && admin.MasterVoiceprint != "" {
		var master []float32
		if err := json.Unmarshal([]byte(admin.MasterVoiceprint), &master); err == nil && len(master) > 0 {
			authState.MasterVoiceprint = master
			authState.HasMaster = true
		}
	}

	// 如果没有录入过声纹，默认放行
	if !authState.HasMaster {
		authState.IsAuthenticated = true
		log.Println("【Voiceprint】未检测到 MasterVoiceprint，默认放行连接")
	}

	// 发送连接就绪消息
	ctrl.sendMessage(conn, &writeMu, WSMessage{
		Type:    "ready",
		Message: "语音连接就绪，安全通信链路已建立",
	})

	// 主循环：持续接收并处理音频数据
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("【WebSocket】连接异常关闭: %v", err)
			} else {
				log.Println("【WebSocket】连接正常关闭")
			}
			break
		}

		switch messageType {
		case websocket.BinaryMessage:
			ctrl.processAudioFrame(conn, &writeMu, session, &authState, data, codec, opusDecoder, opusEncoder)

		case websocket.TextMessage:
			ctrl.processControlMessage(conn, &writeMu, session, data, codec, opusEncoder)
		}
	}

	log.Println("【WebSocket】语音连接已断开")
}

// processAudioFrame 处理一帧 PCM 或 Opus 音频数据并发送到 Qwen-Omni Realtime API
func (ctrl *VoiceWSController) processAudioFrame(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	session *services.Session,
	authState *VoiceprintAuthState,
	pcmData []byte,
	codec string,
	opusDecoder *opus.Decoder,
	opusEncoder *opus.Encoder,
) {
	var pcm16Data []byte

	// 1. 解码音频到 16kHz PCM
	if codec == "opus" {
		if opusDecoder != nil {
			pcmBuf := make([]int16, 1024)
			n, err := opusDecoder.Decode(pcmData, pcmBuf)
			if err == nil && n > 0 {
				pcm16Data = make([]byte, n*2)
				for i, v := range pcmBuf[:n] {
					binary.LittleEndian.PutUint16(pcm16Data[i*2:], uint16(v))
				}
			}
		}
	} else {
		pcm16Data = pcmData
	}

	if len(pcm16Data) == 0 {
		return
	}

	// 转为 int16 样本
	samples := make([]int16, len(pcm16Data)/2)
	for j := 0; j < len(samples); j++ {
		samples[j] = int16(pcm16Data[j*2]) | (int16(pcm16Data[j*2+1]) << 8)
	}

	// 1.5 声纹网关拦截：未鉴权时缓存并校验
	if !authState.IsAuthenticated && authState.HasMaster {
		authState.AuthBuffer = append(authState.AuthBuffer, samples...)
		// 累积满 1.5 秒音频 (约 24000 个采样)
		if len(authState.AuthBuffer) >= 24000 {
			vpSvc := services.GetVoiceprint()
			if vpSvc != nil {
				trimmed := utils.TrimSilence(authState.AuthBuffer)
				if len(trimmed) > 8000 { // 至少包含 0.5 秒的有效人声
					emb, err := vpSvc.ExtractEmbedding(trimmed)
					if err == nil {
						sim := vpSvc.VerifySpeaker(emb, authState.MasterVoiceprint)
						log.Printf("【Voiceprint Gatekeeper】声纹相似度检测完毕: %f", sim)
						if sim > 0.55 { // 阈值 0.55
							authState.IsAuthenticated = true
							ctrl.sendMessage(conn, writeMu, WSMessage{
								Type: "auth_success", Message: "声纹验证通过，已授权接入红皇后系统！",
							})
						} else {
							ctrl.sendMessage(conn, writeMu, WSMessage{
								Type: "auth_failed", Message: "声纹校验未通过，识别为非授权人员，拒绝接入",
							})
							_ = conn.Close()
							return
						}
					}
				}
			}
			// 如果提取失败、人声不够或者还在等待
			if !authState.IsAuthenticated {
				// 滑动窗口：丢弃最旧的 0.5 秒数据，继续等待新音频
				if len(authState.AuthBuffer) > 8000 {
					authState.AuthBuffer = authState.AuthBuffer[8000:]
				}
				return
			}
		} else {
			// 音频长度不够 1.5 秒，继续缓冲
			return
		}
	}

	// 将当前要发送的数据整理好（如果是刚刚通过鉴权，则发送累积的 Buffer；否则只发送当前帧）
	var dataToSend []byte
	if len(authState.AuthBuffer) > 0 {
		dataToSend = make([]byte, len(authState.AuthBuffer)*2)
		for i, s := range authState.AuthBuffer {
			dataToSend[i*2] = byte(s)
			dataToSend[i*2+1] = byte(s >> 8)
		}
		authState.AuthBuffer = nil // 发送完毕后清空
	} else {
		dataToSend = pcm16Data
	}

	// 记录并通知音量用于前台波形绘制
	vol := calculateVolume(pcm16Data)
	session.UpdateVolume(vol)

	// 2. 检查或新建 Qwen-Omni 语音会话
	omniSessionVal, ok := ctrl.activeSessions.Load(session.RoomID)
	var omniSession *services.QwenOmniSession
	if !ok {
		// 载入大模型系统级提示词配置
		var cfg models.ModelConfig
		var systemPrompt string
		var voice string
		if err := database.DB.First(&cfg).Error; err == nil {
			systemPrompt = cfg.SystemPrompt
			systemPrompt = strings.ReplaceAll(systemPrompt, "{{.SystemRole}}", cfg.SystemRole)
			systemPrompt = strings.ReplaceAll(systemPrompt, "{{.SystemPersonality}}", cfg.SystemPersonality)
			voice = cfg.Voice
		} else {
			systemPrompt = "你是一个智能硬件助手红皇后，说话简短冷酷。"
			voice = "Tina"
		}

		var pcmBuf []byte
		var textBuf strings.Builder

		var err error
		omniSession, err = services.NewQwenOmniSession(
			systemPrompt,
			voice,
			func(audioBytes []byte) {
				// 收到音频流回调：发送至客户端
				if codec == "opus" && opusEncoder != nil {
					pcmBuf = append(pcmBuf, audioBytes...)
					const frameBytes = 640
					for len(pcmBuf) >= frameBytes {
						frame := pcmBuf[:frameBytes]
						pcmBuf = pcmBuf[frameBytes:]
						samples := make([]int16, 320)
						for i := 0; i < 320; i++ {
							samples[i] = int16(binary.LittleEndian.Uint16(frame[i*2 : i*2+2]))
						}
						opusBuf := make([]byte, 1024)
						n, err := opusEncoder.Encode(samples, opusBuf)
						if err == nil && n > 0 {
							writeMu.Lock()
							_ = conn.WriteMessage(websocket.BinaryMessage, opusBuf[:n])
							writeMu.Unlock()
						}
					}
				} else {
					writeMu.Lock()
					_ = conn.WriteMessage(websocket.BinaryMessage, audioBytes)
					writeMu.Unlock()
				}
			},
			func(text string, isFinal bool) {
				// 收到文字转写/Token 回调
				if isFinal {
					fullText := textBuf.String()
					textBuf.Reset()

					ctrl.sendMessage(conn, writeMu, WSMessage{
						Type: "final",
						Text: fullText,
					})

					// 记录日志到数据库
					voiceSvc := services.NewVoiceService()
					_, _ = voiceSvc.CreateVoiceCommand("", fullText, 1.0)

					ctrl.sendMessage(conn, writeMu, WSMessage{
						Type: "result", Intent: "conversation", Status: "success", Message: fullText,
					})
				} else {
					textBuf.WriteString(text)
					ctrl.sendMessage(conn, writeMu, WSMessage{
						Type:    "stream_token",
						Message: text,
					})
				}
			},
			func() {
				// 收到 VAD 打断回调
				ctrl.sendMessage(conn, writeMu, WSMessage{
					Type:    "interrupt",
					Message: "playback_interrupted",
				})
			},
		)

		if err != nil {
			log.Printf("【Qwen-Omni】初始化流式对话出错: %v", err)
			return
		}

		ctrl.activeSessions.Store(session.RoomID, omniSession)
	} else {
		omniSession = omniSessionVal.(*services.QwenOmniSession)
	}

	// 3. 将音频帧推入 Qwen-Omni
	_ = omniSession.SendAudioChunk(dataToSend)
}

// processControlMessage 处理客户端发送的文本控制或打断指令
func (ctrl *VoiceWSController) processControlMessage(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	session *services.Session,
	data []byte,
	codec string,
	opusEncoder *opus.Encoder,
) {
	var clientMsg WSClientMessage
	if err := json.Unmarshal(data, &clientMsg); err == nil && clientMsg.Type != "" {
		switch clientMsg.Type {
		case "command":
			if clientMsg.Text != "" {
				log.Printf("【收到客户端文本指令】: %s", clientMsg.Text)
				ctrl.sendMessage(conn, writeMu, WSMessage{
					Type: "final",
					Text: clientMsg.Text,
				})

				// 尝试将文本命令发送给现有的 Qwen-Omni 语音会话
				if val, ok := ctrl.activeSessions.Load(session.RoomID); ok {
					if omniSess, ok := val.(*services.QwenOmniSession); ok {
						_ = omniSess.SendTextCommand(clientMsg.Text)
					}
				}
			}
		case "speaking_status":
			isSpeaking := clientMsg.Text == "true"
			log.Printf("【WebSocket】收到前端朗读状态同步 speaking_status: %v", isSpeaking)
			session.SetSpeaking(isSpeaking)
		case "interrupt":
			log.Printf("【打断机制】收到客户端 JSON 打断指令，停止房间 [%s] 的当前播放", session.RoomID)
			ctrl.interruptPlayback(conn, writeMu, session.RoomID)
		case "ping":
			ctrl.sendMessage(conn, writeMu, WSMessage{
				Type:    "pong",
				Message: "alive",
			})
		}
		return
	}

	cmd := string(data)
	switch cmd {
	case "ping":
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type:    "pong",
			Message: "alive",
		})
	case "interrupt":
		log.Printf("【打断机制】收到客户端普通文本打断指令，停止房间 [%s] 的当前播放", session.RoomID)
		ctrl.interruptPlayback(conn, writeMu, session.RoomID)
	}
}

// sendMessage 线程安全地向 WebSocket 客户端发送 JSON 消息
func (ctrl *VoiceWSController) sendMessage(conn *websocket.Conn, writeMu *sync.Mutex, msg WSMessage) {
	writeMu.Lock()
	defer writeMu.Unlock()

	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("【WebSocket】消息发送失败: %v", err)
	}
}

// calculateVolumeSamples 计算 int16 采样切片的平均绝对振幅
func calculateVolumeSamples(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, val := range samples {
		absVal := val
		if val < 0 {
			if val == -32768 {
				absVal = 32767
			} else {
				absVal = -val
			}
		}
		sum += float64(absVal)
	}
	return sum / float64(len(samples))
}

// stopQwenOmniSession 关闭并清理指定房间的 Qwen-Omni 语音会话
func (ctrl *VoiceWSController) stopQwenOmniSession(roomID string) {
	if val, ok := ctrl.activeSessions.Load(roomID); ok {
		if stream, ok := val.(*services.QwenOmniSession); ok {
			stream.Close()
			log.Printf("【Qwen-Omni】已成功关闭并清理房间 [%s] 的流式对话会话", roomID)
		}
		ctrl.activeSessions.Delete(roomID)
	}
}
