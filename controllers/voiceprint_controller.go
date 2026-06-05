package controllers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"RedQueenSystem/database"
	"RedQueenSystem/models"
	"RedQueenSystem/services"
	"RedQueenSystem/utils"

	"github.com/gin-gonic/gin"
)

// VoiceprintController 负责声纹录入和管理接口
type VoiceprintController struct {
	vpService *services.VoiceprintService
}

// NewVoiceprintController 实例化声纹控制器
func NewVoiceprintController() *VoiceprintController {
	return &VoiceprintController{
		vpService: services.GetVoiceprint(),
	}
}

// EnrollVoiceprintInput 声纹注册输入
type EnrollVoiceprintInput struct {
	Samples []string `json:"samples" binding:"required,min=3,max=3"` // 3段PCM16 Base64字符串
}

// EnrollVoiceprint 处理 POST /api/voiceprint/enroll
func (ctrl *VoiceprintController) EnrollVoiceprint(c *gin.Context) {
	var input EnrollVoiceprintInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.BadRequest(c, "参数绑定失败，请确保传入 3 段录音数据: "+err.Error())
		return
	}

	if ctrl.vpService == nil {
		utils.ServerError(c, "声纹引擎尚未初始化完成")
		return
	}

	var embeddings [][]float32

	// 解析三段 base64 pcm 数据并提取特征
	for i, b64Str := range input.Samples {
		pcmBytes, err := base64.StdEncoding.DecodeString(b64Str)
		if err != nil {
			utils.BadRequest(c, fmt.Sprintf("第 %d 段音频 Base64 解码失败", i+1))
			return
		}

		if len(pcmBytes)%2 != 0 {
			utils.BadRequest(c, fmt.Sprintf("第 %d 段音频 PCM 数据长度有误(应为双字节)", i+1))
			return
		}

		// 转换成 int16
		samples := make([]int16, len(pcmBytes)/2)
		for j := 0; j < len(samples); j++ {
			samples[j] = int16(pcmBytes[j*2]) | (int16(pcmBytes[j*2+1]) << 8)
		}

		// 提取特征 (内部自带 VAD 裁剪)
		emb, err := ctrl.vpService.ExtractEmbedding(samples)
		if err != nil {
			utils.ServerError(c, fmt.Sprintf("第 %d 段音频特征提取失败: %v", i+1, err))
			return
		}

		embeddings = append(embeddings, emb)
	}

	// 计算平均声纹
	avgEmbedding, err := ctrl.vpService.AverageEmbeddings(embeddings)
	if err != nil {
		utils.ServerError(c, "声纹特征融合失败: "+err.Error())
		return
	}

	// 序列化平均声纹
	avgBytes, err := json.Marshal(avgEmbedding)
	if err != nil {
		utils.ServerError(c, "声纹序列化失败: "+err.Error())
		return
	}

	// 将 MasterVoiceprint 存入当前系统管理员用户
	var admin models.User
	if err := database.DB.Where("role = ?", "admin").First(&admin).Error; err != nil {
		utils.ServerError(c, "未找到管理员用户，无法保存声纹")
		return
	}

	admin.MasterVoiceprint = string(avgBytes)
	if err := database.DB.Save(&admin).Error; err != nil {
		utils.ServerError(c, "保存声纹到数据库失败: "+err.Error())
		return
	}

	utils.Success(c, nil, "声纹注册成功，特征已融合保存")
}
