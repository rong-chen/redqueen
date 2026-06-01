import { createApp } from 'vue';
import App from './App.vue';
import router from './router';

// 引入 Element Plus 统一的 UI 组件及样式
import ElementPlus from 'element-plus';
import 'element-plus/dist/index.css';

// 引入并注册所有的 Element Plus 图标组件
import * as ElementPlusIconsVue from '@element-plus/icons-vue';

const app = createApp(App);

// 循环注册所有图标组件
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component);
}

// 载入路由、组件包，并挂载 DOM 渲染
app.use(router);
app.use(ElementPlus);
app.mount('#app');
