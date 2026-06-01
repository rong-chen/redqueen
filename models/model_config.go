package models

import "gorm.io/gorm"

// ModelConfig 存储语音网关所用的大语言模型 (如 DeepSeek) 参数配置
type ModelConfig struct {
	gorm.Model
	ActiveMode   string `gorm:"size:50;default:'local'" json:"active_mode"`    // 激活模式: deepseek (大模型解析) 或 local (本地规则匹配)
	ApiKey       string `gorm:"size:255" json:"api_key"`                       // DeepSeek API 秘钥
	ApiURL       string `gorm:"size:255;default:'https://api.deepseek.com/v1/chat/completions'" json:"api_url"` // API 端点 URL
	ModelName    string `gorm:"size:100;default:'deepseek-chat'" json:"model_name"` // 模型名称
	SystemPrompt string `gorm:"type:text" json:"system_prompt"`                 // 指引大模型的 System Prompt
	SystemRole        string `gorm:"size:100;default:'红皇后'" json:"system_role"` // 角色指定 (大模型将以此角色响应)
	SystemPersonality string `gorm:"size:150;default:'符合皇后的语气'" json:"system_personality"` // 性格指定 (大模型将以此性格特点进行表达)
}
