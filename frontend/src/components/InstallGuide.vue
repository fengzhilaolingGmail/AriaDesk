<template>
  <div class="install-wrap">
    <div class="install-card">
      <div class="install-title">未检测到 aria2</div>
      <div class="install-desc">AriaDesk 需要 aria2 作为下载内核，点击下方按钮通过 winget 一键安装</div>

      <template v-if="!installDone">
        <button class="btn btn-primary btn-lg" :disabled="store.install.running" @click="onInstall">
          <span v-if="store.install.running" class="spinner"></span>
          {{ store.install.running ? '安装中…' : '一键安装 aria2' }}
        </button>
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
const logText = computed(() => store.install.lines.slice(-30).join('\n'))

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
