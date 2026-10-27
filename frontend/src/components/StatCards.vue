<template>
  <div class="stats">
    <div class="stat-card">
      <div class="stat-icon brand">
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <path d="M10 3.5v9M10 12.5L6.3 9.2M10 12.5l3.7-3.3M4.5 16h11" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </div>
      <div class="stat-info">
        <div class="label">当前下载速率</div>
        <div class="value">{{ downloadSpeed }}</div>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon brand">
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <path d="M10 16.5v-9M10 7.5l-3.7 3.3M10 7.5l3.7 3.3M4.5 4h11" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </div>
      <div class="stat-info">
        <div class="label">当前上传速率</div>
        <div class="value">{{ uploadSpeed }}</div>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon success">
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <path d="M11 2L4.5 11H9l-1 7L14.5 8H10l1-6z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
        </svg>
      </div>
      <div class="stat-info">
        <div class="label">进行中任务</div>
        <div class="value">{{ numActive }}</div>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon warning">
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
          <circle cx="10" cy="11" r="6" stroke="currentColor" stroke-width="1.5"/>
          <path d="M10 7.5V11l2.5 1.8M8 1.5h4M10 1.5v3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
        </svg>
      </div>
      <div class="stat-info">
        <div class="label">等待 / 已暂停</div>
        <div class="value">{{ numWaiting }}</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useAppStore } from '../stores/app'
import { formatSpeed, toNum } from '../utils/format'

const store = useAppStore()

const stat = computed(() => (store.connected ? store.stat : null))
const downloadSpeed = computed(() => formatSpeed(stat.value?.downloadSpeed ?? 0))
const uploadSpeed = computed(() => formatSpeed(stat.value?.uploadSpeed ?? 0))
const numActive = computed(() => (stat.value ? toNum(stat.value.numActive) : 0))
const numWaiting = computed(() => (stat.value ? toNum(stat.value.numWaiting) : 0))
</script>
