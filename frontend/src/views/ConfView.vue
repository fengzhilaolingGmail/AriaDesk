<template>
  <div class="container container-narrow">
    <div v-if="loading" class="form-card">加载中…</div>
    <div v-else-if="pageError" class="form-card">
      <div class="form-error">加载配置失败：{{ pageError }}</div>
    </div>

    <template v-else>
      <div v-for="sec in sections" :key="sec.name" class="form-card">
        <div class="section-title">{{ sec.name }}</div>

        <div v-for="item in sec.items" :key="item.key" class="form-row">
          <div class="form-label">
            <span v-if="LABELS[item.key]">{{ LABELS[item.key] }}</span>
            <span v-else class="mono">{{ item.key }}</span>
          </div>

          <!-- RPC 密钥 -->
          <div v-if="item.kind === 'secret'" class="input-wrap">
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

          <!-- 下载目录 -->
          <div v-else-if="item.kind === 'dir'" class="input-wrap">
            <input v-model="item.edited" class="input mono" placeholder="C:\Users\...\Downloads" />
            <button class="btn btn-default" @click="onBrowse(item)">浏览…</button>
          </div>

          <!-- 同时下载数量 -->
          <div v-else-if="item.kind === 'concurrent'" class="input-wrap">
            <select class="pager-select" v-model="item.edited">
              <option v-for="n in cpuOptions" :key="n" :value="String(n)">{{ n }}</option>
            </select>
          </div>

          <!-- bt-tracker：文本 + 手动刷新 -->
          <div v-else-if="item.kind === 'trackers'" class="input-wrap">
            <input v-model="item.edited" class="input mono" />
            <button class="btn btn-default" :disabled="refreshingTrackers" @click="onRefreshTrackers(item)">
              <span v-if="refreshingTrackers" class="spinner"></span>刷新
            </button>
          </div>

          <!-- 开关 -->
          <div v-else-if="item.kind === 'bool'" class="input-wrap">
            <select class="pager-select" v-model="item.edited">
              <option value="true">开启</option>
              <option value="false">关闭</option>
            </select>
          </div>

          <!-- 端口与其余文本 -->
          <div v-else class="input-wrap">
            <input v-model="item.edited" class="input mono" :class="{ 'w-fixed': item.kind === 'port' }" />
          </div>

          <div v-if="tipOf(item.key) || item.comment" class="form-tip">{{ tipOf(item.key) || item.comment }}</div>
        </div>

        <div v-if="sec.error" class="form-error">{{ sec.error }}</div>
        <div class="form-actions">
          <button class="btn btn-primary" :disabled="sec.saving || !isDirty(sec)" @click="onSaveSection(sec)">
            <span v-if="sec.saving" class="spinner"></span>保存修改
          </button>
        </div>
      </div>

      <!-- 操作 -->
      <div class="form-card">
        <div class="section-title">操作</div>
        <div class="form-tip" style="margin-top: 0; margin-bottom: 12px">
          配置文件：%APPDATA%\AriaDesk\aria2\aria2.conf，保存后需重启 aria2 生效（标注“立即生效”的除外）。
        </div>
        <button class="btn btn-danger" :disabled="resetting" @click="onReset">
          <span v-if="resetting" class="spinner"></span>重置为默认配置
        </button>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { useAppStore } from '../stores/app'

const store = useAppStore()

// 由应用统一管理且不在配置页展示的键
const HIDDEN_KEYS = ['input-file', 'save-session', 'log', 'log-level', 'enable-rpc', 'rpc-allow-origin-all', 'rpc-listen-all']
const KIND_MAP = {
  dir: 'dir',
  'rpc-listen-port': 'port',
  'rpc-secret': 'secret',
  'max-concurrent-downloads': 'concurrent',
  'bt-tracker': 'trackers',
}
const LABELS = {
  dir: '默认下载目录',
  'rpc-listen-port': 'RPC 端口',
  'rpc-secret': 'RPC 密钥 (Token)',
  'max-concurrent-downloads': '同时下载数量',
}
// 托管键的操作提示保持与原管理页一致
const TIPS = {
  dir: '保存时校验目录存在且可写，立即生效，仅影响之后新建的任务',
  'rpc-listen-port': '修改后需重启 aria2 生效',
  'rpc-secret': '保存在 %APPDATA%\\AriaDesk\\token.pass，已加密存储',
  'max-concurrent-downloads': '', // 挂载后按 CPU 核心数生成
}

const sections = ref([])
const loading = ref(true)
const pageError = ref('')
const resetting = ref(false)
const refreshingTrackers = ref(false)
const tokenRevealed = ref(false)
const revealedToken = ref('')
const regenerating = ref(false)

const cpuCores = computed(() => store.about?.cpuCores || 16)
const cpuOptions = computed(() => Array.from({ length: cpuCores.value }, (_, i) => i + 1))
const concurrentTip = computed(() =>
  `限制 aria2 同时下载的任务数（最大为 CPU 核心数 ${cpuCores.value}，默认为一半），立即生效`,
)

const tokenDisplay = computed(() => {
  if (tokenRevealed.value && revealedToken.value) return revealedToken.value
  return store.tokenMasked || '••••••••••••'
})

onMounted(async () => {
  try {
    await store.loadAbout()
  } catch { /* 忽略，使用默认核心数 */ }
  await loadSections()
})

function tipOf(key) {
  if (key === 'max-concurrent-downloads') return concurrentTip.value
  return TIPS[key] || ''
}

function toItem(item) {
  if (HIDDEN_KEYS.includes(item.key)) return null
  const kind = KIND_MAP[item.key] || (item.value === 'true' || item.value === 'false' ? 'bool' : 'text')
  const it = { ...item, kind, edited: item.value }
  if (kind === 'dir') it.edited = store.settings.downloadDir
  if (kind === 'port') it.edited = String(store.settings.rpcPort)
  if (kind === 'concurrent') it.edited = String(store.settings.maxConcurrent)
  return it
}

async function loadSections() {
  loading.value = true
  pageError.value = ''
  try {
    const raw = await store.getAria2ConfSections() || []
    sections.value = raw
      .map((sec) => ({
        name: sec.name,
        saving: false,
        error: '',
        items: (sec.items || []).map(toItem).filter(Boolean),
      }))
      .filter((sec) => sec.items.length > 0)
  } catch (e) {
    pageError.value = String(e)
  } finally {
    loading.value = false
  }
}

function isDirty(sec) {
  return sec.items.some((item) => {
    if (item.kind === 'secret') return false
    if (item.kind === 'dir') return item.edited.trim() !== store.settings.downloadDir
    if (item.kind === 'port') return item.edited !== String(store.settings.rpcPort)
    if (item.kind === 'concurrent') return item.edited !== String(store.settings.maxConcurrent)
    return item.edited !== item.value
  })
}

async function onSaveSection(sec) {
  sec.saving = true
  sec.error = ''
  try {
    const patch = {}
    const confValues = {}
    for (const item of sec.items) {
      switch (item.kind) {
        case 'secret':
          break
        case 'dir':
          if (item.edited.trim() !== store.settings.downloadDir) patch.downloadDir = item.edited.trim()
          break
        case 'port': {
          const port = parseInt(item.edited, 10)
          if (!port || port < 1 || port > 65535) throw new Error('RPC 端口无效（1-65535）')
          if (port !== store.settings.rpcPort) patch.rpcPort = port
          break
        }
        case 'concurrent':
          if (item.edited !== String(store.settings.maxConcurrent)) patch.maxConcurrent = Number(item.edited)
          break
        default:
          if (item.edited !== item.value) confValues[item.key] = item.edited
      }
    }
    if (patch.downloadDir !== undefined && !patch.downloadDir) throw new Error('下载目录不能为空')

    // 端口与配置文件项需重启 aria2 才生效；目录和同时下载数通过 RPC 实时生效
    const needsRestart = patch.rpcPort !== undefined || Object.keys(confValues).length > 0
    if (needsRestart && store.running) {
      try {
        await ElMessageBox.confirm(
          '本次修改包含需重启 aria2 才能生效的配置。确认后将保存配置并立即重启服务（下载任务会自动恢复）。',
          '保存并重启',
          { confirmButtonText: '保存并重启', cancelButtonText: '取消', type: 'warning' },
        )
      } catch {
        return
      }
    }

    if (Object.keys(patch).length > 0) await store.saveSettings(patch)
    if (Object.keys(confValues).length > 0) await store.saveAria2ConfValues(confValues)
    for (const item of sec.items) {
      if (!['secret', 'dir', 'port', 'concurrent'].includes(item.kind)) item.value = item.edited
    }

    if (needsRestart && store.running) {
      await store.stopAria2()
      await store.startAria2()
      await store.connect()
      ElMessage.success('已保存并重启 aria2')
    } else {
      ElMessage.success(needsRestart ? '已保存，启动 aria2 后生效' : '已保存')
    }
  } catch (e) {
    sec.error = String(e)
  } finally {
    sec.saving = false
  }
}

async function onBrowse(item) {
  try {
    const dir = await store.browseDownloadDir()
    if (dir) item.edited = dir
  } catch (e) {
    const msg = String(e)
    if (!/取消|cancel/i.test(msg)) ElMessage.error(msg)
  }
}

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
    if (store.running) {
      try {
        await ElMessageBox.confirm(
          '密钥已更新，需重启 aria2 才能生效。确认后将立即重启服务（下载任务会自动恢复）。',
          '重启确认',
          { confirmButtonText: '立即重启', cancelButtonText: '稍后重启', type: 'warning' },
        )
      } catch {
        ElMessage.success('已重新生成，需重启 aria2 生效')
        return
      }
      await store.stopAria2()
      await store.startAria2()
      await store.connect()
      ElMessage.success('已重新生成并重启 aria2')
    } else {
      ElMessage.success('已重新生成')
    }
  } catch (e) {
    ElMessage.error(String(e))
  } finally {
    regenerating.value = false
  }
}

async function onRefreshTrackers(item) {
  refreshingTrackers.value = true
  try {
    const list = await store.updateBtTrackers()
    item.value = list
    item.edited = list
    const n = list ? list.split(',').filter(Boolean).length : 0
    ElMessage.success(`已更新 ${n} 个 Tracker`)
  } catch (e) {
    ElMessage.error(`刷新 Tracker 失败：${e}`)
  } finally {
    refreshingTrackers.value = false
  }
}

async function onReset() {
  try {
    await ElMessageBox.confirm(
      '确定重置为默认配置吗？当前的自定义修改将全部丢失。',
      '重置确认',
      { confirmButtonText: '重置', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  resetting.value = true
  try {
    await store.resetAria2Conf()
    await loadSections()
    ElMessage.success('已重置为默认配置')
  } catch (e) {
    ElMessage.error(String(e))
  } finally {
    resetting.value = false
  }
}
</script>
