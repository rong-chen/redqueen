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
          label="DashScope API Key" 
          prop="api_key"
        >
          <el-input
            v-model="configForm.api_key"
            type="password"
            placeholder="请输入您的 阿里云 DashScope API 秘钥"
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
            placeholder="例如: wss://dashscope.aliyuncs.com/api-ws/v1/realtime"
          ></el-input>
        </el-form-item>

        <!-- 模型名称 -->
        <el-form-item label="使用的模型名称" prop="model_name">
          <el-input 
            v-model="configForm.model_name" 
            placeholder="例如: qwen3.5-omni-plus-realtime"
          ></el-input>
        </el-form-item>

        <!-- 音色选择 -->
        <el-form-item label="使用的音色名称" prop="voice">
          <el-select v-model="configForm.voice" placeholder="选择音色" style="width: 100%;">
            <el-option label="Tina (推荐 - 默认女声)" value="Tina" />
            <el-option label="Cherry (温柔女声)" value="Cherry" />
            <el-option label="Diana (熟女女声)" value="Diana" />
            <el-option label="Grace (知性女声)" value="Grace" />
            <el-option label="Jimmy (活力男声)" value="Jimmy" />
          </el-select>
          <div class="form-tip">
            Qwen-Omni 实时语音模型支持的系统预设音色，支持 Tina, Cherry, Diana, Grace, Jimmy 等。
          </div>
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
        <p>1. <strong>实时响应机制</strong>：后端的 Qwen-Omni 语音服务会在每次接收到交互请求时，自动从数据库载入最新配置参数。</p>
        <p>2. <strong>热重载（Hot-Reload）</strong>：这意味着您在管理后台中输入新的 API Key、切换音色或修改 System Prompts 后，无需重启您的 Golang 后端服务即可自动加载，直接进入测试！</p>
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
  api_url: 'wss://dashscope.aliyuncs.com/api-ws/v1/realtime',
  model_name: 'qwen3.5-omni-plus-realtime',
  voice: 'Tina',
  system_role: '红皇后',
  system_personality: '符合皇后的语气',
  system_prompt: '',
});

const rules = {
  api_key: [{ required: true, message: '请配置 API Key 秘钥以激活大模型', trigger: 'blur' }],
  api_url: [{ required: true, message: 'API 端点 URL 不能为空', trigger: 'blur' }],
  model_name: [{ required: true, message: '大模型名称不能为空', trigger: 'blur' }],
  voice: [{ required: true, message: '请选择或输入音色名称', trigger: 'change' }],
  system_role: [{ required: true, message: '大模型扮演的角色名称不能为空', trigger: 'blur' }],
  system_personality: [{ required: true, message: '大模型扮演的个性和性格特点不能为空', trigger: 'blur' }],
  system_prompt: [{ required: true, message: '系统 Prompt 模板不能为空', trigger: 'blur' }],
};
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
