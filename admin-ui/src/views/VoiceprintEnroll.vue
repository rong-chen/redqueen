<template>
  <div class="enroll-container">
    <el-card class="enroll-card">
      <div class="enroll-content">
        <h2>声纹注册 (Voiceprint Enrollment)</h2>
        <p class="description">
          为了提高系统唤醒的准确性，请在点击“录制”后，立刻、连续、清晰地大声念出唤醒词。
          <br>
          您需要录制 <strong>3 次</strong>，每次最长 <strong>2 秒</strong>（录完可手动点击结束）。
        </p>

        <div class="progress-section">
          <el-steps :active="currentStep" finish-status="success" align-center>
            <el-step title="第一次采样"></el-step>
            <el-step title="第二次采样"></el-step>
            <el-step title="第三次采样"></el-step>
          </el-steps>
        </div>

        <div class="action-section" v-if="currentStep < 3">
          <div class="status-text" :class="{ recording: isRecording }">
            {{ isRecording ? `正在录制 (${(2 - recordTime).toFixed(1)}s)...` : '准备就绪' }}
          </div>

          <div class="button-group">
            <button 
              class="action-btn" 
              :class="{ recording: isRecording }"
              @click="toggleRecording"
            >
              <el-icon class="btn-icon">
                <Microphone v-if="!isRecording" />
                <VideoPause v-else />
              </el-icon>
            </button>
          </div>
          <div class="hint-text">
            {{ isRecording ? '再次点击立刻结束' : '点击麦克风开始录音' }}
          </div>
        </div>

        <div class="result-section" v-else>
          <el-result icon="success" title="采样完成" sub-title="3次采样已收集完毕，准备提交融合特征">
            <template #extra>
              <el-button type="primary" :loading="isSubmitting" @click="submitEnrollment">提交注册声纹</el-button>
              <el-button @click="resetEnrollment" :disabled="isSubmitting">重新录制</el-button>
            </template>
          </el-result>
        </div>

      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onUnmounted } from 'vue';
import { Microphone, VideoPause } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';

const currentStep = ref(0);
const isRecording = ref(false);
const recordTime = ref(0);
const isSubmitting = ref(false);

const samples = ref([]); // Store base64 PCM strings
let audioContext = null;
let mediaStream = null;
let scriptProcessor = null;
let source = null;
let currentPcmData = [];
let timer = null;

// Ensure audio context is ready
function ensureAudioContext() {
  if (!audioContext) {
    audioContext = new (window.AudioContext || window.webkitAudioContext)({
      sampleRate: 16000
    });
  }
  if (audioContext.state === 'suspended') {
    audioContext.resume();
  }
}

// Convert float32 pcm to base64 int16
function encodeBase64PCM(float32Array) {
  const int16Array = new Int16Array(float32Array.length);
  for (let i = 0; i < float32Array.length; i++) {
    const s = Math.max(-1, Math.min(1, float32Array[i]));
    int16Array[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
  }
  const buffer = new Uint8Array(int16Array.buffer);
  let binary = '';
  for (let i = 0; i < buffer.byteLength; i++) {
    binary += String.fromCharCode(buffer[i]);
  }
  return btoa(binary);
}

async function toggleRecording() {
  if (isRecording.value) {
    stopRecording();
  } else {
    await startRecording();
  }
}

async function startRecording() {
  if (isRecording.value) return;
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
    scriptProcessor = audioContext.createScriptProcessor(4096, 1, 1);
    
    currentPcmData = [];
    scriptProcessor.onaudioprocess = (event) => {
      if (!isRecording.value) return;
      const inputData = event.inputBuffer.getChannelData(0);
      currentPcmData.push(new Float32Array(inputData));
    };

    source.connect(scriptProcessor);
    scriptProcessor.connect(audioContext.destination);

    isRecording.value = true;
    recordTime.value = 0;
    
    // Auto stop after 2 seconds
    timer = setInterval(() => {
      recordTime.value += 0.1;
      if (recordTime.value >= 2.0) {
        stopRecording();
      }
    }, 100);

  } catch (err) {
    console.error('麦克风开启失败:', err);
    ElMessage.error('开启声音采集失败，请允许麦克风权限');
  }
}

function stopRecording() {
  if (!isRecording.value) return;
  
  isRecording.value = false;
  if (timer) clearInterval(timer);
  
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

  // Merge PCM chunks
  let totalLength = 0;
  for (let chunk of currentPcmData) {
    totalLength += chunk.length;
  }
  
  if (totalLength === 0) {
    ElMessage.warning('未能采集到有效音频');
    return;
  }

  const merged = new Float32Array(totalLength);
  let offset = 0;
  for (let chunk of currentPcmData) {
    merged.set(chunk, offset);
    offset += chunk.length;
  }

  const base64Data = encodeBase64PCM(merged);
  samples.value.push(base64Data);
  currentStep.value++;
}

async function submitEnrollment() {
  if (samples.value.length < 3) return;
  isSubmitting.value = true;
  
  try {
    const token = localStorage.getItem('rq_token');
    const res = await fetch('http://localhost:9091/api/voiceprint/enroll', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`
      },
      body: JSON.stringify({ samples: samples.value })
    });
    
    const data = await res.json();
    if (res.ok && data.code === 200) {
      ElMessage.success(data.message || '声纹注册成功！');
      resetEnrollment(); // 可以跳转或重置
    } else {
      ElMessage.error(data.message || '声纹注册失败');
    }
  } catch (err) {
    console.error(err);
    ElMessage.error('网络请求失败');
  } finally {
    isSubmitting.value = false;
  }
}

function resetEnrollment() {
  currentStep.value = 0;
  samples.value = [];
  recordTime.value = 0;
}

onUnmounted(() => {
  stopRecording();
});
</script>

<style scoped>
.enroll-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: calc(100vh - 120px);
}

.enroll-card {
  width: 100%;
  max-width: 600px;
  border-radius: 12px;
}

.enroll-content {
  padding: 40px;
  text-align: center;
}

.description {
  color: #606266;
  margin-bottom: 30px;
  line-height: 1.6;
}

.progress-section {
  margin-bottom: 40px;
}

.action-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  min-height: 150px;
}

.status-text {
  font-size: 16px;
  margin-bottom: 20px;
  color: #909399;
  font-weight: bold;
}

.status-text.recording {
  color: #f56c6c;
  animation: blink 1s infinite;
}

@keyframes blink {
  0% { opacity: 1; }
  50% { opacity: 0.5; }
  100% { opacity: 1; }
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

.action-btn.recording {
  background: linear-gradient(135deg, #f56c6c 0%, #f78989 100%);
  box-shadow: 0 6px 20px rgba(245, 108, 108, 0.4);
  animation: pulse-red 1.5s infinite;
}

@keyframes pulse-red {
  0% { transform: scale(1); box-shadow: 0 0 0 0 rgba(245, 108, 108, 0.7); }
  70% { transform: scale(1.05); box-shadow: 0 0 0 15px rgba(245, 108, 108, 0); }
  100% { transform: scale(1); box-shadow: 0 0 0 0 rgba(245, 108, 108, 0); }
}

.hint-text {
  margin-top: 15px;
  font-size: 13px;
  color: #c0c4cc;
}
</style>
