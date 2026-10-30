<template>
  <div class="install-wrap">
    <div class="install-card">
      <div class="install-title">未检测到 aria2</div>
      <div class="install-desc">AriaDesk 需要 aria2 作为下载内核，点击下方按钮通过 winget 一键安装</div>
      <div v-if="uninstalled" class="install-done">aria2 已卸载，可重新安装</div>

      <template v-if="!installDone">
        <button class="btn btn-primary btn-lg" :disabled="installing" @click="onInstall">
          <span v-if="installing" class="spinner"></span>
          {{ installing ? '安装中…' : '一键安装 aria2' }}
        </button>
        <div v-if="installing" class="progress-block">
          <div class="progress-info">
            <span class="progress-text">{{ lastInstallLine }}</span>
          </div>
          <div class="progress-track">
            <div class="progress-fill indeterminate"></div>
          </div>
        </div>
        <div v-if="store.install.lines.length" ref="logRef" class="install-log">{{ logText }}</div>
        <div v-if="store.install.phase === 'error'" class="install-error">
          安装失败：{{ store.install.error || '未知错误' }}，请重试或手动安装 aria2
        </div>
      </template>

      <template v-else>
        <div class="install-done">aria2c {{ store.detect.version }} 已安装</div>
        <button class="btn btn-primary btn-lg" :disabled="starting" @click="onStart">
          <span v-if="starting" class="spinner"></span>
          启动 aria2
        </button>
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { useAppStore } from '../stores/app'

const store = useAppStore()
const logRef = ref(null)
const starting = ref(false)

const installDone = computed(() => store.install.phase === 'done' || store.detect.installed)
const installing = computed(() => store.install.phase === 'running')
const uninstalled = computed(() => store.update.phase === 'done' && !store.update.target)
const logText = computed(() => store.install.lines.slice(-30).join('\n'))
const lastInstallLine = computed(() => {
  const lines = store.install.lines
  for (let i = lines.length - 1; i >= 0; i--) {
    if (lines[i]) return lines[i]
  }
  return '正在通过 winget 安装…'
})

watch(logText, async () => {
  await nextTick()
  if (logRef.value) logRef.value.scrollTop = logRef.value.scrollHeight
})

async function onInstall() {
  await store.installAria2()
}

async function onStart() {
  starting.value = true
  try {
    await store.startAria2()
    await store.connect()
  } catch (e) {
    ElMessage.error(`启动失败：${e}`)
  } finally {
    starting.value = false
  }
}
</script>
