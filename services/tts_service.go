package services

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// Edge TTS 常量定义
// ---------------------------------------------------------------------------

const (
	// 微软 Edge 浏览器 Read Aloud 功能的公开 TTS WebSocket 端点（免费、无需 API Key）
	edgeTTSEndpoint = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
	edgeTTSToken    = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"

	// 默认使用微软晓晓（XiaoxiaoNeural）中文女声，音质自然流畅，非常适合智能助理
	DefaultTTSVoice = "zh-CN-XiaoxiaoNeural"
)

// ---------------------------------------------------------------------------
// TTSService 文本转语音合成服务
// ---------------------------------------------------------------------------

// TTSService 基于微软 Edge TTS 免费接口实现的高质量中文语音合成服务
// 输出格式: PCM 16kHz 16-bit 单声道（与 ASR 输入格式完全一致）
type TTSService struct {
	Voice string // TTS 音色名称，如 zh-CN-XiaoxiaoNeural
}

// NewTTSService 创建 TTS 服务实例
func NewTTSService() *TTSService {
	return &TTSService{Voice: DefaultTTSVoice}
}

// Synthesize 将文本合成为 PCM 音频数据 (16kHz, 16-bit, mono, little-endian)
// 返回原始 PCM 字节数组，可直接通过 WebSocket 二进制帧发送给 ATOM Echo 设备播放
func (t *TTSService) Synthesize(text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	connID := generateHexID()
	url := fmt.Sprintf("%s?TrustedClientToken=%s&ConnectionId=%s",
		edgeTTSEndpoint, edgeTTSToken, connID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	header := http.Header{}
	header.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0")

	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return nil, fmt.Errorf("TTS WebSocket 连接失败: %v", err)
	}
	defer conn.Close()

	// 设置全局读取超时，防止挂死
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	// ---------------------------------------------------------------------------
	// 1. 发送音频输出格式配置
	// ---------------------------------------------------------------------------
	configMsg := "Content-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataOptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"false"},"outputFormat":"raw-16khz-16bit-mono-pcm"}}}}`

	if err := conn.WriteMessage(websocket.TextMessage, []byte(configMsg)); err != nil {
		return nil, fmt.Errorf("发送 TTS 配置失败: %v", err)
	}

	// ---------------------------------------------------------------------------
	// 2. 发送 SSML 合成请求
	// ---------------------------------------------------------------------------
	requestID := generateHexID()
	ssml := fmt.Sprintf(
		`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="zh-CN">`+
			`<voice name="%s">%s</voice></speak>`,
		t.Voice, escapeXML(text),
	)
	ssmlMsg := fmt.Sprintf(
		"X-RequestId:%s\r\nContent-Type:application/ssml+xml\r\nPath:ssml\r\n\r\n%s",
		requestID, ssml,
	)

	if err := conn.WriteMessage(websocket.TextMessage, []byte(ssmlMsg)); err != nil {
		return nil, fmt.Errorf("发送 TTS SSML 请求失败: %v", err)
	}

	// ---------------------------------------------------------------------------
	// 3. 接收合成的音频数据
	// ---------------------------------------------------------------------------
	var audioBuffer bytes.Buffer

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if audioBuffer.Len() > 0 {
				// 已有部分数据，可能是正常结束
				break
			}
			return nil, fmt.Errorf("读取 TTS 音频数据失败: %v", err)
		}

		switch msgType {
		case websocket.TextMessage:
			// 检查是否收到合成完成信号
			if strings.Contains(string(data), "Path:turn.end") {
				goto done
			}

		case websocket.BinaryMessage:
			// 二进制帧格式: [2字节头部长度(大端序)] + [头部文本] + [PCM音频数据]
			if len(data) < 2 {
				continue
			}
			headerLen := int(binary.BigEndian.Uint16(data[:2]))
			audioStart := 2 + headerLen
			if audioStart < len(data) {
				audioBuffer.Write(data[audioStart:])
			}
		}
	}

done:
	pcmBytes := audioBuffer.Bytes()
	if len(pcmBytes) > 0 {
		durationSec := float64(len(pcmBytes)) / (16000.0 * 2.0)
		log.Printf("【TTS】语音合成完成, 音色: %s, 音频大小: %d 字节 (%.1f 秒)",
			t.Voice, len(pcmBytes), durationSec)
	}

	return pcmBytes, nil
}

// ---------------------------------------------------------------------------
// 辅助工具函数
// ---------------------------------------------------------------------------

// escapeXML 转义 XML/SSML 中的特殊字符，防止合成请求解析异常
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// generateHexID 生成 32 位十六进制随机标识符（用于 Edge TTS 协议的连接 ID 和请求 ID）
func generateHexID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
