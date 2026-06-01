package controllers

import (
	"strconv"

	"RedQueenSystem/services"
	"RedQueenSystem/utils"

	"github.com/gin-gonic/gin"
)

// VoiceController 负责接收并响应语音控制相关的 HTTP 请求
type VoiceController struct {
	voiceService *services.VoiceService
}

// NewVoiceController 实例化语音控制器
func NewVoiceController() *VoiceController {
	return &VoiceController{
		voiceService: services.NewVoiceService(),
	}
}

// CreateVoiceInput 定义了新增语音指令所需的 JSON 输入结构
type CreateVoiceInput struct {
	AudioPath  string  `json:"audio_path"`                  // 录音文件地址
	Transcript string  `json:"transcript" binding:"required"` // 识别文本结果 (必填)
	Confidence float64 `json:"confidence"`                  // 识别置信度
}

// CreateCommand 处理 POST /api/voice/command - 记录新语音指令并激活意图分析
func (ctrl *VoiceController) CreateCommand(c *gin.Context) {
	var input CreateVoiceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.BadRequest(c, "参数绑定失败: "+err.Error())
		return
	}

	command, err := ctrl.voiceService.CreateVoiceCommand(input.AudioPath, input.Transcript, input.Confidence)
	if err != nil {
		utils.ServerError(c, "创建语音指令失败: "+err.Error())
		return
	}

	utils.Created(c, command, "语音指令创建成功，意图处理已在后台运行")
}

// GetHistory 处理 GET /api/voice/history - 获取语音请求日志历史记录
func (ctrl *VoiceController) GetHistory(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}

	history, err := ctrl.voiceService.GetVoiceHistory(limit)
	if err != nil {
		utils.ServerError(c, "获取历史日志失败: "+err.Error())
		return
	}

	utils.Success(c, history, "获取历史指令列表成功")
}
