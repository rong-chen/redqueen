package services

import (
	"errors"
	"fmt"

	"RedQueenSystem/database"
	"RedQueenSystem/models"
)

// VoiceService 提供语音处理与意图解析服务
type VoiceService struct{}

// NewVoiceService 实例化语音服务
func NewVoiceService() *VoiceService {
	return &VoiceService{}
}

// CreateVoiceCommand 保存一条新的语音指令记录，并开始异步意图解析
func (s *VoiceService) CreateVoiceCommand(audioPath, transcript string, confidence float64) (*models.VoiceCommand, error) {
	if transcript == "" {
		return nil, errors.New("识别文字内容不能为空")
	}

	command := &models.VoiceCommand{
		AudioPath:  audioPath,
		Transcript: transcript,
		Confidence: confidence,
		Status:     "pending",
	}

	// 1. 保存记录到数据库
	if err := database.DB.Create(command).Error; err != nil {
		return nil, err
	}

	// 2. 异步处理解析该语音的意图，避免阻塞当前 HTTP 请求
	go s.ProcessIntent(command)

	return command, nil
}

// GetVoiceHistory 获取历史语音指令记录
func (s *VoiceService) GetVoiceHistory(limit int) ([]models.VoiceCommand, error) {
	var history []models.VoiceCommand
	err := database.DB.Order("created_at desc").Limit(limit).Find(&history).Error
	return history, err
}

// ProcessIntent 调用 DeepSeek (ds) 语义服务解析语音意图，并联动下发操作到硬件 MCP
func (s *VoiceService) ProcessIntent(command *models.VoiceCommand) {
	// 1. 调用 NLP 服务进行深度语义分析
	nlpSvc := NewNLPService()
	parseResult, err := nlpSvc.ParseIntent(command.Transcript)

	status := "success"
	var errMessage string
	var intent string
	var confidence float64

	if err != nil {
		status = "failed"
		errMessage = err.Error()
		intent = "error"
		confidence = 0.0
	} else {
		intent = parseResult.Intent
		confidence = parseResult.Confidence

		// 2. 如果是外部注册的 MCP 工具调用，则动态转发执行
		if parseResult.IsExternal {
			_, _, toolToServerMap := GetExternalMCPTools()
			srv, exists := toolToServerMap[parseResult.ExternalToolName]
			if !exists {
				status = "failed"
				errMessage = fmt.Sprintf("未在后台找到执行工具 [%s] 的在线 MCP 服务器", parseResult.ExternalToolName)
			} else {
				_, err := CallExternalMCPTool(srv, parseResult.ExternalToolName, parseResult.ExternalArguments)
				if err != nil {
					status = "failed"
					errMessage = "外部 MCP 工具执行失败: " + err.Error()
				}
			}
		} else if parseResult.Intent == "unknown" && parseResult.ReplyText == "" {
			status = "failed"
			errMessage = "无法识别或匹配对应的指令意图"
		}
	}

	// 3. 更新解析和物理控制结果至数据库交互记录中
	database.DB.Model(command).Updates(map[string]interface{}{
		"intent":        intent,
		"status":        status,
		"error_message": errMessage,
		"confidence":    confidence,
		"reply_text":    parseResult.ReplyText,
	})
}
