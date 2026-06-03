<template>
  <div class="model-config-container">
    <el-card class="box-card config-card" v-loading="loading">
      <template #header>
        <div class="card-header">
          <span class="card-title" style="display: flex; align-items: center; gap: 6px;">
            <el-icon><Setting /></el-icon>
            <span>智能语音意图理解大模型参数设置</span>
          </span>
        </div>
      </template>

      <el-form :model="configForm" :rules="rules" ref="configFormRef" label-width="160px" class="config-form">
        <!-- API KEY -->
        <el-form-item 
          label="DeepSeek API Key" 
          prop="api_key"
        >
          <el-input
            v-model="configForm.api_key"
            type="password"
            placeholder="请输入您的 DeepSeek API 秘钥 (Bearer Token)"
            show-password
          ></el-input>
          <div class="form-tip">
            您的秘钥存储在专用的 PostgreSQL 数据库中，后端接口自动提供脱敏脱身遮蔽。
          </div>
        </el-form-item>

        <!-- API URL -->
        <el-form-item label="API 端点 URL" prop="api_url">
          <el-input 
            v-model="configForm.api_url" 
            placeholder="例如: https://api.deepseek.com/v1 (支持标准的 OpenAI 格式)"
          ></el-input>
        </el-form-item>

        <!-- 模型名称 -->
        <el-form-item label="使用的模型名称" prop="model_name">
          <el-input 
            v-model="configForm.model_name" 
            placeholder="例如: deepseek-chat"
          ></el-input>
        </el-form-item>

        <!-- 角色指定 -->
        <el-form-item label="角色指定" prop="system_role">
          <el-input 
            v-model="configForm.system_role" 
            placeholder="例如: 红皇后 (固定字段 system_role，大模型将以该设定的角色身份与意图进行响应)"
          ></el-input>
          <div class="form-tip">
            固定字段 <code>system_role</code>：大语言模型将以此设定角色身份与意图进行对话，并在后台运行时自动安全替换系统 Prompt 模板中的 <code>&#123;&#123;.SystemRole&#125;&#125;</code> 占位符。
          </div>
        </el-form-item>

        <!-- 性格指定 -->
        <el-form-item label="性格指定" prop="system_personality">
          <el-input 
            v-model="configForm.system_personality" 
            placeholder="例如: 符合皇后的语气"
          ></el-input>
          <div class="form-tip">
            固定字段 <code>system_personality</code>：大语言模型将以此性格特点进行对话表达，并在后台运行时自动安全替换系统 Prompt 模板中的 <code>&#123;&#123;.SystemPersonality&#125;&#125;</code> 占位符。
          </div>
        </el-form-item>

        <!-- 系统 Prompt 模板 -->
        <el-form-item label="系统 Prompt 模板" prop="system_prompt">
          <el-input
            v-model="configForm.system_prompt"
            type="textarea"
            :rows="4"
            placeholder="例如: 你是一个&#123;&#123;.SystemRole&#125;&#125;，性格是&#123;&#123;.SystemPersonality&#125;&#125;。你的职责是根据用户的话，挑选并正确调用最适合的工具。"
          ></el-input>
          <div class="form-tip">
            大模型系统级提示词模板，支持使用 <code>&#123;&#123;.SystemRole&#125;&#125;</code> 和 <code>&#123;&#123;.SystemPersonality&#125;&#125;</code> 占位符来动态插值角色与性格指定字段。
          </div>
        </el-form-item>

        <!-- 声纹主人锁配置 -->
        <el-divider content-position="left">
          <span style="font-weight: bold; color: #e6a23c; display: flex; align-items: center; gap: 4px;">
            <el-icon><Lock /></el-icon> 说话人主人声纹锁设置（防止背景杂音与他人控制）
          </span>
        </el-divider>

        <el-form-item label="主人声纹锁开关" prop="enable_voiceprint">
          <el-switch
            v-model="configForm.enable_voiceprint"
            active-text="开启声纹主人锁"
            inactive-text="关闭 (任何人或背景音均可控制)"
          ></el-switch>
          <div class="form-tip">
            开启后，系统将使用 <code>wespeaker</code> 离线声纹算法校验音色。相似度低于阈值时，自动忽略指令且不进行LLM响应，防止电视杂音、旁人聊天导致误触发。
          </div>
        </el-form-item>

        <el-form-item label="声纹匹配阈值" prop="voiceprint_threshold" v-if="configForm.enable_voiceprint">
          <el-slider
            v-model="configForm.voiceprint_threshold"
            :min="0.50"
            :max="0.90"
            :step="0.01"
            show-input
            style="max-width: 450px;"
          ></el-slider>
          <div class="form-tip">
            默认建议 0.65。如果主人发出的指令经常被拒绝，可以适当降低阈值（如0.60）；如果仍有电视背景声音能够误触发，请调高阈值（如0.70）。
          </div>
        </el-form-item>

        <el-form-item label="主人声纹建档" prop="master_voiceprint">
          <div style="margin-bottom: 12px;">
            <el-tag :type="voiceprintCount > 0 ? 'success' : 'danger'" effect="dark" size="large">
              {{ voiceprintCount > 0 ? `已建立 ${voiceprintCount} 条声纹采样` : '尚未录入声纹' }}
            </el-tag>
          </div>

          <div v-if="voiceprintCount > 0" style="margin-bottom: 12px; display: flex; flex-wrap: wrap; gap: 8px;">
            <el-tag
              v-for="i in voiceprintCount"
              :key="i"
              closable
              type="info"
              effect="plain"
              size="large"
              @close="deleteVoiceprint(i - 1)"
            >
              声纹采样 #{{ i }}
            </el-tag>
          </div>

          <el-button
            :type="isRegistering ? 'danger' : 'warning'"
            :loading="isRegistering"
            :disabled="voiceprintCount >= 10"
            @click="startRecording"
            style="width: 100%; max-width: 450px;"
          >
            {{ isRegistering ? `请对着麦克风说话 (${registerTimeLeft} 秒)...` : (voiceprintCount >= 10 ? '已达上限 (最多 10 条)' : '添加一条新的声纹采样') }}
          </el-button>

          <div class="form-tip">
            点此按钮后，请使用您平常说话的自然语气说一段话（推荐说 3 到 5 秒）。<br>
            建议采集 <strong>3 ~ 5 条</strong> 不同状态的声纹（如正常说话、轻声说话、刚起床时的声音），多条采样可以显著提高识别准确率。
          </div>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="submitLoading" @click="handleSave">
            保存配置并实时生效
          </el-button>
          <el-button @click="fetchConfig">重置为当前数据库状态</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 开发使用提示卡片 -->
    <el-card class="box-card info-card">
      <template #header>
        <span class="card-title" style="display: flex; align-items: center; gap: 6px;">
          <el-icon><InfoFilled /></el-icon>
          <span>动态配置是如何运行的？</span>
        </span>
      </template>
      <div class="info-content">
        <p>1. <strong>实时响应机制</strong>：后端的语音转意图服务（<code>services/nlp_service.go</code>）会在每次接收到交互请求时，自动从数据库载入最新更新的配置参数。</p>
        <p>2. <strong>热重载（Hot-Reload）</strong>：这意味着您在管理后台中输入新的 API Key 或修改 System Prompts 后，无需重启您的 Golang 后端服务即可自动加载，直接进入测试！</p>
        <p>3. <strong>服务异常捕获</strong>：如果由于配置错误、欠费、超时或其它网络原因导致大模型查询请求失败，系统将停止执行，并直接在语音历史日志中呈现详尽的错误日志，方便实时追踪排查。</p>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import request from '../utils/request';

const loading = ref(false);
const submitLoading = ref(false);
const configFormRef = ref(null);

const configForm = ref({
  api_key: '',
  api_url: 'https://api.deepseek.com',
  model_name: 'deepseek-v4-pro',
  system_role: '红皇后',
  system_personality: '符合皇后的语气',
  system_prompt: '',
  enable_voiceprint: false,
  voiceprint_threshold: 0.65,
  master_voiceprint: '',
});

const rules = {
  api_key: [{ required: true, message: '请配置 API Key 秘钥以激活大模型', trigger: 'blur' }],
  api_url: [{ required: true, message: 'API 端点 URL 不能为空', trigger: 'blur' }],
  model_name: [{ required: true, message: '大模型名称不能为空', trigger: 'blur' }],
  system_role: [{ required: true, message: '大模型扮演的角色名称不能为空', trigger: 'blur' }],
  system_personality: [{ required: true, message: '大模型扮演的个性和性格特点不能为空', trigger: 'blur' }],
  system_prompt: [{ required: true, message: '系统 Prompt 模板不能为空', trigger: 'blur' }],
};

// 声纹采样数量计算
const voiceprintCount = computed(() => {
  const vp = configForm.value.master_voiceprint;
  if (!vp) return 0;
  try {
    const parsed = JSON.parse(vp);
    if (Array.isArray(parsed) && parsed.length > 0) {
      // 新格式: [][]float32 —— 数组的数组
      if (Array.isArray(parsed[0])) return parsed.length;
      // 旧格式: []float32 —— 单条声纹
      return 1;
    }
  } catch (e) { /* ignore */ }
  return 0;
});

// 删除指定索引的声纹采样
async function deleteVoiceprint(index) {
  try {
    await ElMessageBox.confirm(
      `确定要删除声纹采样 #${index + 1} 吗？`,
      '删除确认',
      { confirmButtonText: '确定删除', cancelButtonText: '取消', type: 'warning' }
    );
  } catch {
    return; // 用户取消
  }
  try {
    await request.delete(`/config/voiceprint/${index}`);
    ElMessage.success(`已删除声纹采样 #${index + 1}`);
    fetchConfig();
  } catch (error) {
    ElMessage.error(error.response?.data?.message || '删除声纹失败');
  }
}

// 主人声纹录音与注册交互逻辑
const isRegistering = ref(false);
const registerTimeLeft = ref(5);
const micStream = ref(null);
let recAudioContext = null;
let recScriptProcessor = null;
let recSource = null;
let pcmChunks = [];

async function startRecording() {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    micStream.value = stream;
    pcmChunks = [];
    isRegistering.value = true;
    registerTimeLeft.value = 5;

    recAudioContext = new (window.AudioContext || window.webkitAudioContext)({
      sampleRate: 16000,
    });
    recSource = recAudioContext.createMediaStreamSource(stream);
    recScriptProcessor = recAudioContext.createScriptProcessor(4096, 1, 1);

    recScriptProcessor.onaudioprocess = (event) => {
      const inputData = event.inputBuffer.getChannelData(0);
      const pcmData = new Int16Array(inputData.length);
      for (let i = 0; i < inputData.length; i++) {
        const s = Math.max(-1, Math.min(1, inputData[i]));
        pcmData[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
      }
      pcmChunks.push(pcmData);
    };

    recSource.connect(recScriptProcessor);
    recScriptProcessor.connect(recAudioContext.destination);

    const interval = setInterval(() => {
      registerTimeLeft.value--;
      if (registerTimeLeft.value <= 0) {
        clearInterval(interval);
        stopRecordingAndRegister();
      }
    }, 1000);

  } catch (err) {
    console.error('麦克风开启失败:', err);
    ElMessage.error('无法启动录音设备，请确保麦克风已授权');
    isRegistering.value = false;
  }
}

async function stopRecordingAndRegister() {
  isRegistering.value = false;

  if (recScriptProcessor) {
    recScriptProcessor.disconnect();
    recScriptProcessor = null;
  }
  if (recSource) {
    recSource.disconnect();
    recSource = null;
  }
  if (recAudioContext) {
    recAudioContext.close();
    recAudioContext = null;
  }
  if (micStream.value) {
    micStream.value.getTracks().forEach(track => track.stop());
    micStream.value = null;
  }

  let totalLength = 0;
  for (const chunk of pcmChunks) {
    totalLength += chunk.length;
  }

  if (totalLength === 0) {
    ElMessage.warning('录音失败，没有捕获到音频数据');
    return;
  }

  const mergedPcm = new Int16Array(totalLength);
  let offset = 0;
  for (const chunk of pcmChunks) {
    mergedPcm.set(chunk, offset);
    offset += chunk.length;
  }

  const bytes = new Uint8Array(mergedPcm.buffer);
  let binary = '';
  const len = bytes.byteLength;
  for (let i = 0; i < len; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  const base64Data = btoa(binary);

  loading.value = true;
  try {
    const res = await request.post('/config/voiceprint/register', {
      audio_data: base64Data
    });
    ElMessage.success(res.data?.message || '声纹采样录入成功！');
    fetchConfig();
  } catch (error) {
    console.error(error);
    ElMessage.error(error.response?.data?.message || '声纹特征录入失败，请确保录制时间足够长且说话清晰');
  } finally {
    loading.value = false;
  }
}

// 从后端获取当前配置详情
const fetchConfig = async () => {
  loading.value = true;
  try {
    const response = await request.get('/config/model');
    if (response.data) {
      configForm.value = response.data;
    }
  } catch (error) {
    console.error(error);
  } finally {
    loading.value = false;
  }
};

// 保存大模型配置参数
const handleSave = async () => {
  if (!configFormRef.value) return;

  await configFormRef.value.validate(async (valid) => {
    if (valid) {
      submitLoading.value = true;
      try {
        await request.post('/config/model', configForm.value);
        ElMessage.success('大模型参数配置保存成功！已动态实时生效');
        fetchConfig();
      } catch (error) {
        console.error(error);
      } finally {
        submitLoading.value = false;
      }
    }
  });
};

onMounted(() => {
  fetchConfig();
});
</script>

<style scoped>
.model-config-container {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.config-card {
  padding: 10px;
}

.card-title {
  font-weight: bold;
  font-size: 15px;
  color: #303133;
}

.config-form {
  max-width: 900px;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  line-height: 1.4;
}

.full-row {
  width: 100%;
}

.info-card {
  border-left: 5px solid #409eff;
}

.info-content {
  font-size: 13px;
  color: #606266;
  line-height: 1.8;
}

.info-content p {
  margin: 0 0 10px 0;
}

.info-content p:last-child {
  margin: 0;
}
</style>
