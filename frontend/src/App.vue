<!--
 * @Author: fengzhilaoling fengzhilaoling@gmail.com
 * @Date: 2026-10-27 14:58:46
 * @LastEditors: fengzhilaoling
 * @LastEditTime: 2026-10-31 12:40:01
 * @FilePath: \AriaDesk\frontend\src\App.vue
 * @Description: 文件解释
 * Copyright (c) 2026 by fengzhilaoling@gmail.com, All Rights Reserved.
-->
<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="side-logo">
        <img class="logo-img" :src="iconUrl" alt="AriaDesk" width="20" height="20" />
        <span class="app-name">AriaDesk</span>
      </div>
      <nav class="nav">
        <router-link to="/" class="nav-item" exact-active-class="active">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M3 7l5-4.5L13 7M4.5 6v6h7V6" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          首页
        </router-link>
        <router-link to="/conf" class="nav-item" active-class="active">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M4 2.5h5.5L12 5v8.5H4v-11z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/>
            <path d="M9.5 2.5V5H12M6 8h4M6 10.5h4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
          </svg>
          配置
        </router-link>
        <router-link to="/manage" class="nav-item" active-class="active">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M2 4.5h12M2 8h12M2 11.5h12" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>
            <circle cx="5" cy="4.5" r="1.6" fill="var(--surface)" stroke="currentColor" stroke-width="1.2"/>
            <circle cx="10" cy="8" r="1.6" fill="var(--surface)" stroke="currentColor" stroke-width="1.2"/>
            <circle cx="6.5" cy="11.5" r="1.6" fill="var(--surface)" stroke="currentColor" stroke-width="1.2"/>
          </svg>
          管理
        </router-link>
        <router-link to="/settings" class="nav-item" active-class="active">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <circle cx="8" cy="8" r="2.2" stroke="currentColor" stroke-width="1.3"/>
            <path d="M8 1.8v1.7M8 12.5v1.7M1.8 8h1.7M12.5 8h1.7M3.6 3.6l1.2 1.2M11.2 11.2l1.2 1.2M12.4 3.6l-1.2 1.2M4.8 11.2l-1.2 1.2" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>
          </svg>
          设置
        </router-link>
      </nav>
    </aside>
    <main class="main">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { useAppStore } from './stores/app'
import iconUrl from './assets/icon.png'

const store = useAppStore()
const router = useRouter()

onMounted(async () => {
  store.registerEvents()
  // 收到浏览器插件推送时切换到首页（下载列表页）
  EventsOn('task:ext-added', () => {
    if (router.currentRoute.value.path !== '/') router.push('/')
  })
  await store.init()
})
</script>
