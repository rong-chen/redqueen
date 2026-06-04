package controllers

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

// WSClientMessage 客户端发送给服务端的 JSON 消息结构
type WSClientMessage struct {
	Type string `json:"type"`           // 消息类型: command, interrupt, ping
	Text string `json:"text,omitempty"` // 指令文本内容
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
	wakeWord         string
	sessionTimeout   time.Duration
	activeCancels    sync.Map // Map[string]context.CancelFunc (RoomID -> cancel)
	activeASRStreams sync.Map // Map[string]*services.DashScopeASRStream (RoomID -> stream)
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
// HandleWebSocket 处理 GET /api/voice/ws —— 实时语音识别 WebSocket 端点
//
// 客户端协议:
//
//	发送: 二进制帧 = 原始 PCM 音频 (16kHz, mono, 16-bit little-endian)
//	      建议每帧 100ms = 3200 bytes (1600 samples × 2 bytes/sample)
//	接收: JSON 文本帧，结构见 WSMessage
func (ctrl *VoiceWSController) HandleWebSocket(c *gin.Context) {
	// 1. 升级 HTTP 连接为 WebSocket
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

	// 2. 创建会话状态机，默认直接进入激活态
	session := services.NewSession(ctrl.wakeWord, ctrl.sessionTimeout, room)

	defer ctrl.stopDashScopeASR(room)

	// 3. 发送连接就绪消息
	ctrl.sendMessage(conn, &writeMu, WSMessage{
		Type:    "ready",
		Message: "语音连接就绪，安全通信链路已建立",
	})

	// 4. 主循环：持续接收并处理音频数据
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
			ctrl.processAudioFrame(conn, &writeMu, session, data, codec, opusDecoder, opusEncoder)

		case websocket.TextMessage:
			// 文本帧 = 控制指令（如手动唤醒/休眠，或前端 Web Speech API 识别的文本指令）
			ctrl.processControlMessage(conn, &writeMu, session, data, codec, opusEncoder)
		}
	}

	log.Println("【WebSocket】语音连接已断开")
}

// processAudioFrame 处理一帧 PCM 或 Opus 音频数据并发送到云端 ASR
func (ctrl *VoiceWSController) processAudioFrame(
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	session *services.Session,
	pcmData []byte,
	codec string,
	opusDecoder *opus.Decoder,
	opusEncoder *opus.Encoder,
) {
	var vol float64
	var pcmToSend []byte

	if codec == "opus" {
		if opusDecoder != nil {
			pcmBuf := make([]int16, 1024)
			n, err := opusDecoder.Decode(pcmData, pcmBuf)
			if err == nil && n > 0 {
				vol = calculateVolumeSamples(pcmBuf[:n])
				session.UpdateVolume(vol)
				
				// 将 []int16 转为 []byte (Little-Endian)
				pcmToSend = make([]byte, n*2)
				for i, v := range pcmBuf[:n] {
					binary.LittleEndian.PutUint16(pcmToSend[i*2:], uint16(v))
				}
			}
		}
	} else {
		pcmToSend = pcmData
		if len(pcmData) >= 2 && len(pcmData)%2 == 0 {
			vol = calculateVolume(pcmData)
			session.UpdateVolume(vol)
		}
	}

	if len(pcmToSend) > 0 {
		dashscopeStream, ok := ctrl.activeASRStreams.Load(session.RoomID)
		if !ok {
			// 创建新的百炼 ASR 识别流
			apiKey := os.Getenv("DASHSCOPE_API_KEY")
			if apiKey == "" {
				apiKey = "sk-41cd73b8bae44957aa31542e03f1521e"
			}
			var err error
			newStream, err := services.NewDashScopeASRStream(apiKey, func(text string, isFinal bool) {
				if isFinal {
					log.Printf("【百炼 ASR 最终转写】%s", text)

					// 校验是否在回复生成或播报状态中，杜绝自反馈环路
					isReplying := ctrl.IsReplying(session.RoomID)
					interruptWords := []string{"退下", "别说了", "闭嘴", "停", "不要", "安静", "再见"}
					hasInterruptWord := false
					for _, w := range interruptWords {
						if strings.Contains(text, w) {
							hasInterruptWord = true
							break
						}
					}

					if isReplying {
						if hasInterruptWord {
							log.Printf("【打断机制】检测到用户说话打断: %s，即刻终止当前播放！", text)
							ctrl.interruptPlayback(conn, writeMu, session.RoomID)
							ctrl.sendMessage(conn, writeMu, WSMessage{
								Type:    "interrupt",
								Message: "语音流被打断，接收全新指令",
							})
						}
						return
					}

					// 发送给前端显示最终结果
					ctrl.sendMessage(conn, writeMu, WSMessage{
						Type: "final",
						Text: text,
					})

					// 过滤单字
					if len([]rune(text)) <= 1 {
						log.Printf("【ASR】过滤单字或超短识别结果: %s", text)
						return
					}

					// 触发打断
					ctrl.interruptPlayback(conn, writeMu, session.RoomID)

					// 状态机处理指令
					action := session.ProcessText(text)
					ctrl.handleSessionAction(conn, writeMu, action, codec, opusEncoder, session.RoomID)
				} else {
					// 处于正常交互状态时，向前端发送中间文字，提供打字机回显
					if !ctrl.IsReplying(session.RoomID) {
						ctrl.sendMessage(conn, writeMu, WSMessage{
							Type: "partial",
							Text: text,
						})
					}
				}
			})
			if err != nil {
				log.Printf("【百炼 ASR】初始化流识别会话失败: %v", err)
				return
			}
			ctrl.activeASRStreams.Store(session.RoomID, newStream)
			dashscopeStream = newStream
		}

		if ds, ok := dashscopeStream.(*services.DashScopeASRStream); ok {
			_ = ds.SendAudio(pcmToSend)
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
	case services.ActionExecute:
		// 激活态中收到指令
		log.Printf("【会话】执行指令: %s", action.Command)
		go ctrl.executeCommand(conn, writeMu, action.Command, codec, opusEncoder, roomID)
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
			// 1. 遇到句末强标点立即断句
			if len(lastRune) > 0 && strings.ContainsRune(sentenceEnders, lastRune[len(lastRune)-1]) {
				shouldSynth = true
			}
			// 2. 遇到逗号/顿号，且累积字数达到 15 个字以上才断句 (提升语气连贯性)
			if !shouldSynth && len(lastRune) >= 15 && strings.ContainsRune("，、,", lastRune[len(lastRune)-1]) {
				shouldSynth = true
			}
			// 3. 无标点长句，达到 25 个字才强制断句 (确保 CosyVoice 具有充足语境)
			if !shouldSynth && len(lastRune) >= 25 {
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
					// 1. 遇到句末强标点立即断句
					if len(lastRune) > 0 && strings.ContainsRune(sentenceEnders, lastRune[len(lastRune)-1]) {
						shouldSynth = true
					}
					// 2. 遇到逗号/顿号，且累积字数达到 15 个字以上才断句 (提升语气连贯性)
					if !shouldSynth && len(lastRune) >= 15 && strings.ContainsRune("，、,", lastRune[len(lastRune)-1]) {
						shouldSynth = true
					}
					// 3. 无标点长句，达到 25 个字才强制断句 (确保 CosyVoice 具有充足语境)
					if !shouldSynth && len(lastRune) >= 25 {
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
	codec string,
	opusEncoder *opus.Encoder,
) {
	// 1. 尝试以 JSON 格式解析客户端消息
	var clientMsg WSClientMessage
	if err := json.Unmarshal(data, &clientMsg); err == nil && clientMsg.Type != "" {
		switch clientMsg.Type {
		case "command":
			if clientMsg.Text != "" {
				log.Printf("【收到客户端文本指令】: %s", clientMsg.Text)
				// 发送最终识别结果给前端，确保界面显示（与 ASR 流程一致）
				ctrl.sendMessage(conn, writeMu, WSMessage{
					Type: "final",
					Text: clientMsg.Text,
				})
				// 【打断机制】如果仍有播放活动，进行打断清理
				ctrl.interruptPlayback(conn, writeMu, session.RoomID)

				// 通过会话状态机进行处理
				action := session.ProcessText(clientMsg.Text)
				ctrl.handleSessionAction(conn, writeMu, action, codec, opusEncoder, session.RoomID)
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

	// 2. 如果不是 JSON 格式，回退到原有的简单文本控制指令
	cmd := string(data)
	switch cmd {
	case "ping":
		ctrl.sendMessage(conn, writeMu, WSMessage{
			Type:    "pong",
			Message: "alive",
		})
	case "interrupt":
		// 【打断机制】支持手动大声或者按钮打断
		log.Printf("【打断机制】收到客户端普通文本打断指令，停止房间 [%s] 的当前播放", session.RoomID)
		ctrl.interruptPlayback(conn, writeMu, session.RoomID)
	}
}

// IsReplying 检查大模型是否正处于回复/生成/推送流程中（通过 roomID 在 activeCancels 中是否存在判断）
func (ctrl *VoiceWSController) IsReplying(roomID string) bool {
	_, ok := ctrl.activeCancels.Load(roomID)
	return ok
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

// stopDashScopeASR 关闭并清理房间的百炼 ASR 流式连接
func (ctrl *VoiceWSController) stopDashScopeASR(roomID string) {
	if val, ok := ctrl.activeASRStreams.Load(roomID); ok {
		if stream, ok := val.(*services.DashScopeASRStream); ok {
			stream.Close()
			log.Printf("【百炼 ASR】已关闭并清理房间 [%s] 的流式识别会话", roomID)
		}
		ctrl.activeASRStreams.Delete(roomID)
	}
}

