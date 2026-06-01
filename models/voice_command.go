package models

import "gorm.io/gorm"

// VoiceCommand 记录语音控制的交互日志，包括识别出来的文本与 NLP 意图
type VoiceCommand struct {
	gorm.Model
	AudioPath    string  `gorm:"size:255" json:"audio_path"`                   // 录音文件存放的路径
	Transcript   string  `gorm:"type:text" json:"transcript"`                  // 语音转成的文字内容 (STT 结果)
	Intent       string  `gorm:"size:100;index" json:"intent"`                 // NLP/规则匹配解析出的意图 (如 turn_on_device)
	Confidence   float64 `json:"confidence"`                                   // 语音识别的置信度得分 (0.0 - 1.0)
	Status       string  `gorm:"size:50;default:'pending'" json:"status"`      // 意图执行状态: pending (等待处理), success (成功), failed (失败)
	ErrorMessage string  `gorm:"type:text" json:"error_message,omitempty"`     // 失败时的详细错误日志
	ReplyText    string  `gorm:"type:text" json:"reply_text,omitempty"`        // 大模型生成的语音播报文本
}
