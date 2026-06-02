package controllers

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"RedQueenSystem/services"

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
	// 允许所有来源连接（开发阶段），生产环境应限制
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

// ---------------------------------------------------------------------------
// 多设备协同唤醒仲裁器定义与实现
// ---------------------------------------------------------------------------

// WakeEvent 表示一个设备的唤醒事件，用于仲裁
type WakeEvent struct {
	RoomID      string
	Volume      float64
	Action      services.SessionAction
	Conn        *websocket.Conn
	WriteMu     *sync.Mutex
	Session     *services.Session
	Timestamp   time.Time
	Codec       string
	OpusEncoder *opus.Encoder
}

// VoiceArbitrator 负责处理多台设备同时被唤醒时的冲突消解
type VoiceArbitrator struct {
	mu          sync.Mutex
	events      []*WakeEvent
	arbitrating bool
}

// Submit 提交一个唤醒事件，如果在 300ms 窗口内有多个唤醒事件，将进行音量比较，只保留最响的那一个
func (a *VoiceArbitrator) Submit(ctrl *VoiceWSController, event *WakeEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.events = append(a.events, event)
	log.Printf("【仲裁器】收到房间 [%s] 的唤醒事件请求, 当前平滑音量: %.2f", event.RoomID, event.Volume)

	// 如果当前没有在仲裁窗口内，启动一个 300ms 的窗口
	if !a.arbitrating {
		a.arbitrating = true
		time.AfterFunc(300*time.Millisecond, func() {
			a.Resolve(ctrl)
		})
	}
}

// Resolve 在 300ms 窗口结束时执行仲裁决议
func (a *VoiceArbitrator) Resolve(ctrl *VoiceWSController) {
	a.mu.Lock()
	events := a.events
	a.events = nil
	a.arbitrating = false
	a.mu.Unlock()

	if len(events) == 0 {
		return
	}

	// 1. 寻找音量分值最高的获胜者
	var winner *WakeEvent
	for _, e := range events {
		if winner == nil || e.Volume > winner.Volume {
			winner = e
		}
	}

	log.Printf("【仲裁器】多房间协同唤醒裁决完成！获胜者: 房间 [%s] (音量: %.2f)。本次共过滤抑制了 %d 个多余唤醒。",
		winner.RoomID, winner.Volume, len(events)-1)

	// 2. 激活并响应获胜的设备
	ctrl.handleSessionAction(winner.Conn, winner.WriteMu, winner.Action, winner.Codec, winner.OpusEncoder, winner.RoomID)

	// 3. 对所有失败的竞争者，强行使其重回休眠态，并通知客户端
	for _, e := range events {
		if e.RoomID != winner.RoomID {
			log.Printf("【仲裁器】过滤/静默房间 [%s] 的唤醒事件 (音量分值偏小: %.2f)", e.RoomID, e.Volume)
			e.Session.ForceSleep() // 重置状态机为休眠
			ctrl.sendMessage(e.Conn, e.WriteMu, WSMessage{
				Type:    "sleep",
				Message: "检测到更近的房间指令，红皇后继续保持休眠",
			})
		}
	}
}

// globalArbitrator 全局唯一的协同唤醒仲裁器实例
var globalArbitrator = &VoiceArbitrator{
	events: make([]*WakeEvent, 0),
}

// calculateVolume 计算一帧 PCM 16-bit 音频数据的平均绝对振幅（音量）
func calculateVolume(pcmData []byte) float64 {
	if len(pcmData) < 2 {
		return 0
	}
	var sum float64
	count := 0
	for i := 0; i < len(pcmData)-1; i += 2 {
		// 16位小端序 signed int16 转换
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
	activeCancels  sync.Map // Map[string]context.CancelFunc (RoomID -> cancel)
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
	if val, ok := ctrl.activeCancels.Load(roomID); ok {
		if cancel, ok := val.(context.CancelFunc); ok {
			cancel()
			log.Printf("【打断机制】成功中断房间 [%s] 的当前语音输出", roomID)
		}
		ctrl.activeCancels.Delete(roomID)
		// 通知客户端播放已被打断
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type:    "interrupt",
			Message: "playback_interrupted",
		})
	}
}

// HandleWebSocket 处理 GET /api/voice/ws —— 实时语音识别 WebSocket 端点
//
// 客户端协议:
//
//	发送: 二进制帧 = 原始 PCM 音频 (16kHz, mono, 16-bit little-endian)
//	      建议每帧 100ms = 3200 bytes (1600 samples × 2 bytes/sample)
//	接收: JSON 文本帧，结构见 WSMessage
func (ctrl *VoiceWSController) HandleWebSocket(c *gin.Context) {
	// 1. 检查 ASR 引擎是否可用
	asr := services.GetASR()
	if asr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "语音识别引擎未初始化，请检查 ASR 模型是否已正确配置",
		})
		return
	}

	// 2. 升级 HTTP 连接为 WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("【WebSocket】连接升级失败: %v", err)
		return
	}
	defer conn.Close()

	// 用于线程安全地写入 WebSocket
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
		_ = enc.SetComplexity(1) // 适合嵌入式/低算力设备的高效参数
		opusEncoder = enc
	}

	// 3. 创建该连接专用的 ASR 识别流
	stream := asr.NewStream()
	defer stream.Delete()

	// 4. 创建会话状态机
	session := services.NewSession(ctrl.wakeWord, ctrl.sessionTimeout, room)

	// 5. 启动超时检测协程
	done := make(chan struct{})
	defer close(done)

	go ctrl.timeoutWatcher(conn, &writeMu, session, done)

	// 6. 发送连接就绪消息
	ctrl.sendMessage(conn, &writeMu, WSMessage{
		Type:    "ready",
		Message: "语音连接就绪，请说 \"皇后\" 来唤醒",
	})

	// 7. 主循环：持续接收并处理音频数据
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
			// 二进制帧 = PCM / Opus 音频数据
			ctrl.processAudioFrame(conn, &writeMu, stream, session, data, codec, opusDecoder, opusEncoder)

		case websocket.TextMessage:
			// 文本帧 = 控制指令（如手动唤醒/休眠）
			ctrl.processControlMessage(conn, &writeMu, session, data)
		}
	}

	log.Println("【WebSocket】语音连接已断开")
}

// ---------------------------------------------------------------------------
// 音频处理核心逻辑
// ---------------------------------------------------------------------------

// processAudioFrame 处理一帧 PCM 或 Opus 音频数据
func (ctrl *VoiceWSController) processAudioFrame(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	stream *services.ASRStream,
	session *services.Session,
	pcmData []byte,
	codec string,
	opusDecoder *opus.Decoder,
	opusEncoder *opus.Encoder,
) {
	var vol float64

	// 1. 根据编码格式进行数据解析与音量计算
	if codec == "opus" {
		if opusDecoder == nil {
			return
		}
		// 每次 20ms @ 16kHz 为 320 采样点，使用 1024 大小缓冲区绰绰有余
		pcmBuf := make([]int16, 1024)
		n, err := opusDecoder.Decode(pcmData, pcmBuf)
		if err != nil {
			log.Printf("【ASR】Opus 音频帧解码失败: %v", err)
			return
		}
		if n <= 0 {
			return
		}
		vol = calculateVolumeSamples(pcmBuf[:n])
		session.UpdateVolume(vol)

		// 2. 直接将解码出的采样喂入 ASR
		stream.FeedSamples(pcmBuf[:n])
	} else {
		// 数据校验：PCM 16-bit 要求字节数为偶数
		if len(pcmData) < 2 || len(pcmData)%2 != 0 {
			return
		}
		vol = calculateVolume(pcmData)
		session.UpdateVolume(vol)

		// 2. 将 PCM 音频喂入 Sherpa-onnx 识别引擎
		stream.FeedAudio(pcmData)
	}

	// 3. 执行解码（处理缓冲区中的所有就绪帧）
	stream.Decode()

	// 4. 获取实时中间识别结果
	partialText := stream.GetResult()
	if partialText != "" {
		// 【打断机制】当正在播放大模型音频时，若 ASR 检测到非空中间文字，瞬间打断
		if _, ok := ctrl.activeCancels.Load(session.RoomID); ok {
			log.Printf("【打断机制】检测到用户说话 (ASR 中间文字: \"%s\")，即刻终止当前播放！", partialText)
			ctrl.interruptPlayback(conn, writeMu, session.RoomID)
		}

		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type: "partial",
			Text: partialText,
		})
	}

	// 5. 检查是否检测到语句结束（用户停顿超过阈值）
	if stream.IsEndpoint() {
		finalText := stream.GetResult()
		stream.Reset() // 重置流，准备识别下一句

		if finalText == "" {
			return
		}

		log.Printf("【ASR 识别完成】%s", finalText)

		// 发送最终识别结果
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type: "final",
			Text: finalText,
		})

		// 【打断机制】如果是在 ASR 最终确定时仍有播放活动，也进行最终二次打断清理
		ctrl.interruptPlayback(conn, writeMu, session.RoomID)

		// 6. 通过会话状态机判断该如何处理
		action := session.ProcessText(finalText)

		// 7. 多设备协同唤醒仲裁
		if action.Type == services.ActionWake || action.Type == services.ActionWakeAndExecute {
			event := &WakeEvent{
				RoomID:      session.RoomID,
				Volume:      session.GetMaxVolume(),
				Action:      action,
				Conn:        conn,
				WriteMu:     writeMu,
				Session:     session,
				Timestamp:   time.Now(),
				Codec:       codec,
				OpusEncoder: opusEncoder,
			}
			globalArbitrator.Submit(ctrl, event)
		} else {
			// 普通控制指令或休眠指令，无需仲裁，直接执行
			ctrl.handleSessionAction(conn, writeMu, action, codec, opusEncoder, session.RoomID)
		}
	}
}

// ---------------------------------------------------------------------------
// 会话动作处理
// ---------------------------------------------------------------------------

// handleSessionAction 根据会话状态机的决策执行对应操作
func (ctrl *VoiceWSController) handleSessionAction(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	action services.SessionAction,
	codec string,
	opusEncoder *opus.Encoder,
	roomID string,
) {
	switch action.Type {
	case services.ActionIgnore:
		// 休眠态且无唤醒词，不处理

	case services.ActionWake:
		// 唤醒成功，但没有附带指令
		log.Println("【会话】红皇后已唤醒，等待指令...")
		wakeMsg := "主人，红皇后已唤醒，静候指示"
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type:    "wake",
			Message: wakeMsg,
		})
		// 异步合成语音并推送给硬件设备扬声器
		go ctrl.streamAudioToClient(conn, writeMu, wakeMsg, codec, opusEncoder, roomID)

	case services.ActionWakeAndExecute:
		// 唤醒成功且附带指令（如 "皇后帮我开灯"）
		log.Printf("【会话】唤醒并执行指令: %s", action.Command)
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type:    "wake",
			Message: "红皇后已唤醒，正在处理指令...",
		})
		go ctrl.executeCommand(conn, writeMu, action.Command, codec, opusEncoder, roomID)

	case services.ActionExecute:
		// 激活态中收到指令
		log.Printf("【会话】执行指令: %s", action.Command)
		go ctrl.executeCommand(conn, writeMu, action.Command, codec, opusEncoder, roomID)

	case services.ActionSleep:
		// 用户主动结束对话
		log.Println("【会话】红皇后已休眠")
		sleepMsg := "红皇后已休眠，如需帮助请再次呼唤"
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type:    "sleep",
			Message: sleepMsg,
		})
		// 异步合成语音并推送给硬件设备扬声器
		go ctrl.streamAudioToClient(conn, writeMu, sleepMsg, codec, opusEncoder, roomID)
	}
}

// executeCommand 将指令文本发送给 NLP 解析并执行硬件控制
// 采用流式管线架构：大模型逐句输出 → 实时 TTS 合成 → 立刻推送音频，三者并行
func (ctrl *VoiceWSController) executeCommand(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	command string,
	codec string,
	opusEncoder *opus.Encoder,
	roomID string,
) {
	// 【打断机制】创建可随时取消的 Context，并保存其 CancelFunc
	ctx, cancel := context.WithCancel(context.Background())
	ctrl.activeCancels.Store(roomID, cancel)
	defer func() {
		cancel()
		ctrl.activeCancels.Delete(roomID)
	}()

	nlpSvc := services.NewNLPService()

	// ---------------------------------------------------------------------------
	// 尝试流式调用大模型（低延迟管线）
	// ---------------------------------------------------------------------------
	streamCh, streamErr := nlpSvc.ParseIntentStream(command)
	if streamErr != nil {
		// 流式调用失败，回退到传统同步模式
		log.Printf("【流式管线】流式调用失败，回退同步模式: %v", streamErr)
		ctrl.executeCommandSync(conn, writeMu, command, codec, opusEncoder, roomID, ctx)
		return
	}

	log.Println("【流式管线】已启动大模型流式输出，开始逐句合成语音...")

	// ---------------------------------------------------------------------------
	// 流式管线核心：边接收 token 边断句 → 逐句 TTS → 逐句推送音频
	// ---------------------------------------------------------------------------
	var (
		sentenceBuf   strings.Builder // 累积 token 直到构成一个完整句子
		fullReply     strings.Builder // 收集完整回复文本
		audioStarted  bool            // 是否已发送 audio_start
		totalAudioLen int             // 已推送的音频总字节数
		ttsSvc        = services.NewTTSService()
		toolCallInfo  *services.ToolCallInfo
	)

	// 中文断句分隔符集合
	sentenceEnders := "。！？；\n"

	for chunk := range streamCh {
		// 【打断检测】在接收大模型下一个 chunk 之前，检查当前是否已经被用户打断
		if ctx.Err() != nil {
			log.Println("【打断机制】检测到打断信号，主动终止 LLM 令牌循环")
			return
		}

		// 处理错误
		if chunk.Error != nil {
			log.Printf("【流式管线】大模型流式输出错误: %v", chunk.Error)
			if audioStarted {
				ctrl.sendMessage(conn, writeMu, WSMessage{Type: "audio_end", Message: "playback_complete"})
			}
			ctrl.sendMessage(conn, writeMu, WSMessage{
				Type: "result", Intent: "error", Status: "failed",
				Message: "语义分析失败: " + chunk.Error.Error(),
			})
			return
		}

		// 处理工具调用结果
		if chunk.Done && chunk.ToolCall != nil {
			toolCallInfo = chunk.ToolCall
			break
		}

		// 累积 token
		if chunk.Token != "" {
			sentenceBuf.WriteString(chunk.Token)
			fullReply.WriteString(chunk.Token)

			// 推送实时 token 给前端 UI 展示
			ctrl.sendMessage(conn, writeMu, WSMessage{
				Type:    "stream_token",
				Message: chunk.Token,
			})
		}

		// 检测断句：当累积的文本以句末标点结尾时，立刻触发 TTS 合成
		currentSentence := sentenceBuf.String()
		shouldSynth := false

		if chunk.Done {
			shouldSynth = len(strings.TrimSpace(currentSentence)) > 0
		} else if len(currentSentence) > 0 {
			lastRune := []rune(currentSentence)
			if len(lastRune) > 0 && strings.ContainsRune(sentenceEnders, lastRune[len(lastRune)-1]) {
				shouldSynth = true
			}
			// 逗号处也可断句，但仅当累积超过一定长度时（避免过于碎片化）
			if !shouldSynth && len(lastRune) > 8 && strings.ContainsRune("，、,", lastRune[len(lastRune)-1]) {
				shouldSynth = true
			}
		}

		if shouldSynth && len(strings.TrimSpace(currentSentence)) > 0 {
			sentence := strings.TrimSpace(currentSentence)
			sentenceBuf.Reset()

			// 首次合成时发送 audio_start 通知设备
			if !audioStarted {
				// 支持发送音频编码标头
				audioMsg := "pcm_16k_16bit_mono"
				if codec == "opus" {
					audioMsg = "opus_16k_mono"
				}
				ctrl.sendMessage(conn, writeMu, WSMessage{
					Type: "audio_start", Message: audioMsg,
				})
				audioStarted = true
			}

			// TTS 合成并立刻推送（同步执行，保证音频顺序）
			audioData, ttsErr := ttsSvc.Synthesize(sentence)
			if ttsErr != nil {
				log.Printf("【流式管线】句段 TTS 合成失败: %v (句段: %s)", ttsErr, sentence)
				continue
			}
			if len(audioData) > 0 {
				totalAudioLen += len(audioData)
				ctrl.pushAudioChunks(conn, writeMu, audioData, codec, opusEncoder, ctx)
			}
		}

		if chunk.Done {
			break
		}
	}

	// ---------------------------------------------------------------------------
	// 处理工具调用（大模型命中外部 MCP 工具，执行后发起二阶段流式重生成）
	// ---------------------------------------------------------------------------
	var status, message, intent string
	var confidence float64

	if toolCallInfo != nil {
		intent = "external_mcp_call"
		confidence = 0.98

		log.Printf("【流式管线】命中外部工具调用: %s | 参数: %s", toolCallInfo.Name, toolCallInfo.Arguments)

		// 1. 发送提示给前端，表明正在执行外部工具
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type:    "stream_token",
			Message: fmt.Sprintf("（正在执行外部工具 [%s]...）\n", toolCallInfo.Name),
		})

		// 2. 执行外部工具
		_, _, toolToServerMap := services.GetExternalMCPTools()
		srv, exists := toolToServerMap[toolCallInfo.Name]
		var respText string
		if !exists {
			status = "failed"
			respText = fmt.Sprintf("未在后台找到执行工具 [%s] 的在线 MCP 服务器", toolCallInfo.Name)
		} else {
			var callErr error
			respText, callErr = services.CallExternalMCPTool(srv, toolCallInfo.Name, toolCallInfo.Arguments)
			if callErr != nil {
				status = "failed"
				respText = "工具执行出错: " + callErr.Error()
			} else {
				status = "success"
			}
		}

		log.Printf("【流式管线】工具执行完毕，状态: %s。发起第二轮流式性格重生成...", status)

		// 3. 发起第二轮流式性格重生成
		streamCh2, err := nlpSvc.GenerateStreamingToolReply(command, toolCallInfo.Name, toolCallInfo.Arguments, respText)
		if err != nil {
			log.Printf("【流式管线】第二轮流式性格重生成启动失败: %v", err)
			message = "工具执行结果: " + respText
			// 兜底：直接合成并推送原始工具回复
			if !audioStarted {
				audioMsg := "pcm_16k_16bit_mono"
				if codec == "opus" {
					audioMsg = "opus_16k_mono"
				}
				ctrl.sendMessage(conn, writeMu, WSMessage{Type: "audio_start", Message: audioMsg})
				audioStarted = true
			}
			if audioData, err := ttsSvc.Synthesize(message); err == nil && len(audioData) > 0 {
				totalAudioLen += len(audioData)
				ctrl.pushAudioChunks(conn, writeMu, audioData, codec, opusEncoder, ctx)
			}
		} else {
			// 清空之前的 buffers，开启二阶段流式渲染与 TTS
			sentenceBuf.Reset()
			fullReply.Reset()

			for chunk := range streamCh2 {
				// 【打断检测】
				if ctx.Err() != nil {
					log.Println("【打断机制】检测到打断信号，主动终止第二轮 LLM 令牌循环")
					return
				}

				if chunk.Error != nil {
					log.Printf("【流式管线】第二轮大模型流式输出错误: %v", chunk.Error)
					break
				}

				if chunk.Token != "" {
					sentenceBuf.WriteString(chunk.Token)
					fullReply.WriteString(chunk.Token)

					// 推送实时 token 给前端 UI
					ctrl.sendMessage(conn, writeMu, WSMessage{
						Type:    "stream_token",
						Message: chunk.Token,
					})
				}

				currentSentence := sentenceBuf.String()
				shouldSynth := false

				if chunk.Done {
					shouldSynth = len(strings.TrimSpace(currentSentence)) > 0
				} else if len(currentSentence) > 0 {
					lastRune := []rune(currentSentence)
					if len(lastRune) > 0 && strings.ContainsRune(sentenceEnders, lastRune[len(lastRune)-1]) {
						shouldSynth = true
					}
					if !shouldSynth && len(lastRune) > 8 && strings.ContainsRune("，、,", lastRune[len(lastRune)-1]) {
						shouldSynth = true
					}
				}

				if shouldSynth && len(strings.TrimSpace(currentSentence)) > 0 {
					sentence := strings.TrimSpace(currentSentence)
					sentenceBuf.Reset()

					if !audioStarted {
						audioMsg := "pcm_16k_16bit_mono"
						if codec == "opus" {
							audioMsg = "opus_16k_mono"
						}
						ctrl.sendMessage(conn, writeMu, WSMessage{
							Type: "audio_start", Message: audioMsg,
						})
						audioStarted = true
					}

					audioData, ttsErr := ttsSvc.Synthesize(sentence)
					if ttsErr != nil {
						log.Printf("【流式管线】第二轮句段 TTS 合成失败: %v (句段: %s)", ttsErr, sentence)
						continue
					}
					if len(audioData) > 0 {
						totalAudioLen += len(audioData)
						ctrl.pushAudioChunks(conn, writeMu, audioData, codec, opusEncoder, ctx)
					}
				}
			}

			message = strings.TrimSpace(fullReply.String())
			if message == "" {
				message = "工具执行完毕，但生成回复为空"
			}
		}
	} else {
		intent = "conversation"
		confidence = 0.98
		status = "success"
		message = strings.TrimSpace(fullReply.String())
		if message == "" {
			message = "无法识别该指令的意图"
		}
	}


	// 发送 audio_end 通知设备恢复麦克风
	if audioStarted {
		ctrl.sendMessage(conn, writeMu, WSMessage{Type: "audio_end", Message: "playback_complete"})
		log.Printf("【流式管线】音频推送完毕, 共 %d 字节 (%.1f 秒)",
			totalAudioLen, float64(totalAudioLen)/(16000.0*2.0))
	}

	// 保存到数据库
	voiceSvc := services.NewVoiceService()
	_, _ = voiceSvc.CreateVoiceCommand("", command, confidence)

	// 推送最终完整结果给客户端
	ctrl.sendMessage(conn, writeMu, WSMessage{
		Type: "result", Intent: intent, Status: status, Message: message,
	})
}

// executeCommandSync 同步模式执行指令（流式调用失败时的回退方案）
func (ctrl *VoiceWSController) executeCommandSync(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	command string,
	codec string,
	opusEncoder *opus.Encoder,
	roomID string,
	ctx context.Context,
) {
	nlpSvc := services.NewNLPService()
	parseResult, err := nlpSvc.ParseIntent(command)

	var status, message, intent string
	var confidence float64

	if err != nil {
		status = "failed"
		message = "语义分析失败: " + err.Error()
		intent = "error"
	} else {
		intent = parseResult.Intent
		confidence = parseResult.Confidence
		if parseResult.IsExternal {
			if parseResult.ToolStatus == "failed" {
				status = "failed"
				message = "外部工具执行失败: " + parseResult.ToolError
			} else {
				status = "success"
				message = parseResult.ReplyText
			}
		} else {
			status = "success"
			if parseResult.ReplyText != "" {
				message = parseResult.ReplyText
			} else {
				message = "无法识别该指令的意图"
			}
		}

	}

	voiceSvc := services.NewVoiceService()
	_, _ = voiceSvc.CreateVoiceCommand("", command, confidence)

	ctrl.sendMessage(conn, writeMu, WSMessage{
		Type: "result", Intent: intent, Status: status, Message: message,
	})
	go ctrl.streamAudioToClient(conn, writeMu, message, codec, opusEncoder, roomID, ctx)
}

// ---------------------------------------------------------------------------
// 控制消息处理
// ---------------------------------------------------------------------------

// processControlMessage 处理客户端发送的文本控制指令
func (ctrl *VoiceWSController) processControlMessage(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	session *services.Session,
	data []byte,
) {
	// 支持简单的文本控制指令
	cmd := string(data)
	switch cmd {
	case "wake":
		// 手动唤醒（调试用）
		action := session.ProcessText(session.WakeWord)
		ctrl.handleSessionAction(conn, writeMu, action, "pcm", nil, session.RoomID)
	case "sleep":
		// 手动休眠
		action := session.ProcessText("好了")
		ctrl.handleSessionAction(conn, writeMu, action, "pcm", nil, session.RoomID)
	case "ping":
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type:    "pong",
			Message: "alive",
		})
	case "interrupt":
		// 【打断机制】支持手动大声或者按钮打断
		log.Printf("【打断机制】收到客户端主动打断指令，停止房间 [%s] 的当前播放", session.RoomID)
		ctrl.interruptPlayback(conn, writeMu, session.RoomID)
	}
}

// ---------------------------------------------------------------------------
// 超时检测与消息发送
// ---------------------------------------------------------------------------

// timeoutWatcher 后台协程，定期检查激活态是否已超时
func (ctrl *VoiceWSController) timeoutWatcher(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	session *services.Session,
	done <-chan struct{},
) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if session.CheckTimeout() {
				log.Println("【会话】超时自动休眠")
				ctrl.sendMessage(conn, writeMu, WSMessage{
					Type:    "sleep",
					Message: "红皇后已因超时休眠，如需帮助请再次呼唤",
				})
			}
		case <-done:
			return
		}
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

// ---------------------------------------------------------------------------
// 服务端 TTS 语音合成与音频回传（用于硬件设备扬声器播放）
// ---------------------------------------------------------------------------

// pushAudioChunks 将 PCM 音频数据分块通过 WebSocket 二进制帧推送给设备，支持 Opus 压缩与打断取消
func (ctrl *VoiceWSController) pushAudioChunks(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	audioData []byte,
	codec string,
	opusEncoder *opus.Encoder,
	ctx context.Context,
) {
	if codec == "opus" {
		if opusEncoder == nil {
			return
		}
		// 16kHz 16-bit mono -> 20ms frame = 320 samples = 640 bytes
		const frameSamples = 320
		const frameBytes = frameSamples * 2

		// 补齐尾部数据，确保是 640 字节（20ms）的整数倍
		leftover := len(audioData) % frameBytes
		if leftover > 0 {
			padding := make([]byte, frameBytes-leftover)
			audioData = append(audioData, padding...)
		}

		// 将字节数组转换为 int16 采样
		pcmSamples := make([]int16, len(audioData)/2)
		for i := 0; i < len(pcmSamples); i++ {
			pcmSamples[i] = int16(binary.LittleEndian.Uint16(audioData[i*2 : i*2+2]))
		}

		opusBuf := make([]byte, 1024)
		for i := 0; i < len(pcmSamples); i += frameSamples {
			// 【打断检测】在每个 Opus 分片发送前，快速检查是否已打断
			if ctx.Err() != nil {
				return
			}

			end := i + frameSamples
			n, err := opusEncoder.Encode(pcmSamples[i:end], opusBuf)
			if err != nil {
				log.Printf("【TTS Opus 编码】压缩音频失败: %v", err)
				return
			}

			writeMu.Lock()
			writeErr := conn.WriteMessage(websocket.BinaryMessage, opusBuf[:n])
			writeMu.Unlock()

			if writeErr != nil {
				log.Printf("【TTS Opus 回传】数据包推送失败: %v", writeErr)
				return
			}

			// 20ms 的音频数据包，微弱延时以模拟真实播放速率，防止缓冲区溢出
			time.Sleep(18 * time.Millisecond) // 略微小于 20ms，保持平滑且不会累积延时
		}
	} else {
		// 原有的 PCM 分块推送逻辑
		chunkSize := 4096
		for i := 0; i < len(audioData); i += chunkSize {
			// 【打断检测】
			if ctx.Err() != nil {
				return
			}
			end := i + chunkSize
			if end > len(audioData) {
				end = len(audioData)
			}

			writeMu.Lock()
			writeErr := conn.WriteMessage(websocket.BinaryMessage, audioData[i:end])
			writeMu.Unlock()

			if writeErr != nil {
				log.Printf("【TTS 回传】音频推送中断: %v", writeErr)
				return
			}
		}
	}
}

// streamAudioToClient 将文本通过 Edge TTS 合成为 PCM 音频，并以 WebSocket 二进制帧流式推送给硬件设备
// 用于简短的唤醒/休眠提示音，支持打断和 Opus 格式
func (ctrl *VoiceWSController) streamAudioToClient(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	text string,
	codec string,
	opusEncoder *opus.Encoder,
	roomID string,
	parentCtx ...context.Context,
) {
	if text == "" {
		return
	}

	// 准备支持打断取消的 Context
	var ctx context.Context
	var cancel context.CancelFunc
	if len(parentCtx) > 0 && parentCtx[0] != nil {
		ctx = parentCtx[0]
	} else {
		ctx, cancel = context.WithCancel(context.Background())
		ctrl.activeCancels.Store(roomID, cancel)
		defer func() {
			cancel()
			ctrl.activeCancels.Delete(roomID)
		}()
	}

	ttsSvc := services.NewTTSService()
	audioData, err := ttsSvc.Synthesize(text)
	if err != nil {
		log.Printf("【TTS 回传】语音合成失败: %v", err)
		return
	}
	if len(audioData) == 0 {
		return
	}

	audioMsg := "pcm_16k_16bit_mono"
	if codec == "opus" {
		audioMsg = "opus_16k_mono"
	}

	ctrl.sendMessage(conn, writeMu, WSMessage{Type: "audio_start", Message: audioMsg})
	ctrl.pushAudioChunks(conn, writeMu, audioData, codec, opusEncoder, ctx)
	ctrl.sendMessage(conn, writeMu, WSMessage{Type: "audio_end", Message: "playback_complete"})

	log.Printf("【TTS 回传】音频已推送至设备, 共 %d 字节 (%.1f 秒)",
		len(audioData), float64(len(audioData))/(16000.0*2.0))
}

// calculateVolumeSamples 计算 int16 采样切片的平均绝对振幅（音量）
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
