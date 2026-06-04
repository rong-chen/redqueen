package controllers

import (
	"RedQueenSystem/database"
	"RedQueenSystem/models"
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
	existing.Voice = input.Voice
	existing.SystemPrompt = input.SystemPrompt
	existing.SystemRole = input.SystemRole
	existing.SystemPersonality = input.SystemPersonality

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
