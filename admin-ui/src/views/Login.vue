<template>
  <div class="login-container">
    <el-card class="login-card">
      <div class="login-header">
        <h2 class="title">RedQueenSystem</h2>
        <p class="subtitle">语音与 MCP 协同管理平台</p>
      </div>

      <el-form :model="loginForm" :rules="rules" ref="loginFormRef" size="large">
        <el-form-item prop="username">
          <el-input
            v-model="loginForm.username"
            placeholder="请输入用户名"
            prefix-icon="User"
          ></el-input>
        </el-form-item>

        <el-form-item prop="password">
          <el-input
            v-model="loginForm.password"
            type="password"
            placeholder="请输入密码"
            prefix-icon="Lock"
            show-password
            @keyup.enter="handleLogin"
          ></el-input>
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            class="login-btn"
            :loading="loading"
            @click="handleLogin"
          >
            登 录
          </el-button>
        </el-form-item>
      </el-form>

      <div class="login-tips">
        <el-alert
          title="系统安全提示"
          type="info"
          description="首次启动服务已自动生成默认超级管理员账号 admin，如需查看默认密码请遵循内部安全规范。"
          show-icon
          :closable="false"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { ElMessage } from 'element-plus';
import request from '../utils/request';
import { User, Lock } from '@element-plus/icons-vue';

const router = useRouter();
const loginFormRef = ref(null);
const loading = ref(false);

const loginForm = ref({
  username: 'admin', // 默认回填供快速开发调试
  password: '',
});

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
};

const handleLogin = async () => {
  if (!loginFormRef.value) return;

  await loginFormRef.value.validate(async (valid) => {
    if (valid) {
      loading.value = true;
      try {
        const response = await request.post('/auth/login', {
          username: loginForm.value.username,
          password: loginForm.value.password,
        });

        // 存储 Token 与用户信息至本地
        localStorage.setItem('rq_token', response.data.token);
        localStorage.setItem('rq_user', JSON.stringify(response.data.user));

        ElMessage.success({
          message: response.message || '欢迎回来！登录成功',
          type: 'success',
        });
        
        router.push('/');
      } catch (error) {
        console.error(error);
      } finally {
        loading.value = false;
      }
    }
  });
};
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background-color: #f5f7fa;
}

.login-card {
  width: 420px;
  padding: 20px;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
}

.login-header {
  text-align: center;
  margin-bottom: 30px;
}

.title {
  margin: 0;
  font-size: 26px;
  color: #303133;
  font-weight: 600;
}

.subtitle {
  margin: 8px 0 0 0;
  font-size: 14px;
  color: #909399;
}

.login-btn {
  width: 100%;
  margin-top: 10px;
}

.login-tips {
  margin-top: 20px;
}
</style>
