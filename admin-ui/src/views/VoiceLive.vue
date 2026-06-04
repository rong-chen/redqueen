<template>
  <div class="voice-live-container">
    <el-card class="voice-card">
      <div class="voice-content">
        <!-- 状态文本 -->
        <div class="status-section">
          <span class="status-dot" :class="statusDotClass"></span>
          <span class="status-text">{{ statusTitle }}</span>
        </div>

        <!-- 按钮容器 -->
        <div class="button-section">
          <div class="pulse-container" :class="{ 'pulsing': isConnected, 'speaking': isSpeaking }">
            <button 
              @click="handleConnectionToggle" 
              class="action-btn"
              :class="{ 'connected': isConnected }"
            >
              <el-icon class="btn-icon">
                <Microphone v-if="!isConnected" />
                <VideoPause v-else />
              </el-icon>
            </button>
            <div class="pulse-ring ring-1"></div>
            <div class="pulse-ring ring-2"></div>
          </div>
          <div class="btn-hint">
            {{ isConnected ? '点击断开语音信道' : '点击授权麦克风并连接' }}
          </div>
        </div>

        <!-- 对话显示区域 -->
        <div class="chat-section" v-if="isConnected && (currentText || aiReplyText)">
          <transition name="fade">
            <div class="chat-bubble user-bubble" v-if="currentText">
              <div class="bubble-title">您说：</div>
              <div class="bubble-text" :class="{ 'partial-text': !isFinal }">
                {{ currentText }}
              </div>
            </div>
          </transition>
          <transition name="fade">
            <div class="chat-bubble ai-bubble" v-if="aiReplyText">
              <div class="bubble-title">红皇后：</div>
              <div class="bubble-text">{{ aiReplyText }}</div>
            </div>
          </transition>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onUnmounted, computed } from 'vue';
import { Microphone, VideoPause } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';

// ---------------------------------------------------------------------------
// 状态与响应式变量
// ---------------------------------------------------------------------------
const isConnected = ref(false);
const isRecording = ref(false);
const currentText = ref('');       // 实时识别/转写文字
const isFinal = ref(false);        // 是否为最终转写
const isSpeaking = ref(false);      // 正在播放答复语音
const aiReplyText = ref('');       // 智能体文字回复

// ---------------------------------------------------------------------------
// 原生多媒体与 Web Audio 状态
// ---------------------------------------------------------------------------
let ws = null;
let audioContext = null;
let mediaStream = null;
let scriptProcessor = null;
let source = null;
let analyser = null;
let activeAudioSources = [];
let nextPlaybackTime = 0;

// ---------------------------------------------------------------------------
// 计算属性
// ---------------------------------------------------------------------------
const statusTitle = computed(() => {
  if (!isConnected.value) return '系统已离线';
  if (isSpeaking.value) return '红皇后播音中...';
  if (isRecording.value) return '麦克风监听中，请说话...';
  return '通道就绪，静默中';
});

const statusDotClass = computed(() => ({
  'dot-active': isConnected.value && isRecording.value,
  'dot-speaking': isConnected.value && isSpeaking.value,
  'dot-disconnected': !isConnected.value,
}));

// ---------------------------------------------------------------------------
// 连接控制与切换
// ---------------------------------------------------------------------------
async function handleConnectionToggle() {
  if (isConnected.value) {
    stopVoice();
  } else {
    await startVoice();
  }
}

async function startVoice() {
  try {
    currentText.value = '';
    aiReplyText.value = '';
    await startMicrophone();
    await connectWebSocket();
    ElMessage.success('安全语音信道激活成功');
  } catch (err) {
    if (ws) ws.close();
    stopMicrophone();
  }
}

function stopVoice() {
  stopMicrophone();
  if (ws) {
    ws.close();
    ws = null;
  }
  isConnected.value = false;
  currentText.value = '';
  aiReplyText.value = '';
}

// ---------------------------------------------------------------------------
// 音频采集与处理
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

    ensureAudioContext();

    source = audioContext.createMediaStreamSource(mediaStream);
    source.connect(analyser);

    scriptProcessor = audioContext.createScriptProcessor(4096, 1, 1);
    scriptProcessor.onaudioprocess = (event) => {
      if (!isRecording.value || !ws || ws.readyState !== WebSocket.OPEN) return;

      const inputData = event.inputBuffer.getChannelData(0);
      const pcmData = new Int16Array(inputData.length);
      for (let i = 0; i < inputData.length; i++) {
        const s = Math.max(-1, Math.min(1, inputData[i]));
        pcmData[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
      }
      ws.send(pcmData.buffer);
    };

    source.connect(scriptProcessor);
    scriptProcessor.connect(audioContext.destination);

    isRecording.value = true;
  } catch (err) {
    console.error('麦克风开启失败:', err);
    ElMessage.error('开启声音采集失败，请在浏览器中允许麦克风权限');
    throw err;
  }
}

function stopMicrophone() {
  isRecording.value = false;
  if (scriptProcessor) {
    scriptProcessor.disconnect();
    scriptProcessor = null;
  }
  if (source) {
    source.disconnect();
    source = null;
  }
  if (mediaStream) {
    mediaStream.getTracks().forEach(track => track.stop());
    mediaStream = null;
  }
  stopAllAudioPlayback();
}

function ensureAudioContext() {
  if (!audioContext) {
    audioContext = new (window.AudioContext || window.webkitAudioContext)({
      sampleRate: 16000
    });
  }
  if (!analyser) {
    analyser = audioContext.createAnalyser();
    analyser.fftSize = 256;
  }
  if (audioContext.state === 'suspended') {
    audioContext.resume();
  }
}

// ---------------------------------------------------------------------------
// 播放服务端推送的二进制 PCM 原始音频流
// ---------------------------------------------------------------------------
function playBinaryPCM(arrayBuffer) {
  try {
    ensureAudioContext();
    
    const rawData = new Int16Array(arrayBuffer);
    if (rawData.length === 0) return;
    
    const floatData = new Float32Array(rawData.length);
    for (let i = 0; i < rawData.length; i++) {
      floatData[i] = rawData[i] / 32768.0;
    }
    
    const buffer = audioContext.createBuffer(1, floatData.length, 16000);
    buffer.copyToChannel(floatData, 0);
    
    const sourceNode = audioContext.createBufferSource();
    sourceNode.buffer = buffer;
    
    sourceNode.connect(analyser);
    sourceNode.connect(audioContext.destination);
    
    const startTime = Math.max(audioContext.currentTime, nextPlaybackTime);
    sourceNode.start(startTime);
    
    nextPlaybackTime = startTime + buffer.duration;
    activeAudioSources.push(sourceNode);
    
    sourceNode.onended = () => {
      const idx = activeAudioSources.indexOf(sourceNode);
      if (idx !== -1) {
        activeAudioSources.splice(idx, 1);
      }
      if (activeAudioSources.length === 0 && isSpeaking.value) {
        setTimeout(() => {
          if (activeAudioSources.length === 0) {
            isSpeaking.value = false;
          }
        }, 100);
      }
    };
  } catch (err) {
    console.error('【音频播放错误】:', err);
  }
}

function stopAllAudioPlayback() {
  activeAudioSources.forEach(src => {
    try {
      src.stop();
    } catch (e) {}
  });
  activeAudioSources = [];
  nextPlaybackTime = 0;
  isSpeaking.value = false;
}

// ---------------------------------------------------------------------------
// WebSocket 通信
// ---------------------------------------------------------------------------
function getWSUrl() {
  const token = localStorage.getItem('rq_token') || '';
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//localhost:9091/api/voice/ws?codec=pcm&token=${token}`;
}

function connectWebSocket() {
  return new Promise((resolve, reject) => {
    const url = getWSUrl();
    ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
      isConnected.value = true;
      resolve();
    };

    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        handleServerMessage(JSON.parse(event.data));
      } else if (event.data instanceof ArrayBuffer) {
        isSpeaking.value = true;
        playBinaryPCM(event.data);
      }
    };

    ws.onclose = () => {
      isConnected.value = false;
      isRecording.value = false;
      stopAllAudioPlayback();
    };

    ws.onerror = (err) => {
      console.error('WebSocket connection error:', err);
      ElMessage.error('无法连接到控制网关端口，请确认后端已正常启动');
      reject(err);
    };
  });
}

function handleServerMessage(msg) {
  switch (msg.type) {
    case 'ready':
      break;

    case 'partial':
      currentText.value = msg.text;
      isFinal.value = false;
      break;

    case 'final':
      currentText.value = msg.text;
      isFinal.value = true;
      aiReplyText.value = ''; // Reset reply text for the new dialog turn
      break;

    case 'stream_token':
      aiReplyText.value += (msg.message || '');
      break;

    case 'result':
      if (msg.message) {
        aiReplyText.value = msg.message;
      }
      break;

    case 'interrupt':
      stopAllAudioPlayback();
      aiReplyText.value += ' [播音已打断]';
      break;
  }
}

onUnmounted(() => {
  stopVoice();
});
</script>

<style scoped>
.voice-live-container {
  width: 100%;
  height: calc(100vh - 120px);
  display: flex;
  justify-content: center;
  align-items: center;
}

.voice-card {
  width: 100%;
  max-width: 600px;
  min-height: 460px;
  display: flex;
  flex-direction: column;
  border-radius: 12px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.05);
}

:deep(.el-card__body) {
  display: flex;
  flex-direction: column;
  flex: 1;
  padding: 0;
}

.voice-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px;
  flex: 1;
}

.status-section {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 40px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: #909399;
}

.status-dot.dot-active {
  background-color: #67c23a;
  box-shadow: 0 0 8px rgba(103, 194, 58, 0.6);
  animation: breathe 1.5s infinite ease-in-out;
}

.status-dot.dot-speaking {
  background-color: #409eff;
  box-shadow: 0 0 8px rgba(64, 158, 255, 0.6);
  animation: breathe 0.8s infinite ease-in-out;
}

.status-dot.dot-disconnected {
  background-color: #f56c6c;
}

@keyframes breathe {
  0%, 100% { opacity: 0.5; transform: scale(0.9); }
  50% { opacity: 1; transform: scale(1.15); }
}

.button-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin-bottom: 40px;
}

.pulse-container {
  position: relative;
  width: 130px;
  height: 130px;
  display: flex;
  justify-content: center;
  align-items: center;
  margin-bottom: 16px;
}

.action-btn {
  width: 90px;
  height: 90px;
  border-radius: 50%;
  border: none;
  background: linear-gradient(135deg, #409eff 0%, #66b1ff 100%);
  color: #ffffff;
  font-size: 32px;
  cursor: pointer;
  z-index: 10;
  display: flex;
  justify-content: center;
  align-items: center;
  box-shadow: 0 6px 20px rgba(64, 158, 255, 0.3);
  transition: all 0.3s ease;
}

.action-btn:hover {
  transform: scale(1.05);
  box-shadow: 0 8px 24px rgba(64, 158, 255, 0.4);
}

.action-btn.connected {
  background: linear-gradient(135deg, #67c23a 0%, #85ce61 100%);
  box-shadow: 0 6px 20px rgba(103, 194, 58, 0.3);
}

.action-btn.connected:hover {
  background: linear-gradient(135deg, #f56c6c 0%, #f78989 100%);
  box-shadow: 0 6px 20px rgba(245, 108, 108, 0.3);
}

.btn-hint {
  font-size: 14px;
  color: #909399;
  font-weight: 500;
  margin-top: 4px;
}

/* Pulsing rings */
.pulse-ring {
  position: absolute;
  width: 100%;
  height: 100%;
  border-radius: 50%;
  background-color: rgba(64, 158, 255, 0.15);
  opacity: 0;
  pointer-events: none;
  z-index: 1;
}

.pulse-container.pulsing .pulse-ring {
  background-color: rgba(103, 194, 58, 0.15);
  animation: pulse-ring-animation 2s infinite cubic-bezier(0.215, 0.610, 0.355, 1);
}

.pulse-container.pulsing.speaking .pulse-ring {
  background-color: rgba(64, 158, 255, 0.25);
  animation: pulse-ring-animation 1.2s infinite cubic-bezier(0.215, 0.610, 0.355, 1);
}

.ring-2 {
  animation-delay: 0.5s !important;
}

@keyframes pulse-ring-animation {
  0% {
    transform: scale(0.7);
    opacity: 0.8;
  }
  100% {
    transform: scale(1.3);
    opacity: 0;
  }
}

.chat-section {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 16px;
  border-top: 1px solid #f2f2f2;
  padding-top: 30px;
}

.chat-bubble {
  padding: 12px 16px;
  border-radius: 8px;
  line-height: 1.5;
  font-size: 14px;
  width: 100%;
  box-sizing: border-box;
}

.user-bubble {
  background-color: rgba(64, 158, 255, 0.06);
  border-left: 4px solid #409eff;
}

.ai-bubble {
  background-color: rgba(103, 194, 58, 0.06);
  border-left: 4px solid #67c23a;
}

.bubble-title {
  font-weight: bold;
  font-size: 12px;
  color: #606266;
  margin-bottom: 4px;
}

.bubble-text {
  color: #303133;
  word-break: break-all;
  white-space: pre-wrap;
}

.partial-text {
  color: #409eff;
  font-style: italic;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
