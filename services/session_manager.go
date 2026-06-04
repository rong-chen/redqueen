package services

import (
	"strings"
	"sync"
)

// ActionType 会话动作类型
type ActionType int

const (
	// ActionIgnore 忽略
	ActionIgnore ActionType = iota
	// ActionExecute 收到指令，直接执行
	ActionExecute
)

// SessionAction 会话状态机的处理结果
type SessionAction struct {
	Type    ActionType // 动作类型
	Command string     // 需要发送给 NLP 的指令文本
}

// Session 单个连接的会话状态
type Session struct {
	mu         sync.Mutex
	RoomID     string  // 房间 ID
	MaxVolume  float64 // 最近音频的平滑最大音量
	IsSpeaking bool    // 前端是否正在播放语音
}

// NewSession 创建新的会话实例
func NewSession(wakeWord string, timeout interface{}, roomID string) *Session {
	return &Session{
		RoomID: roomID,
	}
}

// UpdateVolume 平滑更新当前会话的音量（使用指数衰减平滑法）
func (s *Session) UpdateVolume(vol float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MaxVolume = s.MaxVolume*0.8 + vol*0.2
}

// GetMaxVolume 获取当前平滑音量（线程安全）
func (s *Session) GetMaxVolume() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.MaxVolume
}

// ResetVolume 重置平滑音量
func (s *Session) ResetVolume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MaxVolume = 0
}

// ProcessText 处理一句识别完成的文字，返回应执行的动作
func (s *Session) ProcessText(text string) SessionAction {
	s.mu.Lock()
	defer s.mu.Unlock()

	text = strings.TrimSpace(text)
	if text == "" {
		return SessionAction{Type: ActionIgnore}
	}

	return SessionAction{
		Type:    ActionExecute,
		Command: text,
	}
}

// IsSpeakingGetter 获取前端是否正在播放语音（线程安全）
func (s *Session) IsSpeakingGetter() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.IsSpeaking
}

// SetSpeaking 设置前端朗读/播放状态（线程安全）
func (s *Session) SetSpeaking(val bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsSpeaking = val
}
