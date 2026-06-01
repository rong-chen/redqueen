import axios from 'axios';
import { ElMessage } from 'element-plus';

// 创建 axios 实例
const request = axios.create({
  baseURL: 'http://localhost:9091/api', // Golang 后端接口根路径
  timeout: 5000,
});

// 请求拦截器: 自动注入 Token 凭证
request.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('rq_token');
    if (token) {
      config.headers['Authorization'] = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 响应拦截器: 统一拦截并提示 API 错误
request.interceptors.response.use(
  (response) => {
    const res = response.data;
    // 适配后端统一响应结构: code 为 200 或 201 表示正常
    if (res.code === 200 || res.code === 201) {
      return res;
    }
    ElMessage.error(res.message || '系统错误');
    return Promise.reject(new Error(res.message || 'Error'));
  },
  (error) => {
    if (error.response) {
      const status = error.response.status;
      const data = error.response.data;

      // 如果返回 401 Unauthorized，清除无效 Token 并跳转回登录页
      if (status === 401) {
        ElMessage.error(data.message || '登录会话已失效，请重新登录');
        localStorage.removeItem('rq_token');
        localStorage.removeItem('rq_user');
        window.location.hash = '#/login'; // 强行跳回 Hash 登录页
      } else {
        ElMessage.error(data.message || '服务器内部错误');
      }
    } else {
      ElMessage.error('无法连接到后端网关服务，请检查服务器是否正常运行');
    }
    return Promise.reject(error);
  }
);

export default request;
