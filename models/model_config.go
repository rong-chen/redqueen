package models

import "gorm.io/gorm"

// ModelConfig 存储语音网关所用的大语言模型 (如 Qwen-Omni) 参数配置
type ModelConfig struct {
	gorm.Model
	ActiveMode   string `gorm:"size:50;default:'qwen-omni'" json:"active_mode"`    // 激活模式: qwen-omni 或 local 等
	ApiKey       string `gorm:"size:255" json:"api_key"`                       // API 秘钥 (例如 DashScope API Key)
	ApiURL       string `gorm:"size:255;default:'wss://dashscope.aliyuncs.com/api-ws/v1/realtime'" json:"api_url"` // API 端点 URL
	ModelName    string `gorm:"size:100;default:'qwen3.5-omni-plus-realtime'" json:"model_name"` // 模型名称
	Voice        string `gorm:"size:50;default:'Tina'" json:"voice"`                           // 音色名称 (如 Tina, Cherry, Diana, Grace)
	SystemPrompt string `gorm:"type:text" json:"system_prompt"`                 // 指引大模型的 System Prompt
	SystemRole        string `gorm:"size:100;default:'红皇后'" json:"system_role"` // 角色指定 (大模型将以此角色响应)
	SystemPersonality string `gorm:"size:150;default:'符合皇后的语气'" json:"system_personality"` // 性格指定 (大模型将以此性格特点进行表达)
}
