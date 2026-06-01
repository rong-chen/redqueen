package services

import (
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// 会话状态定义
// ---------------------------------------------------------------------------

// SessionState 会话状态枚举
type SessionState int

const (
	// StateSleeping 休眠态：只监听唤醒词，忽略其他语音
	StateSleeping SessionState = iota
	// StateActive 激活态：将识别到的语音作为指令处理
	StateActive
)

// ActionType 会话动作类型
type ActionType int

const (
	// ActionIgnore 忽略（休眠态中未检测到唤醒词）
	ActionIgnore ActionType = iota
	// ActionWake 唤醒成功但无附带指令，等待后续输入
	ActionWake
	// ActionWakeAndExecute 唤醒成功且附带了指令（如 "皇后帮我开灯"）
	ActionWakeAndExecute
	// ActionExecute 激活态中收到指令，直接执行
	ActionExecute
	// ActionSleep 用户主动结束对话
	ActionSleep
	// ActionTimeout 超时自动休眠
	ActionTimeout
)

// SessionAction 会话状态机的处理结果
type SessionAction struct {
	Type    ActionType // 动作类型
	Command string     // 需要发送给 NLP 的指令文本（仅 Execute/WakeAndExecute 时有值）
}

// ---------------------------------------------------------------------------
// Session 会话管理器
// ---------------------------------------------------------------------------

// Session 单个连接的会话状态机
type Session struct {
	mu             sync.Mutex
	State          SessionState
	WakeWord       string        // 唤醒词，默认 "皇后"
	Timeout        time.Duration // 激活态超时时长
	LastActiveTime time.Time     // 上次活跃时间戳
	RoomID         string        // 房间 ID
	MaxVolume      float64       // 最近音频的平滑最大音量
}

// 结束对话的关键词列表
var sleepKeywords = []string{
	"好了", "谢谢", "没事了", "再见", "拜拜",
	"谢谢皇后", "好的谢谢", "不用了", "退下",
}

// NewSession 创建新的会话状态机实例
func NewSession(wakeWord string, timeout time.Duration, roomID string) *Session {
	if wakeWord == "" {
		wakeWord = "皇后"
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Session{
		State:    StateSleeping,
		WakeWord: wakeWord,
		Timeout:  timeout,
		RoomID:   roomID,
	}
}

// UpdateVolume 平滑更新当前会话的音量（使用指数衰减平滑法）
func (s *Session) UpdateVolume(vol float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 音量平滑滤波：80% 历史 + 20% 当前
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

// ForceSleep 强制将会话重置为休眠状态
func (s *Session) ForceSleep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = StateSleeping
}

// ProcessText 处理一句识别完成的文字，返回应执行的动作
//
// 状态转换逻辑:
//
//	休眠 + 包含唤醒词 → 激活（提取唤醒词后的内容作为指令）
//	休眠 + 无唤醒词   → 忽略
//	激活 + 结束关键词 → 休眠
//	激活 + 普通文字   → 作为指令执行
func (s *Session) ProcessText(text string) SessionAction {
	s.mu.Lock()
	defer s.mu.Unlock()

	text = strings.TrimSpace(text)
	if text == "" {
		return SessionAction{Type: ActionIgnore}
	}

	switch s.State {
	case StateSleeping:
		return s.handleSleepingState(text)
	case StateActive:
		return s.handleActiveState(text)
	default:
		return SessionAction{Type: ActionIgnore}
	}
}

// CheckTimeout 检查激活态是否已超时，如果超时则自动切换到休眠态
// 返回 true 表示刚刚发生了超时切换
func (s *Session) CheckTimeout() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != StateActive {
		return false
	}

	if time.Since(s.LastActiveTime) > s.Timeout {
		s.State = StateSleeping
		return true
	}
	return false
}

// GetState 获取当前会话状态（线程安全）
func (s *Session) GetState() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.State
}

// ---------------------------------------------------------------------------
// 内部状态处理方法
// ---------------------------------------------------------------------------

// handleSleepingState 休眠态下的文字处理
func (s *Session) handleSleepingState(text string) SessionAction {
	// 检查是否包含唤醒词
	wakeIdx := strings.Index(text, s.WakeWord)
	if wakeIdx < 0 {
		return SessionAction{Type: ActionIgnore}
	}

	// 唤醒成功，切换到激活态
	s.State = StateActive
	s.LastActiveTime = time.Now()

	// 提取唤醒词之后的内容作为指令
	afterWake := strings.TrimSpace(text[wakeIdx+len(s.WakeWord):])

	// 清理常见的唤醒前缀词（如 "嗨"、"hi"、"你好" 等）
	afterWake = cleanCommand(afterWake)

	if afterWake != "" {
		return SessionAction{
			Type:    ActionWakeAndExecute,
			Command: afterWake,
		}
	}

	return SessionAction{Type: ActionWake}
}

// handleActiveState 激活态下的文字处理
func (s *Session) handleActiveState(text string) SessionAction {
	// 刷新活跃时间
	s.LastActiveTime = time.Now()

	// 检查是否为结束对话指令
	if isSleepCommand(text) {
		s.State = StateSleeping
		return SessionAction{Type: ActionSleep}
	}

	// 如果又提到了唤醒词，提取唤醒词后面的内容
	if wakeIdx := strings.Index(text, s.WakeWord); wakeIdx >= 0 {
		afterWake := strings.TrimSpace(text[wakeIdx+len(s.WakeWord):])
		afterWake = cleanCommand(afterWake)
		if afterWake != "" {
			return SessionAction{
				Type:    ActionExecute,
				Command: afterWake,
			}
		}
		// 只是又叫了一声，继续保持激活
		return SessionAction{Type: ActionIgnore}
	}

	// 普通指令，直接发给 NLP
	return SessionAction{
		Type:    ActionExecute,
		Command: text,
	}
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// isSleepCommand 判断文字是否为结束对话的指令
func isSleepCommand(text string) bool {
	text = strings.TrimSpace(text)
	for _, keyword := range sleepKeywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// cleanCommand 清理指令文本中的无意义前缀词
func cleanCommand(text string) string {
	// 移除常见的口语化连接词前缀
	prefixes := []string{"请", "帮我", "帮忙", "麻烦", "可以"}
	result := text
	for _, prefix := range prefixes {
		if strings.HasPrefix(result, prefix) {
			trimmed := strings.TrimPrefix(result, prefix)
			// 只有剩余内容非空才去掉前缀，避免 "帮我" 被清空
			if strings.TrimSpace(trimmed) != "" {
				result = strings.TrimSpace(trimmed)
			}
			break // 只去一层前缀
		}
	}
	return result
}
