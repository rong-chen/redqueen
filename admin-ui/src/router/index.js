import { createRouter, createWebHashHistory } from 'vue-router';
import Login from '../views/Login.vue';
import Layout from '../views/Layout.vue';
import VoiceLogs from '../views/VoiceLogs.vue';
import VoiceLive from '../views/VoiceLive.vue';
import ModelConfig from '../views/ModelConfig.vue';
import MCPServers from '../views/MCPServers.vue';
import VoiceprintEnroll from '../views/VoiceprintEnroll.vue';

// 配置后台路由列表
const routes = [
  {
    path: '/login',
    name: 'Login',
    component: Login,
  },
  {
    path: '/',
    component: Layout,
    redirect: '/voice-logs', // 登录后默认重定向至语音交互日志
    children: [
      {
        path: 'voice-logs',
        name: 'VoiceLogs',
        component: VoiceLogs,
      },
      {
        path: 'voice-live',
        name: 'VoiceLive',
        component: VoiceLive,
      },

      {
        path: 'mcp-servers',
        name: 'MCPServers',
        component: MCPServers,
      },
      {
        path: 'model-config',
        name: 'ModelConfig',
        component: ModelConfig,
      },
      {
        path: 'voiceprint-enroll',
        name: 'VoiceprintEnroll',
        component: VoiceprintEnroll,
      },
    ],
  },
];

const router = createRouter({
  history: createWebHashHistory(),
  routes,
});

// 全局路由守卫: 拦截未登录请求并跳转回登录页
router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('rq_token');
  if (to.path !== '/login' && !token) {
    next('/login');
  } else {
    next();
  }
});

export default router;
