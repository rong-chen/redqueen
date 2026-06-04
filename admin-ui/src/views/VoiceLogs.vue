<template>
  <div class="voice-logs-container">
    <!-- 模拟测试控制卡片 -->
    <el-card class="box-card simulator-card">
      <template #header>
        <div class="card-header">
          <span class="card-title" style="display: flex; align-items: center; gap: 6px;">
            <el-icon><Microphone /></el-icon>
            <span>开发者语音模拟注入端 (STT 模拟器)</span>
          </span>
        </div>
      </template>
      <el-form :inline="true" :model="simulateForm" class="simulate-form">
        <el-form-item label="模拟转写文字" class="form-item-transcript">
          <el-input
            v-model="simulateForm.transcript"
            placeholder="例如: 查询北京明天的温度并返回报告"
            style="width: 320px"
          ></el-input>
        </el-form-item>
        <el-form-item label="置信度" class="form-item-slider">
          <el-slider
            v-model="simulateForm.confidence"
            :min="0.1"
            :max="1"
            :step="0.05"
            class="simulate-slider"
          ></el-slider>
        </el-form-item>
        <el-form-item class="form-item-btn">
          <el-button type="success" :loading="submitLoading" @click="handleSimulate" class="simulate-btn">
            模拟语音下发 (调用 DeepSeek)
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 交互日志列表卡片 -->
    <el-card class="box-card table-card">
      <template #header>
        <div class="card-header header-between">
          <span class="card-title" style="display: flex; align-items: center; gap: 6px;">
            <el-icon><List /></el-icon>
            <span>交互识别与意图日志</span>
          </span>
          <el-button type="primary" size="small" @click="fetchHistory(true)">
            刷新日志
          </el-button>
        </div>
      </template>

      <el-table :data="historyList" v-loading="tableLoading" stripe border style="width: 100%">
        <el-table-column prop="ID" label="ID" width="70" align="center"></el-table-column>
        <el-table-column label="交互时间" width="180">
          <template #default="scope">
            {{ formatDate(scope.row.CreatedAt) }}
          </template>
        </el-table-column>
        <el-table-column prop="audio_path" label="音频地址" width="180" show-overflow-tooltip></el-table-column>
        <el-table-column prop="transcript" label="语音转文字 (STT 结果)" min-width="200"></el-table-column>
        <el-table-column prop="reply_text" label="大模型回复" min-width="220" show-overflow-tooltip>
          <template #default="scope">
            <span v-if="scope.row.reply_text" style="color: #409EFF; font-weight: 500;">
              {{ scope.row.reply_text }}
            </span>
            <span v-else style="color: #909399; font-style: italic;">无回复</span>
          </template>
        </el-table-column>
        <el-table-column prop="intent" label="解析意图 (NLP)" width="150" align="center">
          <template #default="scope">
            <el-tooltip
              v-if="scope.row.reply_text"
              :content="scope.row.reply_text"
              placement="top"
              effect="dark"
            >
              <el-tag :type="getIntentTag(scope.row.intent)" style="cursor: pointer;">
                {{ getIntentLabel(scope.row.intent) }}
              </el-tag>
            </el-tooltip>
            <el-tag v-else :type="getIntentTag(scope.row.intent)">
              {{ getIntentLabel(scope.row.intent) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="意图置信度" width="120" align="center">
          <template #default="scope">
            <el-progress 
              :percentage="Math.round(scope.row.confidence * 100)" 
              :status="scope.row.confidence >= 0.85 ? 'success' : 'warning'"
            />
          </template>
        </el-table-column>
        <el-table-column prop="status" label="执行结果" width="120" align="center">
          <template #default="scope">
            <el-tag :type="scope.row.status === 'success' ? 'success' : (scope.row.status === 'failed' ? 'danger' : 'info')">
              {{ getStatusLabel(scope.row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="error_message" label="执行异常日志" min-width="150" show-overflow-tooltip>
          <template #default="scope">
            <span class="text-danger">{{ scope.row.error_message || '无异常' }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue';
import { ElMessage } from 'element-plus';
import request from '../utils/request';

const simulateForm = ref({
  transcript: '',
  confidence: 0.95,
});

const submitLoading = ref(false);
const tableLoading = ref(false);
const historyList = ref([]);
let pollTimer = null;

// 获取历史语音控制交互记录 (showLoading 控制是否显示加载动画，默认不显示以避免定时轮询时界面闪烁)
const fetchHistory = async (showLoading = false) => {
  if (showLoading) {
    tableLoading.value = true;
  }
  try {
    const response = await request.get('/voice/history?limit=15');
    historyList.value = response.data || [];
  } catch (error) {
    console.error(error);
  } finally {
    if (showLoading) {
      tableLoading.value = false;
    }
  }
};

// 提交模拟的语音转写输入，触发后端大模型意图提取
const handleSimulate = async () => {
  if (!simulateForm.value.transcript.trim()) {
    ElMessage.warning('模拟文字不能为空');
    return;
  }

  submitLoading.value = true;
  try {
    await request.post('/voice/command', {
      audio_path: `/tmp/mocks/${Date.now()}.wav`,
      transcript: simulateForm.value.transcript,
      confidence: simulateForm.value.confidence,
    });
    ElMessage.success('模拟语音命令注入成功！大模型已开始后台意图处理');
    simulateForm.value.transcript = '';
    // 延迟 500ms 刷新列表，保证异步意图处理已存储
    setTimeout(fetchHistory, 500);
  } catch (error) {
    console.error(error);
  } finally {
    submitLoading.value = false;
  }
};

// 日期格式化辅助函数
const formatDate = (isoString) => {
  if (!isoString) return '-';
  const date = new Date(isoString);
  return date.toLocaleString();
};

// 获取意图标签分类
const getIntentTag = (intent) => {
  switch (intent) {
    case 'external_mcp_call': return 'success';
    case 'conversation': return 'primary';
    case 'error': return 'danger';
    default: return 'info';
  }
};

// 意图名称汉化
const getIntentLabel = (intent) => {
  switch (intent) {
    case 'conversation': return '日常对话';
    case 'external_mcp_call': return 'MCP 工具调用';
    case 'error': return '解析异常';
    default: return intent || '解析中...';
  }
};

// 状态文字汉化
const getStatusLabel = (status) => {
  switch (status) {
    case 'success': return '执行成功';
    case 'failed': return '执行失败';
    case 'pending': return '正在解析';
    default: return status;
  }
};

onMounted(() => {
  // 首次加载，显示加载动画以提供明确的反馈
  fetchHistory(true);

  // 开启每2秒一次的静默后台自动轮询，实时展现转写与意图执行结果，省去手动刷新的麻烦
  pollTimer = setInterval(() => {
    fetchHistory(false);
  }, 2000);
});

onBeforeUnmount(() => {
  // 组件卸载前清除定时器，避免内存泄漏与无效的后台请求
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
});
</script>

<style scoped>
.voice-logs-container {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.simulator-card {
  border-left: 5px solid #67c23a;
}

.card-title {
  font-weight: bold;
  font-size: 15px;
  color: #303133;
}

.header-between {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.simulate-form {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 20px;
}

.simulate-form :deep(.el-form-item) {
  margin-bottom: 0 !important;
  margin-right: 0 !important;
  display: inline-flex;
  align-items: center;
}

.simulate-form :deep(.el-form-item__content) {
  display: inline-flex;
  align-items: center;
  height: 40px;
}

.simulate-form :deep(.el-form-item__label) {
  padding-right: 8px;
  line-height: 40px;
  display: inline-flex;
  align-items: center;
}

.simulate-slider {
  width: 140px;
  margin-left: 5px;
  margin-right: 10px;
  display: inline-flex;
  align-items: center;
}

/* Ensure the slider bar track is centered vertically with no extra top margins */
.simulate-form :deep(.el-slider__runway) {
  margin: 0 !important;
  flex: 1;
}

.text-danger {
  color: #f56c6c;
}
</style>
