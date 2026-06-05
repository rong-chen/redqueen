<template>
  <div class="recorder-container">
    <div class="header">
      <h2>唤醒词专属录音机</h2>
      <p class="subtitle">
        为确保最佳训练效果，请在一个相对安静的环境下，用正常音量和语速说出您的唤醒词（如“红皇后”）。<br>
        建议录制 20~30 遍。系统会自动为您转码为模型要求的 16kHz 单声道 WAV 格式。
      </p>
    </div>

    <div class="recording-area">
      <div class="record-button-wrapper">
        <el-button 
          type="primary" 
          circle 
          class="record-btn" 
          :class="{ 'is-recording': isRecording }"
          @click="toggleRecording"
        >
          <el-icon v-if="!isRecording" size="40"><Microphone /></el-icon>
          <el-icon v-else size="40"><VideoPause /></el-icon>
        </el-button>
        <div v-if="isRecording" class="pulse-ring"></div>
      </div>
      
      <div class="status-text">
        <span v-if="isRecording" class="recording-text">正在录音，请说话... ({{ formatTime(recordingDuration) }})</span>
        <span v-else class="idle-text">点击开始录制</span>
      </div>
    </div>

    <div class="clips-section">
      <div class="clips-header">
        <h3>已录制列表 ({{ recordedClips.length }})</h3>
        <el-button 
          v-if="recordedClips.length > 0" 
          type="success" 
          @click="downloadAllAsZip"
        >
          <el-icon><Download /></el-icon> 一键打包下载 (ZIP)
        </el-button>
      </div>

      <el-empty v-if="recordedClips.length === 0" description="暂无录音，赶快开始录制吧" />

      <div class="clips-list" v-else>
        <div 
          class="clip-item" 
          v-for="(clip, index) in recordedClips" 
          :key="clip.id"
        >
          <div class="clip-info">
            <span class="clip-index">#{{ index + 1 }}</span>
            <span class="clip-name">{{ clip.name }}</span>
            <span class="clip-duration">{{ formatTime(clip.duration) }}</span>
          </div>
          <div class="clip-actions">
            <audio :src="clip.url" controls class="audio-player"></audio>
            <el-button type="primary" link @click="downloadClip(clip)">
              下载
            </el-button>
            <el-button type="danger" link @click="deleteClip(index)">
              删除
            </el-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onUnmounted } from 'vue';
import { Microphone, VideoPause, Download } from '@element-plus/icons-vue';
import { ElMessage } from 'element-plus';
import JSZip from 'jszip';
import { saveAs } from 'file-saver';

// ---------------------------------------------------------------------------
// 状态
// ---------------------------------------------------------------------------
const isRecording = ref(false);
const recordingDuration = ref(0);
const recordedClips = ref([]);

// ---------------------------------------------------------------------------
// 音频录制相关对象
// ---------------------------------------------------------------------------
let mediaStream = null;
let audioContext = null;
let sourceNode = null;
let scriptProcessor = null;
let pcmDataBuffer = [];
let recordingInterval = null;

// ---------------------------------------------------------------------------
// 控制逻辑
// ---------------------------------------------------------------------------
async function toggleRecording() {
  if (isRecording.value) {
    stopRecording();
  } else {
    await startRecording();
  }
}

async function startRecording() {
  try {
    // 请求麦克风权限，要求关闭回声消除和降噪，获取最原始的声音
    mediaStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        channelCount: 1,
        echoCancellation: false,
        noiseSuppression: false,
        autoGainControl: false,
      },
    });

    // 强行指定 16000 采样率
    audioContext = new (window.AudioContext || window.webkitAudioContext)({ sampleRate: 16000 });
    
    sourceNode = audioContext.createMediaStreamSource(mediaStream);
    scriptProcessor = audioContext.createScriptProcessor(4096, 1, 1);

    pcmDataBuffer = [];
    recordingDuration.value = 0;

    scriptProcessor.onaudioprocess = (event) => {
      if (!isRecording.value) return;
      const inputData = event.inputBuffer.getChannelData(0);
      // 收集浮点数 PCM 数据
      pcmDataBuffer.push(new Float32Array(inputData));
    };

    sourceNode.connect(scriptProcessor);
    scriptProcessor.connect(audioContext.destination);

    isRecording.value = true;
    
    // 计时器
    recordingInterval = setInterval(() => {
      recordingDuration.value += 100; // ms
    }, 100);

  } catch (err) {
    console.error('麦克风获取失败', err);
    ElMessage.error('无法访问麦克风，请检查权限设置');
  }
}

function stopRecording() {
  isRecording.value = false;
  clearInterval(recordingInterval);
  
  if (scriptProcessor) {
    scriptProcessor.disconnect();
    scriptProcessor = null;
  }
  if (sourceNode) {
    sourceNode.disconnect();
    sourceNode = null;
  }
  if (mediaStream) {
    mediaStream.getTracks().forEach(track => track.stop());
    mediaStream = null;
  }
  if (audioContext) {
    audioContext.close();
    audioContext = null;
  }

  processAndSaveRecording();
}

// ---------------------------------------------------------------------------
// 音频处理与导出
// ---------------------------------------------------------------------------
function processAndSaveRecording() {
  if (pcmDataBuffer.length === 0) return;

  // 1. 合并所有 Float32Array 块
  let totalLength = 0;
  for (let block of pcmDataBuffer) totalLength += block.length;
  
  const combinedFloat32 = new Float32Array(totalLength);
  let offset = 0;
  for (let block of pcmDataBuffer) {
    combinedFloat32.set(block, offset);
    offset += block.length;
  }

  // 2. 转换为 16-bit PCM (WAV 需要)
  const pcm16 = new Int16Array(totalLength);
  for (let i = 0; i < totalLength; i++) {
    let s = Math.max(-1, Math.min(1, combinedFloat32[i]));
    pcm16[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
  }

  // 3. 生成 WAV 文件头和 Blob
  const wavBlob = createWavBlob(pcm16, 16000);
  const url = URL.createObjectURL(wavBlob);
  const clipName = `wakeword_${new Date().getTime()}.wav`;

  recordedClips.value.push({
    id: new Date().getTime(),
    name: clipName,
    url: url,
    blob: wavBlob,
    duration: recordingDuration.value
  });
}

function createWavBlob(pcm16Data, sampleRate) {
  const numChannels = 1;
  const byteRate = sampleRate * numChannels * 2;
  const blockAlign = numChannels * 2;
  const dataSize = pcm16Data.length * 2;
  const buffer = new ArrayBuffer(44 + dataSize);
  const view = new DataView(buffer);

  // RIFF chunk descriptor
  writeString(view, 0, 'RIFF');
  view.setUint32(4, 36 + dataSize, true);
  writeString(view, 8, 'WAVE');

  // fmt sub-chunk
  writeString(view, 12, 'fmt ');
  view.setUint32(16, 16, true); // Subchunk1Size (16 for PCM)
  view.setUint16(20, 1, true); // AudioFormat (1 for PCM)
  view.setUint16(22, numChannels, true); // NumChannels
  view.setUint32(24, sampleRate, true); // SampleRate
  view.setUint32(28, byteRate, true); // ByteRate
  view.setUint16(32, blockAlign, true); // BlockAlign
  view.setUint16(34, 16, true); // BitsPerSample

  // data sub-chunk
  writeString(view, 36, 'data');
  view.setUint32(40, dataSize, true);

  // 写入 PCM 16-bit 数据
  let offset = 44;
  for (let i = 0; i < pcm16Data.length; i++, offset += 2) {
    view.setInt16(offset, pcm16Data[i], true);
  }

  return new Blob([buffer], { type: 'audio/wav' });
}

function writeString(view, offset, string) {
  for (let i = 0; i < string.length; i++) {
    view.setUint8(offset + i, string.charCodeAt(i));
  }
}

// ---------------------------------------------------------------------------
// 列表操作
// ---------------------------------------------------------------------------
function downloadClip(clip) {
  saveAs(clip.blob, clip.name);
}

function deleteClip(index) {
  const clip = recordedClips.value[index];
  URL.revokeObjectURL(clip.url);
  recordedClips.value.splice(index, 1);
}

async function downloadAllAsZip() {
  const zip = new JSZip();
  recordedClips.value.forEach(clip => {
    zip.file(clip.name, clip.blob);
  });
  const content = await zip.generateAsync({ type: 'blob' });
  saveAs(content, 'custom_wakeword_samples.zip');
  ElMessage.success('全部打包下载完成！请将压缩包里的文件解压到训练目录使用。');
}

function formatTime(ms) {
  const seconds = (ms / 1000).toFixed(1);
  return `${seconds}s`;
}

// ---------------------------------------------------------------------------
// 卸载清理
// ---------------------------------------------------------------------------
onUnmounted(() => {
  if (isRecording.value) {
    stopRecording();
  }
  recordedClips.value.forEach(clip => URL.revokeObjectURL(clip.url));
});
</script>

<style scoped>
.recorder-container {
  max-width: 800px;
  margin: 0 auto;
  padding: 20px;
}

.header {
  text-align: center;
  margin-bottom: 40px;
}

.subtitle {
  color: #606266;
  font-size: 14px;
  line-height: 1.6;
}

.recording-area {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin-bottom: 50px;
}

.record-button-wrapper {
  position: relative;
  margin-bottom: 20px;
}

.record-btn {
  width: 100px;
  height: 100px;
  font-size: 40px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  transition: all 0.3s ease;
  z-index: 2;
  position: relative;
}

.record-btn.is-recording {
  background-color: #f56c6c;
  border-color: #f56c6c;
  color: white;
}

.pulse-ring {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 100px;
  height: 100px;
  background-color: rgba(245, 108, 108, 0.4);
  border-radius: 50%;
  animation: pulse 1.5s infinite ease-out;
  z-index: 1;
}

@keyframes pulse {
  0% { transform: translate(-50%, -50%) scale(1); opacity: 1; }
  100% { transform: translate(-50%, -50%) scale(2); opacity: 0; }
}

.status-text {
  font-size: 16px;
  font-weight: bold;
}

.recording-text {
  color: #f56c6c;
  animation: blink 1s infinite alternate;
}

@keyframes blink {
  from { opacity: 1; }
  to { opacity: 0.5; }
}

.idle-text {
  color: #909399;
}

.clips-section {
  background: white;
  border-radius: 12px;
  padding: 20px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.05);
}

.clips-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
  border-bottom: 1px solid #ebeef5;
  padding-bottom: 10px;
}

.clips-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.clip-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: #f8f9fa;
  border-radius: 8px;
  transition: background 0.3s;
}

.clip-item:hover {
  background: #f0f2f5;
}

.clip-info {
  display: flex;
  align-items: center;
  gap: 15px;
}

.clip-index {
  font-weight: bold;
  color: #909399;
  width: 30px;
}

.clip-name {
  font-family: monospace;
  color: #303133;
}

.clip-duration {
  color: #67c23a;
  font-size: 13px;
  background: #f0f9eb;
  padding: 2px 6px;
  border-radius: 4px;
}

.clip-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.audio-player {
  height: 30px;
  width: 250px;
}
</style>
