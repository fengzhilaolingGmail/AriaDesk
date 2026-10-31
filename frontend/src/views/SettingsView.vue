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

    <!-- 下载历史 -->
    <div class="form-card">
      <div class="section-title has-desc">下载历史</div>
      <div class="section-desc">本地保存已完成 / 停止任务的记录（aria2 本身不保留）</div>

      <div class="setting-row">
        <div class="setting-main">
          <div class="setting-label">历史记录条数上限</div>
          <div class="setting-desc">超过上限后自动清理最旧的记录，最多 5000 条</div>
        </div>
        <select class="pager-select" :value="store.settings.historyMax" @change="onHistoryMaxChange">
          <option v-for="n in [100, 500, 1000, 2000, 5000]" :key="n" :value="n">{{ n }} 条</option>
        </select>
      </div>
    </div>

    <!-- 浏览器插件 -->
    <div class="form-card">
      <div class="section-title has-desc">浏览器插件</div>
      <div class="section-desc">接收浏览器插件推送的下载任务（普通链接 / 磁力 / 种子），插件通过本地端口与 AriaDesk 通信</div>

      <div class="setting-row">
        <div class="setting-main">
          <div class="setting-label">启用浏览器联动</div>
          <div class="setting-desc">在本机 20000 ~ 20005 端口范围内监听插件推送服务</div>
        </div>
        <button class="switch" :class="{ off: !store.settings.browserExtEnabled }" @click="toggle('browserExtEnabled')">
          <span class="knob"></span>
        </button>
      </div>

      <div class="kv-row">
        <div class="kv-key">服务状态</div>
        <div class="kv-val">
          <span v-if="!store.settings.browserExtEnabled">已停用</span>
          <span v-else-if="extInfo?.running">监听中，端口 <span class="mono">{{ extInfo.port }}</span></span>
          <span v-else>未监听（20000 ~ 20005 均被占用）</span>
        </div>
      </div>

      <div class="kv-row">
        <div class="kv-key">配对 token</div>
        <div class="kv-val">
          <span class="mono">{{ extTokenRevealed ? extToken : '••••••••••••' }}</span>
        </div>
      </div>

      <div class="kv-actions">
        <button class="btn btn-default" @click="toggleExtToken">{{ extTokenRevealed ? '隐藏' : '显示' }} token</button>
        <button class="btn btn-default" :disabled="extTokenBusy" @click="onRegenExtToken">重新生成</button>
      </div>
      <div class="setting-desc">token 用于插件与软件配对，需填写到插件设置页；重新生成后旧 token 立即失效</div>
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
const extInfo = ref(null)
const extToken = ref('')
const extTokenRevealed = ref(false)
const extTokenBusy = ref(false)

onMounted(async () => {
  try {
    about.value = await store.loadAbout()
  } catch { /* 忽略，保持占位 */ }
  await refreshExtInfo()
})

async function refreshExtInfo() {
  try {
    extInfo.value = await store.getExtInfo()
  } catch { /* 保持占位 */ }
}

async function toggle(key) {
  const prev = store.settings[key]
  try {
    await store.saveSettings({ [key]: !prev })
    if (key === 'browserExtEnabled') await refreshExtInfo()
  } catch (e) {
    ElMessage.error(`保存失败：${e}`)
  }
}

async function toggleExtToken() {
  if (extTokenRevealed.value) {
    extTokenRevealed.value = false
    extToken.value = ''
    return
  }
  try {
    extToken.value = await store.revealExtToken()
    extTokenRevealed.value = true
  } catch (e) {
    ElMessage.error(String(e))
  }
}

async function onRegenExtToken() {
  extTokenBusy.value = true
  try {
    await store.regenerateExtToken()
    extTokenRevealed.value = false
    extToken.value = ''
    ElMessage.success('已重新生成，请在插件设置中重新填写 token')
  } catch (e) {
    ElMessage.error(String(e))
  } finally {
    extTokenBusy.value = false
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

async function onHistoryMaxChange(e) {
  const v = Number(e.target.value)
  if (!v || v === store.settings.historyMax) return
  try {
    await store.saveSettings({ historyMax: v })
    ElMessage.success('已保存')
  } catch (err) {
    ElMessage.error(`保存失败：${err}`)
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
