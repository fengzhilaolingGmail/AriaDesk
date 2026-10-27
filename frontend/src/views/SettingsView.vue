<template>
  <div class="container container-narrow">
    <!-- 启动 -->
    <div class="form-card">
      <div class="section-title has-desc">启动</div>
      <div class="section-desc">控制应用与 aria2 的启动行为</div>

      <div class="setting-row">
        <div class="setting-main">
          <div class="setting-label">开机自启动</div>
          <div class="setting-desc">登录 Windows 后自动启动 AriaDesk（最小化到托盘）</div>
        </div>
        <button class="switch" :class="{ off: !store.settings.autoStartApp }" @click="toggle('autoStartApp')">
          <span class="knob"></span>
        </button>
      </div>

      <div class="setting-row">
        <div class="setting-main">
          <div class="setting-label">aria2 随应用启动</div>
          <div class="setting-desc">打开 AriaDesk 后自动启动 aria2 并恢复上次任务</div>
        </div>
        <button class="switch" :class="{ off: !store.settings.autoStartAria2 }" @click="toggle('autoStartAria2')">
          <span class="knob"></span>
        </button>
      </div>
    </div>

    <!-- 关闭行为 -->
    <div class="form-card">
      <div class="section-title has-desc">关闭行为</div>
      <div class="section-desc">点击窗口右上角关闭按钮时执行的操作</div>

      <div class="radio-group">
        <div class="radio-item" @click="setCloseToTray(false)">
          <div class="radio" :class="{ checked: !store.settings.closeToTray }"></div>
          <div class="radio-text">
            <div class="radio-label">直接退出</div>
            <div class="radio-desc">关闭窗口并退出程序，同时停止 aria2</div>
          </div>
        </div>
        <div class="radio-item" @click="setCloseToTray(true)">
          <div class="radio" :class="{ checked: store.settings.closeToTray }"></div>
          <div class="radio-text">
            <div class="radio-label">隐藏到托盘</div>
            <div class="radio-desc">窗口隐藏到系统托盘，aria2 继续在后台下载</div>
          </div>
        </div>
      </div>
    </div>

    <!-- 关于 -->
    <div class="form-card">
      <div class="section-title">关于</div>

      <div class="kv-row">
        <div class="kv-key">AriaDesk 版本</div>
        <div class="kv-val"><span class="mono">{{ about?.appVersion || '—' }}</span></div>
      </div>
      <div class="kv-row">
        <div class="kv-key">aria2 版本</div>
        <div class="kv-val"><span class="mono">{{ about?.aria2Version || '未安装' }}</span></div>
      </div>
      <div class="kv-row">
        <div class="kv-key">数据目录</div>
        <div class="kv-val"><span class="mono">{{ about?.dataDir || '—' }}</span></div>
      </div>

      <div class="kv-actions">
        <button class="btn btn-default" @click="openLog">查看日志</button>
        <button class="btn btn-default" @click="openDataDir">打开数据目录</button>
      </div>
    </div>

    <!-- 日志弹窗 -->
    <el-dialog v-model="logVisible" title="aria2 日志" width="640px">
      <pre class="log-view">{{ logText || '暂无日志内容' }}</pre>
      <template #footer>
        <button class="btn btn-default" @click="logVisible = false">关闭</button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { useAppStore } from '../stores/app'

const store = useAppStore()

const about = ref(null)
const logVisible = ref(false)
const logText = ref('')

onMounted(async () => {
  try {
    about.value = await store.loadAbout()
  } catch { /* 忽略，保持占位 */ }
})

async function toggle(key) {
  const prev = store.settings[key]
  try {
    await store.saveSettings({ [key]: !prev })
  } catch (e) {
    ElMessage.error(`保存失败：${e}`)
  }
}

async function setCloseToTray(v) {
  if (store.settings.closeToTray === v) return
  try {
    await store.saveSettings({ closeToTray: v })
  } catch (e) {
    ElMessage.error(`保存失败：${e}`)
  }
}

async function openLog() {
  try {
    logText.value = await store.readLog()
    logVisible.value = true
  } catch (e) {
    ElMessage.error(String(e))
  }
}

async function openDataDir() {
  try {
    await store.openDataDir()
  } catch (e) {
    ElMessage.error(String(e))
  }
}
</script>
