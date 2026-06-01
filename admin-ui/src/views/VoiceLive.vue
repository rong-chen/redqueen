<template>
  <div class="voice-live-container">
    <!-- 语音交互主控面板 -->
    <el-card class="voice-main-card" :class="{ 'card-active': isActive, 'card-sleeping': !isActive }">
      <template #header>
        <div class="card-header">
          <span class="card-title">
            <span class="status-dot" :class="statusDotClass"></span>
            {{ statusTitle }}
          </span>
          <el-tag :type="isConnected ? 'success' : 'danger'" size="small">
            {{ isConnected ? 'WebSocket 已连接' : 'WebSocket 未连接' }}
          </el-tag>
        </div>
      </template>

      <!-- 语音波形可视化区域 -->
      <div class="voice-visualizer" :class="{ 'visualizer-active': isRecording }">
        <canvas ref="canvasRef" class="waveform-canvas"></canvas>
        <div v-if="!isRecording && !isConnected" class="visualizer-placeholder">
          <el-icon :size="48" color="#909399"><Microphone /></el-icon>
          <p>点击下方按钮开始语音交互</p>
        </div>
        <div v-if="isRecording" class="recording-indicator">
          <span class="pulse-ring"></span>
          <span class="recording-text">正在聆听...</span>
        </div>
      </div>

      <!-- 实时识别文字显示 -->
      <div class="recognition-text-area">
        <div class="recognition-label">实时识别:</div>
        <div class="recognition-content" :class="{ 'content-partial': !isFinal }">
          {{ currentText || '等待语音输入...' }}
        </div>
      </div>

      <!-- 控制按钮组 -->
      <div class="control-buttons">
        <el-button
          v-if="!isConnected"
          type="primary"
          size="large"
          round
          :icon="Microphone"
          @click="startVoice"
          class="main-btn"
        >
          连接并开始录音
        </el-button>

        <template v-else>
          <el-button
            :type="isRecording ? 'danger' : 'success'"
            size="large"
            round
            @click="toggleRecording"
            class="main-btn"
          >
            <el-icon :size="20" style="margin-right: 6px">
              <component :is="isRecording ? 'VideoPause' : 'Microphone'" />
            </el-icon>
            {{ isRecording ? '暂停录音' : '继续录音' }}
          </el-button>

          <el-button
            type="info"
            size="large"
            round
            @click="stopVoice"
            class="stop-btn"
          >
            断开连接
          </el-button>
        </template>
      </div>
    </el-card>

    <!-- 交互消息时间线 -->
    <el-card class="message-card">
      <template #header>
        <div class="card-header header-between">
          <span class="card-title" style="display: flex; align-items: center; gap: 6px;">
            <el-icon><ChatLineRound /></el-icon>
            <span>交互消息记录</span>
          </span>
          <div style="display: flex; align-items: center; gap: 15px;">
            <div class="voice-selector" style="display: flex; align-items: center; gap: 6px;" v-if="availableVoices.length > 0">
              <span style="font-size: 12px; color: #909399;">系统音色:</span>
              <el-select v-model="selectedVoiceName" size="small" placeholder="选择音色" style="width: 160px;">
                <el-option
                  v-for="voice in availableVoices"
                  :key="voice.name"
                  :label="voice.name.replace('Microsoft', '微软').replace('Google', '谷歌')"
                  :value="voice.name"
                ></el-option>
              </el-select>
            </div>
            <el-button size="small" @click="messages = []">清空</el-button>
          </div>
        </div>
      </template>

      <div class="message-list" ref="messageListRef">
        <div v-if="messages.length === 0" class="empty-messages">
          <el-empty description="暂无交互消息" :image-size="80" />
        </div>
        <TransitionGroup name="msg" tag="div">
          <div
            v-for="(msg, index) in messages"
            :key="index"
            class="message-item"
            :class="'msg-' + msg.type"
          >
            <div class="msg-icon">{{ getMessageIcon(msg.type) }}</div>
            <div class="msg-body">
              <div class="msg-content">
                <span v-if="msg.text" class="msg-text">{{ msg.text }}</span>
                <span v-if="msg.message" class="msg-desc">{{ msg.message }}</span>
                <el-tag
                  v-if="msg.intent"
                  size="small"
                  :type="msg.status === 'success' ? 'success' : 'danger'"
                  style="margin-left: 8px"
                >
                  {{ msg.intent }} | {{ msg.status }}
                </el-tag>
              </div>
              <div class="msg-time">{{ msg.time }}</div>
            </div>
          </div>
        </TransitionGroup>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick, computed } from 'vue';
import { Microphone, VideoPause, ChatLineRound } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';

// ---------------------------------------------------------------------------
// 状态
// ---------------------------------------------------------------------------
const isConnected = ref(false);
const isRecording = ref(false);
const isActive = ref(false);      // 皇后是否被唤醒
const currentText = ref('');       // 实时识别文字
const isFinal = ref(false);       // 当前文字是否为最终结果
const messages = ref([]);          // 交互消息历史
const availableVoices = ref([]);
const selectedVoiceName = ref('');

function loadVoices() {
  if (typeof window !== 'undefined' && window.speechSynthesis) {
    const allVoices = window.speechSynthesis.getVoices();
    // 过滤中文人声
    availableVoices.value = allVoices.filter(v => v.lang.startsWith('zh') || v.lang.includes('ZH'));
    
    if (availableVoices.value.length > 0 && !selectedVoiceName.value) {
      const defaultVoice = availableVoices.value.find(v => 
        v.name.includes('Siri') || v.name.includes('Tingting') || v.name.includes('Xiaoxiao') || v.name.includes('Google') || v.name.includes('Microsoft')
      ) || availableVoices.value[0];
      selectedVoiceName.value = defaultVoice.name;
    }
  }
}

onMounted(() => {
  loadVoices();
  if (typeof window !== 'undefined' && window.speechSynthesis) {
    window.speechSynthesis.onvoiceschanged = loadVoices;
  }
});

const canvasRef = ref(null);
const messageListRef = ref(null);

let ws = null;                      // WebSocket 实例
let audioContext = null;            // Web Audio API 上下文
let mediaStream = null;             // 麦克风媒体流
let scriptProcessor = null;        // 音频处理节点
let source = null;                  // 音频源节点
let analyser = null;                // 频谱分析节点
let animationId = null;             // 波形动画帧 ID

// ---------------------------------------------------------------------------
// 计算属性
// ---------------------------------------------------------------------------
const statusTitle = computed(() => {
  if (!isConnected.value) return '未连接';
  if (isActive.value) return '红皇后已唤醒 — 请说出您的指令';
  return '红皇后休眠中 — 请说 "皇后" 唤醒';
});

const statusDotClass = computed(() => ({
  'dot-active': isConnected.value && isActive.value,
  'dot-sleeping': isConnected.value && !isActive.value,
  'dot-disconnected': !isConnected.value,
}));

// ---------------------------------------------------------------------------
// WebSocket 连接
// ---------------------------------------------------------------------------
function getWSUrl() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  // 开发环境直连后端
  return `${protocol}//localhost:9091/api/voice/ws`;
}

function connectWebSocket() {
  return new Promise((resolve, reject) => {
    const url = getWSUrl();
    ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      isConnected.value = true;
      addMessage('system', { message: 'WebSocket 连接成功' });
      resolve();
    };

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        handleServerMessage(JSON.parse(event.data));
      }
    };

    ws.onclose = () => {
      isConnected.value = false;
      isActive.value = false;
      isRecording.value = false;
      addMessage('system', { message: 'WebSocket 连接已断开' });
    };

    ws.onerror = (err) => {
      console.error('WebSocket error:', err);
      ElMessage.error('WebSocket 连接失败，请检查后端服务是否运行');
      reject(err);
    };
  });
}

// ---------------------------------------------------------------------------
// 服务端消息处理
// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------
// 网页本地 TTS 语音播放 (Web Speech Synthesis)
// ---------------------------------------------------------------------------
function speakText(text) {
  if (!text) return;
  try {
    // 立即取消当前正在播放的音频，避免重叠
    window.speechSynthesis.cancel();

    const utterance = new SpeechSynthesisUtterance(text);
    utterance.lang = 'zh-CN';
    utterance.rate = 1.1;  // 略微加快语速，听起来更自然
    utterance.pitch = 1.0; // 标准音调

    const voices = window.speechSynthesis.getVoices();
    // 使用用户选择的音色，如果不存在则使用默认逻辑
    const targetVoice = voices.find(v => v.name === selectedVoiceName.value);
    if (targetVoice) {
      utterance.voice = targetVoice;
    } else {
      const fallbackVoice = voices.find(v => v.lang.startsWith('zh'));
      if (fallbackVoice) utterance.voice = fallbackVoice;
    }

    window.speechSynthesis.speak(utterance);
  } catch (err) {
    console.error('TTS 播放失败:', err);
  }
}

// 首次唤起加载系统声库，防止首句播放无声音
if (typeof window !== 'undefined' && window.speechSynthesis) {
  window.speechSynthesis.getVoices();
}

function handleServerMessage(msg) {
  switch (msg.type) {
    case 'ready':
      addMessage('system', msg);
      break;

    case 'partial':
      currentText.value = msg.text;
      isFinal.value = false;
      break;

    case 'final':
      currentText.value = msg.text;
      isFinal.value = true;
      addMessage('final', msg);
      break;

    case 'wake':
      isActive.value = true;
      addMessage('wake', msg);
      speakText(msg.message); // 播报唤醒温馨语
      break;

    case 'sleep':
      isActive.value = false;
      addMessage('sleep', msg);
      speakText(msg.message); // 播报休眠温馨语
      break;

    case 'result':
      addMessage('result', msg);
      speakText(msg.message); // 播报大模型执行结果或问候
      break;

    case 'pong':
      break;

    default:
      addMessage('system', msg);
  }
}

// ---------------------------------------------------------------------------
// 麦克风采集与音频处理
// ---------------------------------------------------------------------------
async function startMicrophone() {
  try {
    mediaStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        sampleRate: 16000,
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
      },
    });

    audioContext = new (window.AudioContext || window.webkitAudioContext)({
      sampleRate: 16000,
    });

    source = audioContext.createMediaStreamSource(mediaStream);
    analyser = audioContext.createAnalyser();
    analyser.fftSize = 256;
    source.connect(analyser);

    // 使用 ScriptProcessorNode 捕获 PCM 数据
    // bufferSize = 4096 对应约 256ms 的音频块 (16kHz)
    scriptProcessor = audioContext.createScriptProcessor(4096, 1, 1);

    scriptProcessor.onaudioprocess = (event) => {
      if (!isRecording.value || !ws || ws.readyState !== WebSocket.OPEN) return;

      const inputData = event.inputBuffer.getChannelData(0); // Float32 [-1, 1]

      // 转换为 Int16 PCM
      const pcmData = new Int16Array(inputData.length);
      for (let i = 0; i < inputData.length; i++) {
        const s = Math.max(-1, Math.min(1, inputData[i]));
        pcmData[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
      }

      // 通过 WebSocket 发送二进制音频帧
      ws.send(pcmData.buffer);
    };

    source.connect(scriptProcessor);
    scriptProcessor.connect(audioContext.destination);

    // 启动波形可视化
    startWaveformAnimation();

    isRecording.value = true;
  } catch (err) {
    console.error('麦克风获取失败:', err);
    ElMessage.error('无法获取麦克风权限，请在浏览器中允许麦克风访问');
    throw err;
  }
}

function stopMicrophone() {
  isRecording.value = false;

  if (animationId) {
    cancelAnimationFrame(animationId);
    animationId = null;
  }

  if (scriptProcessor) {
    scriptProcessor.disconnect();
    scriptProcessor = null;
  }

  if (source) {
    source.disconnect();
    source = null;
  }

  if (analyser) {
    analyser.disconnect();
    analyser = null;
  }

  if (audioContext) {
    audioContext.close();
    audioContext = null;
  }

  if (mediaStream) {
    mediaStream.getTracks().forEach(track => track.stop());
    mediaStream = null;
  }

  // 清空画布
  const canvas = canvasRef.value;
  if (canvas) {
    const ctx = canvas.getContext('2d');
    ctx.clearRect(0, 0, canvas.width, canvas.height);
  }
}

// ---------------------------------------------------------------------------
// 波形可视化
// ---------------------------------------------------------------------------
function startWaveformAnimation() {
  const canvas = canvasRef.value;
  if (!canvas || !analyser) return;

  const ctx = canvas.getContext('2d');
  const bufferLength = analyser.frequencyBinCount;
  const dataArray = new Uint8Array(bufferLength);

  function draw() {
    animationId = requestAnimationFrame(draw);

    // 自适应画布大小
    canvas.width = canvas.offsetWidth * (window.devicePixelRatio || 1);
    canvas.height = canvas.offsetHeight * (window.devicePixelRatio || 1);
    ctx.scale(window.devicePixelRatio || 1, window.devicePixelRatio || 1);

    const WIDTH = canvas.offsetWidth;
    const HEIGHT = canvas.offsetHeight;

    analyser.getByteFrequencyData(dataArray);

    // 渐变背景
    ctx.fillStyle = 'rgba(15, 15, 30, 0.85)';
    ctx.fillRect(0, 0, WIDTH, HEIGHT);

    const barWidth = (WIDTH / bufferLength) * 2.5;
    let x = 0;

    for (let i = 0; i < bufferLength; i++) {
      const barHeight = (dataArray[i] / 255) * HEIGHT * 0.8;

      // 渐变色条
      const hue = (i / bufferLength) * 120 + (isActive.value ? 120 : 220);
      const saturation = 80;
      const lightness = 50 + (dataArray[i] / 255) * 20;

      ctx.fillStyle = `hsl(${hue}, ${saturation}%, ${lightness}%)`;

      // 圆角柱状
      const barY = HEIGHT - barHeight;
      const radius = Math.min(barWidth / 2, 3);
      ctx.beginPath();
      ctx.moveTo(x + radius, barY);
      ctx.lineTo(x + barWidth - radius, barY);
      ctx.quadraticCurveTo(x + barWidth, barY, x + barWidth, barY + radius);
      ctx.lineTo(x + barWidth, HEIGHT);
      ctx.lineTo(x, HEIGHT);
      ctx.lineTo(x, barY + radius);
      ctx.quadraticCurveTo(x, barY, x + radius, barY);
      ctx.fill();

      x += barWidth + 1;
    }
  }

  draw();
}

// ---------------------------------------------------------------------------
// 控制方法
// ---------------------------------------------------------------------------
async function startVoice() {
  try {
    await connectWebSocket();
    await startMicrophone();
    ElMessage.success('语音连接已建立，开始录音');
  } catch (err) {
    // 如果麦克风失败，也关闭 WS
    if (ws) ws.close();
  }
}

function toggleRecording() {
  if (isRecording.value) {
    // 暂停录音但保持 WS 连接
    isRecording.value = false;
    addMessage('system', { message: '录音已暂停' });
  } else {
    isRecording.value = true;
    addMessage('system', { message: '录音已恢复' });
  }
}

function stopVoice() {
  stopMicrophone();
  if (ws) {
    ws.close();
    ws = null;
  }
  isConnected.value = false;
  isActive.value = false;
  currentText.value = '';
}

// ---------------------------------------------------------------------------
// 消息管理
// ---------------------------------------------------------------------------
function addMessage(type, msg) {
  const now = new Date();
  const time = now.toLocaleTimeString('zh-CN', { hour12: false });

  messages.value.push({
    type,
    text: msg.text || '',
    message: msg.message || '',
    intent: msg.intent || '',
    status: msg.status || '',
    time,
  });

  // 自动滚动到底部
  nextTick(() => {
    const list = messageListRef.value;
    if (list) {
      list.scrollTop = list.scrollHeight;
    }
  });
}

function getMessageIcon(type) {
  const icons = {
    system: '[系统]',
    final: '[指令]',
    wake: '[唤醒]',
    sleep: '[休眠]',
    result: '[执行]',
    error: '[异常]',
  };
  return icons[type] || '[消息]';
}

// ---------------------------------------------------------------------------
// 组件卸载清理
// ---------------------------------------------------------------------------
onUnmounted(() => {
  stopVoice();
});
</script>

<style scoped>
.voice-live-container {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 主控面板卡片 */
.voice-main-card {
  border-left: 5px solid #909399;
  transition: border-color 0.4s ease, box-shadow 0.4s ease;
}
.voice-main-card.card-active {
  border-left-color: #67c23a;
  box-shadow: 0 0 20px rgba(103, 194, 58, 0.15);
}
.voice-main-card.card-sleeping {
  border-left-color: #e6a23c;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.card-title {
  font-weight: bold;
  font-size: 15px;
  color: #303133;
  display: flex;
  align-items: center;
  gap: 8px;
}
.header-between {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

/* 状态圆点 */
.status-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  display: inline-block;
}
.dot-active {
  background: #67c23a;
  box-shadow: 0 0 8px #67c23a;
  animation: pulse-green 1.5s infinite;
}
.dot-sleeping {
  background: #e6a23c;
  animation: pulse-orange 2s infinite;
}
.dot-disconnected {
  background: #909399;
}

@keyframes pulse-green {
  0%, 100% { box-shadow: 0 0 4px #67c23a; }
  50% { box-shadow: 0 0 12px #67c23a; }
}
@keyframes pulse-orange {
  0%, 100% { box-shadow: 0 0 4px #e6a23c; }
  50% { box-shadow: 0 0 10px #e6a23c; }
}

/* 波形可视化 */
.voice-visualizer {
  position: relative;
  height: 140px;
  background: linear-gradient(135deg, #0f0f1e 0%, #1a1a2e 100%);
  border-radius: 12px;
  overflow: hidden;
  margin-bottom: 16px;
}
.waveform-canvas {
  width: 100%;
  height: 100%;
  display: block;
}
.visualizer-placeholder {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #909399;
  gap: 8px;
}
.visualizer-placeholder p {
  margin: 0;
  font-size: 14px;
}

.recording-indicator {
  position: absolute;
  top: 10px;
  right: 14px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.pulse-ring {
  width: 12px;
  height: 12px;
  background: #f56c6c;
  border-radius: 50%;
  animation: pulse-rec 1s infinite;
}
@keyframes pulse-rec {
  0% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.4); opacity: 0.6; }
  100% { transform: scale(1); opacity: 1; }
}
.recording-text {
  color: #f56c6c;
  font-size: 13px;
  font-weight: 500;
}

/* 识别文字区域 */
.recognition-text-area {
  background: #f5f7fa;
  border-radius: 8px;
  padding: 12px 16px;
  margin-bottom: 16px;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-height: 44px;
}
.recognition-label {
  color: #909399;
  font-size: 13px;
  white-space: nowrap;
  padding-top: 2px;
}
.recognition-content {
  font-size: 16px;
  color: #303133;
  font-weight: 500;
  line-height: 1.5;
  transition: color 0.3s;
}
.recognition-content.content-partial {
  color: #909399;
  font-style: italic;
}

/* 控制按钮 */
.control-buttons {
  display: flex;
  justify-content: center;
  gap: 16px;
}
.main-btn {
  min-width: 180px;
  font-size: 15px;
  font-weight: 600;
  height: 46px;
}
.stop-btn {
  min-width: 120px;
  height: 46px;
}

/* 消息列表 */
.message-card {
  max-height: 400px;
}
.message-list {
  max-height: 300px;
  overflow-y: auto;
  padding-right: 4px;
}
.empty-messages {
  padding: 20px 0;
}

.message-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 8px;
  margin-bottom: 6px;
  transition: background 0.2s;
}
.message-item:hover {
  background: #f5f7fa;
}

.msg-icon {
  font-size: 18px;
  flex-shrink: 0;
  padding-top: 1px;
}
.msg-body {
  flex: 1;
  min-width: 0;
}
.msg-content {
  font-size: 14px;
  color: #303133;
  word-break: break-all;
}
.msg-text {
  font-weight: 500;
}
.msg-desc {
  color: #606266;
}
.msg-time {
  font-size: 12px;
  color: #c0c4cc;
  margin-top: 2px;
}

/* 消息类型颜色 */
.msg-wake {
  background: linear-gradient(90deg, rgba(103,194,58,0.08) 0%, transparent 100%);
  border-left: 3px solid #67c23a;
}
.msg-sleep {
  background: linear-gradient(90deg, rgba(230,162,60,0.08) 0%, transparent 100%);
  border-left: 3px solid #e6a23c;
}
.msg-result {
  background: linear-gradient(90deg, rgba(64,158,255,0.08) 0%, transparent 100%);
  border-left: 3px solid #409eff;
}
.msg-final {
  border-left: 3px solid #909399;
}
.msg-system {
  opacity: 0.7;
  font-size: 13px;
}

/* 消息动画 */
.msg-enter-active {
  transition: all 0.3s ease;
}
.msg-enter-from {
  opacity: 0;
  transform: translateY(10px);
}
</style>
