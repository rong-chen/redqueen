package services

import (
	"errors"

	"RedQueenSystem/database"
	"RedQueenSystem/models"
)

// VoiceService 提供语音处理与意图解析服务
type VoiceService struct{}

// NewVoiceService 实例化语音服务
func NewVoiceService() *VoiceService {
	return &VoiceService{}
}

// CreateVoiceCommand 保存一条新的语音指令记录
func (s *VoiceService) CreateVoiceCommand(audioPath, transcript string, confidence float64) (*models.VoiceCommand, error) {
	if transcript == "" {
		return nil, errors.New("识别文字内容不能为空")
	}

	command := &models.VoiceCommand{
		AudioPath:  audioPath,
		Transcript: transcript,
		Confidence: confidence,
		Status:     "success",
	}

	// 保存记录到数据库
	if err := database.DB.Create(command).Error; err != nil {
		return nil, err
	}

	return command, nil
}

// GetVoiceHistory 获取历史语音指令记录
func (s *VoiceService) GetVoiceHistory(limit int) ([]models.VoiceCommand, error) {
	var history []models.VoiceCommand
	err := database.DB.Order("created_at desc").Limit(limit).Find(&history).Error
	return history, err
}
