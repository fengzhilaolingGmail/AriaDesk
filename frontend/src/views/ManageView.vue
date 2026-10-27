<template>
  <div class="container container-narrow">
    <InstallGuide v-if="!store.loading && !store.detect.installed" />

    <template v-else>
      <!-- aria2 状态 -->
      <div class="form-card">
        <div class="section-title">aria2</div>

        <div class="status-row">
          <div class="status-icon" :class="store.detect.installed ? 'success' : 'muted'">
            <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
              <circle cx="9" cy="9" r="6.5" stroke="currentColor" stroke-width="1.4"/>
              <path d="M6 9.2l2 2 4-4.4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </div>
          <div class="status-main">
            <div class="status-label">
              <template v-if="store.detect.installed">已安装 aria2 <span class="mono">{{ store.detect.version }}</span></template>
              <template v-else>未安装 aria2</template>
            </div>
            <div class="status-desc">{{ store.detect.installed ? '路径已缓存' : '请先完成安装' }}</div>
          </div>
          <div class="run-badge" :class="store.running ? 'running' : 'stopped'">
            <span class="dot"></span>{{ store.running ? '运行中' : '未运行' }}
          </div>
        </div>

        <div class="status-row">
          <div class="status-icon brand">
            <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
              <path d="M4 9a5 5 0 015-5h5M14 9a5 5 0 01-5 5H4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>
              <path d="M12.6 2.4L15 4l-2.4 1.6M5.4 15.6L3 14l2.4-1.6" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </div>
          <div class="status-main">
            <div class="status-label">进程控制</div>
            <div class="status-desc">
              {{ store.settings.autoStartAria2 ? '随应用启动已开启' : '随应用启动已关闭' }}，监听端口
              <span class="mono">{{ store.settings.rpcPort }}</span>
            </div>
          </div>
          <button v-if="!store.running" class="btn btn-default" :disabled="!store.detect.installed || procBusy" @click="onStart">
            <span v-if="procBusy" class="spinner"></span>启动
          </button>
          <button v-else class="btn btn-default" :disabled="procBusy" @click="onStop">
            <span v-if="procBusy" class="spinner"></span>停止
          </button>
        </div>
      </div>

      <!-- RPC -->
      <div class="form-card">
        <div class="section-title">RPC</div>

        <div class="form-row">
          <div class="form-label">RPC 密钥 (Token)</div>
          <div class="input-wrap">
            <div class="input readonly">
              <span class="mono">{{ tokenDisplay }}</span>
              <span class="suffix-icon" :title="tokenRevealed ? '隐藏' : '显示'" @click="toggleReveal">
                <svg v-if="!tokenRevealed" width="14" height="14" viewBox="0 0 14 14" fill="none">
                  <path d="M1.5 7s2.2-3.5 5.5-3.5S12.5 7 12.5 7 10.3 10.5 7 10.5 1.5 7 1.5 7z" stroke="currentColor" stroke-width="1.2"/>
                  <circle cx="7" cy="7" r="1.8" stroke="currentColor" stroke-width="1.2"/>
                </svg>
                <svg v-else width="14" height="14" viewBox="0 0 14 14" fill="none">
                  <path d="M2.5 2.5l9 9M6 3.6A5.4 5.4 0 017 3.5c3.3 0 5.5 3.5 5.5 3.5a10.6 10.6 0 01-1.7 2.2M8 10.4a5.4 5.4 0 01-1 .1c-3.3 0-5.5-3.5-5.5-3.5a10.6 10.6 0 011.7-2.2" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
                </svg>
              </span>
            </div>
            <button class="btn btn-default" :disabled="regenerating" @click="onRegenerate">重新生成</button>
          </div>
          <div class="form-tip">保存在 %APPDATA%\AriaDesk\token.pass，已加密存储</div>
        </div>

        <div class="form-row">
          <div class="form-label">RPC 端口</div>
          <div class="input-wrap">
            <input class="input mono w-fixed" :value="store.settings.rpcPort" @change="onPortChange" />
          </div>
          <div class="form-tip">修改后需重启 aria2 生效</div>
        </div>
      </div>

      <!-- 下载 -->
      <div class="form-card">
        <div class="section-title">下载</div>

        <div class="form-row">
          <div class="form-label">默认下载目录</div>
          <div class="input-wrap">
            <input v-model="downloadDir" class="input mono" placeholder="C:\Users\...\Downloads" />
            <button class="btn btn-default" @click="onBrowse">浏览…</button>
          </div>
          <div class="form-tip">保存时校验目录存在且可写，仅影响之后新建的任务</div>
          <div v-if="saveError" class="form-error">{{ saveError }}</div>
        </div>

        <div class="form-actions">
          <button class="btn btn-primary" :disabled="saving" @click="onSave">
            <span v-if="saving" class="spinner"></span>保存修改
          </button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { useAppStore } from '../stores/app'
import InstallGuide from '../components/InstallGuide.vue'

const store = useAppStore()

const tokenRevealed = ref(false)
const revealedToken = ref('')
const regenerating = ref(false)
const procBusy = ref(false)
const downloadDir = ref('')
const saving = ref(false)
const saveError = ref('')

const tokenDisplay = computed(() => {
  if (tokenRevealed.value && revealedToken.value) return revealedToken.value
  return store.tokenMasked || '••••••••••••'
})

watch(() => store.settings.downloadDir, (v) => {
  if (!downloadDir.value) downloadDir.value = v
}, { immediate: true })

async function toggleReveal() {
  if (tokenRevealed.value) {
    tokenRevealed.value = false
    return
  }
  try {
    revealedToken.value = await store.revealToken()
    tokenRevealed.value = true
  } catch (e) {
    ElMessage.error(String(e))
  }
}

async function onRegenerate() {
  regenerating.value = true
  try {
    await store.regenerateToken()
    tokenRevealed.value = false
    revealedToken.value = ''
  } catch (e) {
    ElMessage.error(String(e))
  } finally {
    regenerating.value = false
  }
}

async function onPortChange(e) {
  const port = parseInt(e.target.value, 10)
  if (!port || port < 1 || port > 65535 || port === store.settings.rpcPort) {
    e.target.value = store.settings.rpcPort
    return
  }
  try {
    await store.saveSettings({ rpcPort: port })
  } catch (err) {
    ElMessage.error(`保存失败：${err}`)
    e.target.value = store.settings.rpcPort
  }
}

async function onStart() {
  procBusy.value = true
  try {
    await store.startAria2()
    await store.connect()
  } catch (e) {
    ElMessage.error(`启动失败：${e}`)
  } finally {
    procBusy.value = false
  }
}

async function onStop() {
  procBusy.value = true
  try {
    await store.stopAria2()
  } catch (e) {
    ElMessage.error(`停止失败：${e}`)
  } finally {
    procBusy.value = false
  }
}

async function onBrowse() {
  try {
    const dir = await store.browseDownloadDir()
    if (dir) downloadDir.value = dir
  } catch (e) {
    const msg = String(e)
    if (!/取消|cancel/i.test(msg)) saveError.value = msg
  }
}

async function onSave() {
  saving.value = true
  saveError.value = ''
  try {
    await store.saveSettings({ downloadDir: downloadDir.value.trim() })
    ElMessage.success('已保存')
  } catch (e) {
    saveError.value = String(e)
  } finally {
    saving.value = false
  }
}
</script>
