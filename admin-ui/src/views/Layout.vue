<template>
  <el-container class="layout-container">
    <!-- 侧边栏菜单 -->
    <el-aside width="240px" class="layout-aside">
      <div class="aside-logo">
        <span class="logo-text">RedQueen Admin</span>
      </div>
      <el-menu
        :default-active="activeMenu"
        class="aside-menu"
        background-color="#304156"
        text-color="#bfcbd9"
        active-text-color="#409EFF"
        router
      >
        <el-menu-item index="/voice-logs">
          <el-icon><Microphone /></el-icon>
          <span>语音交互日志</span>
        </el-menu-item>
        <el-menu-item index="/voice-live">
          <el-icon><Headset /></el-icon>
          <span>实时语音交互</span>
        </el-menu-item>

        <el-menu-item index="/mcp-servers">
          <el-icon><Connection /></el-icon>
          <span>外部 MCP 服务</span>
        </el-menu-item>
        <el-menu-item index="/model-config">
          <el-icon><Setting /></el-icon>
          <span>模型参数配置</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <!-- 右侧主操作区 -->
    <el-container>
      <!-- 页头 -->
      <el-header class="layout-header">
        <div class="header-left">
          <span class="breadcrumb">{{ currentRouteName }}</span>
        </div>
        <div class="header-right">
          <el-dropdown trigger="click">
            <span class="user-info">
              <el-avatar :size="32" class="user-avatar">A</el-avatar>
              <span class="username">{{ username }}</span>
              <el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>角色: {{ role }}</el-dropdown-item>
                <el-dropdown-item divided @click="handleLogout">
                  安全退出
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <!-- 主体内容卡片 -->
      <el-main class="layout-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Microphone, Headset, Setting, Connection, ArrowDown } from '@element-plus/icons-vue';
import { ElMessageBox, ElMessage } from 'element-plus';

const route = useRoute();
const router = useRouter();

const username = ref('Admin');
const role = ref('Administrator');

// 获取当前激活菜单项路径
const activeMenu = computed(() => {
  return route.path;
});

// 计算当前页面中文面包屑
const currentRouteName = computed(() => {
  if (route.path === '/voice-logs') {
    return '语音交互指令历史日志';
  } else if (route.path === '/voice-live') {
    return '实时语音交互 — 红皇后';
  } else if (route.path === '/mcp-servers') {
    return '外部 MCP 发现与服务管理看板';
  } else if (route.path === '/model-config') {
    return '语音与 NLU 大模型参数配置';
  }
  return '首页';
});

// 加载本地用户信息
onMounted(() => {
  const userStr = localStorage.getItem('rq_user');
  if (userStr) {
    try {
      const user = JSON.parse(userStr);
      username.value = user.username;
      role.value = user.role === 'admin' ? '系统超级管理员' : '普通操作员';
    } catch (e) {
      console.error(e);
    }
  }
});

// 安全登出逻辑
const handleLogout = () => {
  ElMessageBox.confirm('您确定要退出 RedQueen 系统管理后台吗?', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning',
  }).then(() => {
    localStorage.removeItem('rq_token');
    localStorage.removeItem('rq_user');
    ElMessage.success('已安全退出登录');
    router.push('/login');
  }).catch(() => {});
};
</script>

<style scoped>
.layout-container {
  height: 100vh;
}

.layout-aside {
  background-color: #304156;
  color: #fff;
  display: flex;
  flex-direction: column;
}

.aside-logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #2b2f3a;
  border-bottom: 1px solid #1f2d3d;
}

.logo-text {
  font-size: 18px;
  font-weight: 600;
  color: #fff;
  letter-spacing: 0.5px;
}

.aside-menu {
  border-right: none;
  flex: 1;
}

.layout-header {
  background-color: #fff;
  border-bottom: 1px solid #e6e6e6;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 20px;
  height: 60px;
}

.breadcrumb {
  font-size: 16px;
  font-weight: 500;
  color: #303133;
}

.user-info {
  display: flex;
  align-items: center;
  cursor: pointer;
}

.user-avatar {
  background-color: #409eff;
  color: #fff;
  margin-right: 8px;
  font-weight: bold;
}

.username {
  font-size: 14px;
  color: #606266;
}

.layout-main {
  background-color: #f0f2f5;
  padding: 20px;
}
</style>
