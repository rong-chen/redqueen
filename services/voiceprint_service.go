package services

import (
	"fmt"
	"log"
	"math"
	"sync"

	"RedQueenSystem/utils"
	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

var (
	voiceprintService     *VoiceprintService
	voiceprintServiceOnce sync.Once
)

// VoiceprintService 说话人声纹特征提取与匹配服务
type VoiceprintService struct {
	extractor *sherpa.SpeakerEmbeddingExtractor
}

// InitVoiceprint 初始化全局声纹提取器
func InitVoiceprint(modelPath string) error {
	var initErr error
	voiceprintServiceOnce.Do(func() {
		config := &sherpa.SpeakerEmbeddingExtractorConfig{
			Model:      modelPath,
			NumThreads: 2,
			Provider:   "cpu",
		}
		extractor := sherpa.NewSpeakerEmbeddingExtractor(config)
		if extractor == nil {
			initErr = fmt.Errorf("声纹提取器初始化失败，请检查模型文件路径: %s", modelPath)
			return
		}
		voiceprintService = &VoiceprintService{
			extractor: extractor,
		}
		log.Printf("【声纹引擎】声纹提取器初始化成功，模型文件: %s", modelPath)
	})
	return initErr
}

// GetVoiceprint 获取全局声纹服务实例
func GetVoiceprint() *VoiceprintService {
	return voiceprintService
}

// ExtractEmbedding 从 16kHz Mono 16-bit PCM 采样数据中提取 256 维声纹 Embedding 向量
func (s *VoiceprintService) ExtractEmbedding(samples []int16) ([]float32, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("输入音频采样数据为空")
	}

	// 使用 VAD 切除前后的静音，只保留人声部分
	trimmedSamples := utils.TrimSilence(samples)
	if len(trimmedSamples) == 0 {
		return nil, fmt.Errorf("VAD 静音裁剪后音频为空(未检测到有效人声)")
	}

	stream := s.extractor.CreateStream()
	if stream == nil {
		return nil, fmt.Errorf("创建声纹流失败")
	}
	defer sherpa.DeleteOnlineStream(stream)

	// 转换 trimmedSamples 到 float32 归一化形式
	floatSamples := make([]float32, len(trimmedSamples))
	for i, v := range trimmedSamples {
		floatSamples[i] = float32(v) / 32768.0
	}

	// 输入波形数据并标记结束
	stream.AcceptWaveform(16000, floatSamples)
	stream.InputFinished()

	// 提取声纹 Embedding
	if !s.extractor.IsReady(stream) {
		return nil, fmt.Errorf("声纹提取流未就绪(音频过短或无效)")
	}

	embedding := s.extractor.Compute(stream)
	return embedding, nil
}

// VerifySpeaker 计算两个声纹 Embedding 之间的余弦相似度 (Cosine Similarity)
// 返回值范围：[-1.0, 1.0]，越接近 1.0 说明音色越一致，属于同一个人说话。
func (s *VoiceprintService) VerifySpeaker(emb1, emb2 []float32) float64 {
	if len(emb1) != len(emb2) || len(emb1) == 0 {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(emb1); i++ {
		valA := float64(emb1[i])
		valB := float64(emb2[i])
		dotProduct += valA * valB
		normA += valA * valA
		normB += valB * valB
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// AverageEmbeddings 计算多个声纹特征向量的平均向量
func (s *VoiceprintService) AverageEmbeddings(embs [][]float32) ([]float32, error) {
	if len(embs) == 0 {
		return nil, fmt.Errorf("没有提供任何声纹向量")
	}

	dim := len(embs[0])
	avgEmb := make([]float32, dim)

	for _, emb := range embs {
		if len(emb) != dim {
			return nil, fmt.Errorf("声纹向量维度不一致")
		}
		for i := 0; i < dim; i++ {
			avgEmb[i] += emb[i]
		}
	}

	count := float32(len(embs))
	for i := 0; i < dim; i++ {
		avgEmb[i] /= count
	}

	return avgEmb, nil
}
