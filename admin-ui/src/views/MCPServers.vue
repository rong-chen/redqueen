<template>
  <div class="mcp-servers-container">
    <!-- 头部横幅与介绍 -->
    <div class="mcp-header-card">
      <div class="header-overlay"></div>
      <div class="header-content">
        <h1>
          <el-icon style="vertical-align: middle; margin-right: 8px;"><Connection /></el-icon>
          外部 MCP 发现与服务管理看板
        </h1>
        <p>
          在这里，您可以动态导入并注册符合 Model Context Protocol (MCP) 规范的外部工具与数据微服务。
          系统将在启动或热刷新时通过 <strong>mark3labs/mcp-go</strong> 第三方合规标准包，自动同步并挂载其声明的所有工具，无缝注入到红皇后 AI 的决策大脑中！
        </p>
      </div>
    </div>

    <!-- 主布局：表单与表格左右排版 -->
    <div class="mcp-content-layout">
      <!-- 左侧：导入服务表单 -->
      <div class="form-section">
        <el-card class="mcp-card glass-card">
          <template #header>
            <div style="display: flex; align-items: center; gap: 6px;">
              <el-icon><Edit /></el-icon>
              <span>导入新 MCP 服务</span>
            </div>
          </template>
          <el-form :model="serverForm" :rules="serverRules" ref="serverFormRef" label-position="top" class="mcp-form">
            <el-form-item label="服务名称 (描述)" prop="name">
              <el-input 
                v-model="serverForm.name" 
                placeholder="例如：天气及室内传感器服务" 
                prefix-icon="Edit"
              />
            </el-form-item>

            <el-form-item label="服务器 Base URL" prop="base_url">
              <el-input 
                v-model="serverForm.base_url" 
                placeholder="例如：http://localhost:8080/api/mcp/rpc"
                prefix-icon="Link"
              />
            </el-form-item>

            <el-form-item label="请求方法 (Method)">
              <el-radio-group v-model="serverForm.method">
                <el-radio-button label="POST">POST (默认)</el-radio-button>
                <el-radio-button label="GET">GET</el-radio-button>
              </el-radio-group>
            </el-form-item>

            <el-form-item label="自定义 HTTP Headers (请求头列表, 可选)">
              <div class="pairs-editor">
                <div v-for="(pair, idx) in headerPairs" :key="'header-' + idx" class="pair-row">
                  <el-input v-model="pair.key" placeholder="Key (如 Header)" class="pair-input-key" />
                  <span class="pair-separator">:</span>
                  <el-input v-model="pair.value" placeholder="Value" class="pair-input-val" />
                  <el-button type="danger" icon="Delete" circle size="small" @click="removeHeaderPair(idx)" />
                </div>
                <el-button type="primary" size="small" plain icon="Plus" @click="addHeaderPair" class="add-pair-btn">
                  添加 Header
                </el-button>
              </div>
            </el-form-item>

            <el-form-item label="配置初始化参数 Params (参数配置, 可选)">
              <div class="pairs-editor">
                <div v-for="(pair, idx) in paramPairs" :key="'param-' + idx" class="pair-row">
                  <el-input v-model="pair.key" placeholder="Key (如 timeout)" class="pair-input-key" />
                  <span class="pair-separator">:</span>
                  <el-input v-model="pair.value" placeholder="Value" class="pair-input-val" />
                  <el-button type="danger" icon="Delete" circle size="small" @click="removeParamPair(idx)" />
                </div>
                <el-button type="primary" size="small" plain icon="Plus" @click="addParamPair" class="add-pair-btn">
                  添加 Parameter
                </el-button>
              </div>
            </el-form-item>

            <el-divider class="form-divider" />

            <div class="form-actions">
              <el-button 
                type="success" 
                icon="Connection"
                :loading="testLoading" 
                class="action-btn test-btn"
                @click="testNewServer"
              >
                测试连接与握手
              </el-button>
              <el-button 
                type="primary" 
                icon="Plus"
                :loading="serverSubmitLoading" 
                class="action-btn submit-btn"
                @click="handleRegisterServer"
              >
                导入到数据库
              </el-button>
            </div>
          </el-form>
        </el-card>
      </div>

      <!-- 右侧：服务列表表格 -->
      <div class="list-section">
        <el-card class="mcp-card table-card">
          <template #header>
            <div class="table-card-header">
              <span class="table-title" style="display: flex; align-items: center; gap: 6px;">
                <el-icon><List /></el-icon>
                <span>手动注册的服务清单 (实时监控)</span>
              </span>
              <el-button type="info" size="small" plain icon="Refresh" @click="fetchServers" :loading="serverLoading">
                刷新状态
              </el-button>
            </div>
          </template>

          <el-table 
            :data="serverList" 
            style="width: 100%" 
            v-loading="serverLoading" 
            stripe 
            border 
            class="custom-table"
          >
            <el-table-column prop="ID" label="ID" width="60" align="center" />
            
            <el-table-column prop="name" label="服务名称" width="180" show-overflow-tooltip>
              <template #default="scope">
                <span class="server-name-col">{{ scope.row.name }}</span>
              </template>
            </el-table-column>

            <el-table-column prop="base_url" label="服务器地址 Base URL" min-width="240" show-overflow-tooltip>
              <template #default="scope">
                <code class="code-url">{{ scope.row.base_url }}</code>
              </template>
            </el-table-column>

            <el-table-column prop="method" label="请求方法" width="100" align="center">
              <template #default="scope">
                <el-tag :type="scope.row.method === 'GET' ? 'warning' : 'primary'" size="small">
                  {{ scope.row.method || 'POST' }}
                </el-tag>
              </template>
            </el-table-column>

            <el-table-column label="在线状态" width="120" align="center">
              <template #default="scope">
                <div class="status-wrapper">
                  <span 
                    class="status-pulse" 
                    :class="scope.row.status === 'online' ? 'online' : 'offline'"
                  ></span>
                  <el-tag 
                    :type="scope.row.status === 'online' ? 'success' : 'danger'" 
                    size="small" 
                    class="status-tag"
                  >
                    {{ scope.row.status === 'online' ? '在线' : '离线' }}
                  </el-tag>
                </div>
              </template>
            </el-table-column>

            <el-table-column label="操作" width="180" align="center">
              <template #default="scope">
                <div class="btn-group">
                  <el-button 
                    type="success" 
                    size="small" 
                    plain
                    @click="testExistingServer(scope.row)"
                  >
                    测试
                  </el-button>
                  <el-button 
                    type="danger" 
                    size="small" 
                    plain
                    @click="handleDeleteServer(scope.row.ID)"
                  >
                    删除
                  </el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { ElMessage, ElMessageBox } from 'element-plus';
import request from '../utils/request';

// 数据列表和加载状态
const serverLoading = ref(false);
const testLoading = ref(false);
const serverSubmitLoading = ref(false);
const serverList = ref([]);

// 动态表格形式的 Headers 与 Params 参数编辑器数据源
const headerPairs = ref([]);
const paramPairs = ref([]);

const addHeaderPair = () => {
  headerPairs.value.push({ key: '', value: '' });
};

const removeHeaderPair = (index) => {
  headerPairs.value.splice(index, 1);
};

const addParamPair = () => {
  paramPairs.value.push({ key: '', value: '' });
};

const removeParamPair = (index) => {
  paramPairs.value.splice(index, 1);
};

// 将表格键值对序列化为 JSON 字符串传递给后端
const getSerializedJSON = (pairs) => {
  const obj = {};
  pairs.forEach(pair => {
    if (pair.key && pair.key.trim()) {
      let val = pair.value.trim();
      // 尝试对 Value 进行自动数字/布尔转换
      if (val === 'true') {
        obj[pair.key.trim()] = true;
      } else if (val === 'false') {
        obj[pair.key.trim()] = false;
      } else if (!isNaN(val) && val !== '') {
        obj[pair.key.trim()] = Number(val);
      } else {
        obj[pair.key.trim()] = pair.value;
      }
    }
  });
  return JSON.stringify(obj);
};

// 绑定表单与校验规则
const serverFormRef = ref(null);
const serverForm = ref({
  name: '本地模拟 MCP 传感器',
  base_url: 'http://localhost:8080/api/mcp', // 示例外部 MCP 服务地址
  method: 'POST',
  headers: '{}',
  params: '{}',
});

const serverRules = {
  name: [{ required: true, message: '请输入服务名称描述', trigger: 'blur' }],
  base_url: [{ required: true, message: '请输入服务器 Base URL 端点', trigger: 'blur' }],
};

// 获取服务列表
const fetchServers = async () => {
  serverLoading.value = true;
  try {
    const response = await request.get('/mcp/servers');
    serverList.value = response.data || [];
  } catch (error) {
    console.error(error);
  } finally {
    serverLoading.value = false;
  }
};

// 1. 握手测试连接 (新输入的临时配置)
const testNewServer = async () => {
  if (!serverFormRef.value) return;

  await serverFormRef.value.validate(async (valid) => {
    if (valid) {
      testLoading.value = true;
      try {
        const res = await request.post('/mcp/servers/test', {
          base_url: serverForm.value.base_url,
          method: serverForm.value.method,
          headers: getSerializedJSON(headerPairs.value),
          params: getSerializedJSON(paramPairs.value),
        });
        
        if (res.status === 'online') {
          ElMessageBox.alert(res.message, 'MCP 握手测试成功', { 
            type: 'success',
            confirmButtonText: '确定'
          });
        } else {
          ElMessageBox.alert(res.message, '连接失败', { 
            type: 'warning',
            confirmButtonText: '确定'
          });
        }
      } catch (error) {
        console.error(error);
      } finally {
        testLoading.value = false;
      }
    }
  });
};

// 2. 测试现有列表中存在的服务 (自动刷新最新状态)
const testExistingServer = async (server) => {
  try {
    ElMessage.info(`正在实时探测外部服务 [${server.name}]...`);
    const res = await request.post('/mcp/servers/test', {
      base_url: server.base_url,
      method: server.method || 'POST',
      headers: server.headers,
      params: server.params,
    });
    
    if (res.status === 'online') {
      ElMessage.success(`[${server.name}] 握手测试通过，服务状态更新为: 在线`);
    } else {
      ElMessage.error(`[${server.name}] 握手异常: ${res.message}`);
    }
    fetchServers(); // 刷新表格
  } catch (error) {
    console.error(error);
  }
};

// 3. 注册新 MCP 服务入库
const handleRegisterServer = async () => {
  if (!serverFormRef.value) return;

  await serverFormRef.value.validate(async (valid) => {
    if (valid) {
      serverSubmitLoading.value = true;
      try {
        const res = await request.post('/mcp/servers', {
          name: serverForm.value.name,
          base_url: serverForm.value.base_url,
          method: serverForm.value.method,
          headers: getSerializedJSON(headerPairs.value),
          params: getSerializedJSON(paramPairs.value),
        });
        
        ElMessage.success(res.message || '外部 MCP 服务导入并注册成功！');
        
        // 重置表单
        serverForm.value = {
          name: '',
          base_url: '',
          method: 'POST',
          headers: '{}',
          params: '{}',
        };
        headerPairs.value = [];
        paramPairs.value = [];
        fetchServers();
      } catch (error) {
        console.error(error);
      } finally {
        serverSubmitLoading.value = false;
      }
    }
  });
};

// 4. 删除 MCP 服务配置
const handleDeleteServer = (id) => {
  ElMessageBox.confirm(
    '确认要彻底删除该外部 MCP 服务吗？这会导致其注册的所有工具从红皇后的 LLM 大脑中立即注销下线。',
    '警告',
    {
      confirmButtonText: '确定删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    }
  ).then(async () => {
    try {
      await request.delete(`/mcp/servers/${id}`);
      ElMessage.success('外部 MCP 服务已成功卸载并删除');
      fetchServers();
    } catch (error) {
      console.error(error);
    }
  }).catch(() => {});
};

onMounted(() => {
  fetchServers();
});
</script>

<style scoped>
.mcp-servers-container {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

/* 顶部玻璃感渐变横幅 */
.mcp-header-card {
  position: relative;
  border-radius: 12px;
  background: linear-gradient(135deg, #1f2d3d 0%, #304156 100%);
  padding: 30px 40px;
  color: #ffffff;
  overflow: hidden;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.08);
}

.header-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: radial-gradient(circle at 80% 20%, rgba(64, 158, 255, 0.15) 0%, transparent 50%);
  pointer-events: none;
}

.header-content h1 {
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 10px 0;
  letter-spacing: 0.5px;
}

.header-content p {
  font-size: 14px;
  margin: 0;
  color: #bfcbd9;
  line-height: 1.6;
}

/* 主内容布局 */
.mcp-content-layout {
  display: grid;
  grid-template-columns: 380px 1fr;
  gap: 20px;
  align-items: start;
}

.mcp-card {
  border-radius: 10px;
  border: none;
  box-shadow: 0 4px 16px rgba(0,0,0,0.03);
  transition: all 0.3s;
}

.mcp-card:hover {
  box-shadow: 0 6px 24px rgba(0,0,0,0.06);
}

.glass-card {
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid rgba(235, 238, 245, 0.6);
}

.mcp-form :deep(.el-form-item__label) {
  font-weight: 600;
  color: #48576a;
  padding-bottom: 4px;
}

.pairs-editor {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
}

.pair-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.pair-input-key {
  width: 110px !important;
  flex-shrink: 0;
}

.pair-separator {
  color: #909399;
  font-weight: bold;
}

.pair-input-val {
  flex: 1;
}

.add-pair-btn {
  align-self: flex-start;
  margin-top: 5px;
}

.json-textarea :deep(.el-textarea__inner) {
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  background-color: #f7f9fc;
  border-color: #e4e7ed;
  color: #2c3e50;
}

.form-divider {
  margin: 20px 0;
}

.form-actions {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.action-btn {
  width: 100%;
  margin-left: 0 !important;
  font-weight: bold;
  height: 38px;
  border-radius: 6px;
}

.table-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.table-title {
  font-weight: bold;
  color: #303133;
  font-size: 16px;
}

.custom-table {
  border-radius: 8px;
  overflow: hidden;
}

.server-name-col {
  font-weight: bold;
  color: #303133;
}

.code-url {
  background-color: #f0f2f5;
  padding: 4px 8px;
  border-radius: 4px;
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  color: #606266;
  border: 1px solid #e4e7ed;
}

/* 状态信号圆点与波纹 */
.status-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.status-pulse {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  position: relative;
}

.status-pulse.online {
  background-color: #67c23a;
  box-shadow: 0 0 8px rgba(103, 194, 58, 0.6);
}

.status-pulse.online::after {
  content: '';
  position: absolute;
  top: -2px;
  left: -2px;
  right: -2px;
  bottom: -2px;
  border-radius: 50%;
  border: 2px solid #67c23a;
  animation: pulse 1.6s infinite ease-in-out;
  opacity: 0;
}

.status-pulse.offline {
  background-color: #f56c6c;
  box-shadow: 0 0 8px rgba(245, 108, 108, 0.4);
}

@keyframes pulse {
  0% {
    transform: scale(0.8);
    opacity: 0.5;
  }
  100% {
    transform: scale(2.2);
    opacity: 0;
  }
}

.status-tag {
  font-weight: bold;
  border-radius: 4px;
}

.btn-group {
  display: flex;
  gap: 8px;
  justify-content: center;
}

@media (max-width: 1024px) {
  .mcp-content-layout {
    grid-template-columns: 1fr;
  }
}
</style>
