package services

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// ---------------------------------------------------------------------------
// 全局 ASR 引擎单例（类似 database.DB 的设计模式）
// ---------------------------------------------------------------------------

var (
	asrEngine     *ASRService
	asrEngineOnce sync.Once
)

// ASRService Sherpa-onnx 离线语音识别引擎封装
type ASRService struct {
	recognizer *sherpa.OnlineRecognizer
}

// ASRStream 每个 WebSocket 连接独享的识别流，封装了底层 sherpa 类型
type ASRStream struct {
	stream     *sherpa.OnlineStream
	recognizer *sherpa.OnlineRecognizer
}

// InitASR 初始化全局 ASR 引擎，应在 main 函数启动时调用一次
// modelDir 为 Sherpa-onnx 模型文件所在目录
func InitASR(modelDir string) error {
	var initErr error
	asrEngineOnce.Do(func() {
		config := &sherpa.OnlineRecognizerConfig{
			ModelConfig: sherpa.OnlineModelConfig{
				Transducer: sherpa.OnlineTransducerModelConfig{
					Encoder: modelDir + "/encoder-epoch-99-avg-1.int8.onnx",
					Decoder: modelDir + "/decoder-epoch-99-avg-1.int8.onnx",
					Joiner:  modelDir + "/joiner-epoch-99-avg-1.int8.onnx",
				},
				Tokens:     modelDir + "/tokens.txt",
				Provider:   "cpu",
				Debug:      0,
				NumThreads: 2,
			},
			FeatConfig: sherpa.FeatureConfig{
				SampleRate: 16000,
				FeatureDim: 80,
			},
			DecodingMethod:         "greedy_search",
			EnableEndpoint:         1,
			Rule1MinTrailingSilence: 2.4,  // 长停顿阈值（秒），超过则判定为句末
			Rule2MinTrailingSilence: 1.2,  // 短停顿阈值（秒），句中自然停顿
			Rule3MinUtteranceLength: 20.0, // 单句最大时长（秒）
		}

		recognizer := sherpa.NewOnlineRecognizer(config)
		if recognizer == nil {
			initErr = fmt.Errorf("Sherpa-onnx 识别器初始化失败，请检查模型文件路径: %s", modelDir)
			return
		}

		asrEngine = &ASRService{
			recognizer: recognizer,
		}
		log.Printf("【ASR 引擎】Sherpa-onnx 初始化成功，模型目录: %s", modelDir)
	})
	return initErr
}

// GetASR 获取全局 ASR 引擎实例，未初始化时返回 nil
func GetASR() *ASRService {
	return asrEngine
}

// ---------------------------------------------------------------------------
// ASRService 方法
// ---------------------------------------------------------------------------

// NewStream 创建新的识别流（每个 WebSocket 连接应创建独立的流）
func (s *ASRService) NewStream() *ASRStream {
	return &ASRStream{
		stream:     sherpa.NewOnlineStream(s.recognizer),
		recognizer: s.recognizer,
	}
}

// Close 释放全局识别器资源，应在程序退出时调用
func (s *ASRService) Close() {
	if s.recognizer != nil {
		sherpa.DeleteOnlineRecognizer(s.recognizer)
		log.Println("【ASR 引擎】Sherpa-onnx 识别器已释放")
	}
}

// ---------------------------------------------------------------------------
// ASRStream 方法 —— 控制器直接操作这些方法，无需导入 sherpa 包
// ---------------------------------------------------------------------------

// FeedAudio 将原始 16-bit little-endian PCM 字节数据喂入识别流
func (s *ASRStream) FeedAudio(pcmData []byte) {
	samples := PCMBytesToFloat32(pcmData)
	s.stream.AcceptWaveform(16000, samples)
}

// Decode 处理识别流中已缓冲的全部音频帧
func (s *ASRStream) Decode() {
	for s.recognizer.IsReady(s.stream) {
		s.recognizer.Decode(s.stream)
	}
}

// GetResult 获取当前累积的识别文字（包含中间结果）
func (s *ASRStream) GetResult() string {
	result := s.recognizer.GetResult(s.stream)
	return result.Text
}

// IsEndpoint 检查是否检测到语句结束（用户停顿超过阈值）
func (s *ASRStream) IsEndpoint() bool {
	return s.recognizer.IsEndpoint(s.stream)
}

// Reset 重置识别流状态，一句话识别完毕后调用以准备下一句
func (s *ASRStream) Reset() {
	s.recognizer.Reset(s.stream)
}

// Delete 释放识别流资源，WebSocket 连接关闭时必须调用
func (s *ASRStream) Delete() {
	if s.stream != nil {
		sherpa.DeleteOnlineStream(s.stream)
	}
}

// ---------------------------------------------------------------------------
// 音频格式转换工具
// ---------------------------------------------------------------------------

// PCMBytesToFloat32 将 16-bit little-endian PCM 字节数组转换为归一化 float32 切片
// 输入: 原始 PCM 字节 (每个采样 2 字节, little-endian)
// 输出: 归一化到 [-1.0, 1.0] 的 float32 切片
func PCMBytesToFloat32(pcmData []byte) []float32 {
	numSamples := len(pcmData) / 2
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		sample := int16(binary.LittleEndian.Uint16(pcmData[i*2 : i*2+2]))
		samples[i] = float32(sample) / 32768.0
	}
	return samples
}
