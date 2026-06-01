<template>
  <el-config-provider>
    <router-view />
  </el-config-provider>
</template>

<script setup>
// 页面根模板，仅挂载路由渲染视图
</script>

<style>
/* 全局页面样式重置 */
body {
  margin: 0;
  padding: 0;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}
</style>
