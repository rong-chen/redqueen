package controllers

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"RedQueenSystem/database"
	"RedQueenSystem/models"
	"RedQueenSystem/services"
	"RedQueenSystem/utils"

	"github.com/gin-gonic/gin"
)

// ConfigController 负责系统的动态配置读取与保存
type ConfigController struct{}

// NewConfigController 实例化配置控制器
func NewConfigController() *ConfigController {
	return &ConfigController{}
}

// GetModelConfig 处理 GET /api/config/model - 读取大模型配置，对 ApiKey 实施安全脱敏
func (ctrl *ConfigController) GetModelConfig(c *gin.Context) {
	var cfg models.ModelConfig
	err := database.DB.First(&cfg).Error
	if err != nil {
		utils.ServerError(c, "获取模型参数配置失败: "+err.Error())
		return
	}

	// 安全脱敏处理: 如果有 API Key，隐藏其主体内容，仅作占位符返回给前端
	if cfg.ApiKey != "" {
		cfg.ApiKey = "******"
	}

	utils.Success(c, cfg, "获取大模型参数配置成功")
}

// UpdateModelConfig 处理 POST /api/config/model - 更新大模型配置参数
func (ctrl *ConfigController) UpdateModelConfig(c *gin.Context) {
	var input models.ModelConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.BadRequest(c, "参数解析失败: "+err.Error())
		return
	}

	var existing models.ModelConfig
	err := database.DB.First(&existing).Error
	if err != nil {
		utils.ServerError(c, "未初始化默认配置，更新失败: "+err.Error())
		return
	}

	// 核心安全逻辑:
	// 如果前端传过来的 Key 是 "******"，说明用户没有修改 Key，我们应该保留数据库中原有的密文 Key。
	// 否则，说明用户输入了新 Key，我们才覆盖原有值。
	if input.ApiKey == "******" {
		input.ApiKey = existing.ApiKey
	}

	// 更新字段值
	existing.ActiveMode = input.ActiveMode
	existing.ApiKey = input.ApiKey
	existing.ApiURL = input.ApiURL
	existing.ModelName = input.ModelName
	existing.SystemPrompt = input.SystemPrompt
	existing.SystemRole = input.SystemRole
	existing.SystemPersonality = input.SystemPersonality
	existing.EnableVoiceprint = input.EnableVoiceprint
	existing.VoiceprintThreshold = input.VoiceprintThreshold
	if input.MasterVoiceprint != "" {
		existing.MasterVoiceprint = input.MasterVoiceprint
	}

	err = database.DB.Save(&existing).Error
	if err != nil {
		utils.ServerError(c, "保存模型参数配置失败: "+err.Error())
		return
	}

	// 同样脱敏返回
	if existing.ApiKey != "" {
		existing.ApiKey = "******"
	}

	utils.Success(c, existing, "保存模型参数配置成功")
}

// RegisterVoiceprint 处理 POST /api/config/voiceprint/register
// 接收前端采集的 Base64 编码的 16kHz mono 16bit PCM 音频数据，提取声纹特征并追加到主人声纹库中
func (ctrl *ConfigController) RegisterVoiceprint(c *gin.Context) {
	var req struct {
		AudioData string `json:"audio_data"` // Base64 PCM 数据
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequest(c, "参数解析失败: "+err.Error())
		return
	}

	rawBytes, err := base64.StdEncoding.DecodeString(req.AudioData)
	if err != nil {
		utils.BadRequest(c, "Base64 解码失败: "+err.Error())
		return
	}

	if len(rawBytes) < 2 {
		utils.BadRequest(c, "音频数据为空或过短")
		return
	}

	// 转换为 []int16 采样
	numSamples := len(rawBytes) / 2
	samples := make([]int16, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(rawBytes[i*2 : i*2+2]))
	}

	vpSvc := services.GetVoiceprint()
	if vpSvc == nil {
		utils.ServerError(c, "声纹识别引擎未初始化")
		return
	}

	emb, err := vpSvc.ExtractEmbedding(samples)
	if err != nil {
		utils.BadRequest(c, "提取声纹特征失败(请确保录音时长足够长，如 3 秒以上且说话清晰): "+err.Error())
		return
	}

	// 读取现有配置
	var existing models.ModelConfig
	err = database.DB.First(&existing).Error
	if err != nil {
		utils.ServerError(c, "未初始化默认配置: "+err.Error())
		return
	}

	// 解析已有的声纹向量数组 (多条声纹存储格式: [][]float32)
	var allEmbs [][]float32
	if existing.MasterVoiceprint != "" {
		if err := json.Unmarshal([]byte(existing.MasterVoiceprint), &allEmbs); err != nil {
			// 兼容旧的单条格式 []float32 → 自动升级为 [][]float32
			var singleEmb []float32
			if err2 := json.Unmarshal([]byte(existing.MasterVoiceprint), &singleEmb); err2 == nil && len(singleEmb) > 0 {
				allEmbs = [][]float32{singleEmb}
			}
		}
	}

	// 限制最多 10 条声纹采样
	if len(allEmbs) >= 10 {
		utils.BadRequest(c, "已达到最大声纹采样数量上限 (10 条)，请先删除部分旧声纹再添加")
		return
	}

	// 追加新声纹
	allEmbs = append(allEmbs, emb)

	// 序列化为 JSON 字符串
	embBytes, err := json.Marshal(allEmbs)
	if err != nil {
		utils.ServerError(c, "声纹特征序列化失败: "+err.Error())
		return
	}

	existing.MasterVoiceprint = string(embBytes)
	err = database.DB.Save(&existing).Error
	if err != nil {
		utils.ServerError(c, "保存声纹特征到数据库失败: "+err.Error())
		return
	}

	utils.Success(c, gin.H{
		"total_count": len(allEmbs),
	}, fmt.Sprintf("声纹采样录入成功！当前共 %d 条声纹建档", len(allEmbs)))
}

// DeleteVoiceprint 处理 DELETE /api/config/voiceprint/:index
// 删除指定索引的声纹采样
func (ctrl *ConfigController) DeleteVoiceprint(c *gin.Context) {
	indexStr := c.Param("index")
	var idx int
	if _, err := fmt.Sscanf(indexStr, "%d", &idx); err != nil {
		utils.BadRequest(c, "无效的声纹索引")
		return
	}

	var existing models.ModelConfig
	if err := database.DB.First(&existing).Error; err != nil {
		utils.ServerError(c, "未初始化默认配置: "+err.Error())
		return
	}

	var allEmbs [][]float32
	if existing.MasterVoiceprint != "" {
		if err := json.Unmarshal([]byte(existing.MasterVoiceprint), &allEmbs); err != nil {
			utils.ServerError(c, "解析声纹数据失败: "+err.Error())
			return
		}
	}

	if idx < 0 || idx >= len(allEmbs) {
		utils.BadRequest(c, "声纹索引超出范围")
		return
	}

	// 移除指定索引
	allEmbs = append(allEmbs[:idx], allEmbs[idx+1:]...)

	embBytes, _ := json.Marshal(allEmbs)
	existing.MasterVoiceprint = string(embBytes)
	if len(allEmbs) == 0 {
		existing.MasterVoiceprint = ""
	}

	if err := database.DB.Save(&existing).Error; err != nil {
		utils.ServerError(c, "保存失败: "+err.Error())
		return
	}

	utils.Success(c, gin.H{
		"total_count": len(allEmbs),
	}, fmt.Sprintf("已删除第 %d 条声纹，剩余 %d 条", idx+1, len(allEmbs)))
}


