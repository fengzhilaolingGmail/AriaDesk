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

        <div class="status-row">
          <div class="status-icon warning">
            <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
              <path d="M6.2 2.6h5.6l3.4 3.4v5.6l-3.4 3.4H6.2L2.8 11.6V6l3.4-3.4z" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/>
              <path d="M9 6v3.4M9 11.6v.2" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>
            </svg>
          </div>
          <div class="status-main">
            <div class="status-label">版本管理</div>
            <div class="status-desc">{{ versionSummary }}</div>
          </div>
          <button class="btn btn-default" :disabled="store.versionCheck.checking || updateBusy" @click="onCheck">
            <span v-if="store.versionCheck.checking" class="spinner"></span>检查更新
          </button>
          <button class="btn btn-danger" :disabled="updateBusy || !store.detect.installed" @click="onUninstall">卸载</button>
        </div>

        <div v-if="store.versionCheck.error" class="form-error">检查更新失败：{{ store.versionCheck.error }}</div>

        <div v-if="updateBusy || store.update.phase === 'done'" class="progress-block">
          <div class="progress-info">
            <span v-if="updateBusy" class="spinner"></span>
            <svg v-else width="14" height="14" viewBox="0 0 14 14" fill="none" class="progress-check">
              <circle cx="7" cy="7" r="5.5" stroke="currentColor" stroke-width="1.3"/>
              <path d="M4.6 7.2l1.6 1.6 3.2-3.6" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
            <span class="progress-text">{{ updateBusy ? lastUpdateLine : updateDoneText }}</span>
            <span v-if="updateBusy && store.update.percent != null" class="progress-pct mono">{{ store.update.percent }}%</span>
          </div>
          <div class="progress-track">
            <div
              class="progress-fill"
              :class="{ indeterminate: updateBusy && store.update.percent == null, done: !updateBusy }"
              :style="progressStyle"
            ></div>
          </div>
        </div>

        <div v-if="updateLog" ref="updateLogRef" class="install-log">{{ updateLog }}</div>
        <div v-if="store.update.phase === 'error'" class="install-error">{{ store.update.error }}</div>

        <div v-if="store.versionCheck.checked && releases.length" class="version-list">
          <div v-for="r in releases" :key="r.version" class="version-item">
            <span class="mono version-name">{{ r.version }}</span>
            <span class="version-tag" :class="r.isCurrent ? 'current' : (r.prerelease ? 'beta' : 'stable')">
              {{ r.isCurrent ? '当前版本' : (r.prerelease ? '测试版' : '正式版') }}
            </span>
            <span class="version-date">{{ r.date }}</span>
            <button v-if="!r.isCurrent" class="btn btn-default btn-sm" :disabled="updateBusy" @click="onSwitch(r)">
              <span v-if="updateBusy && store.update.target === r.version" class="spinner"></span>
              {{ r.isNewer ? '更新' : '回退' }}
            </button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { ElMessageBox } from 'element-plus/es/components/message-box/index'
import { useAppStore } from '../stores/app'
import InstallGuide from '../components/InstallGuide.vue'

const store = useAppStore()

const procBusy = ref(false)
const updateLogRef = ref(null)

const updateBusy = computed(() => store.update.phase === 'running')
const updateLog = computed(() => store.update.lines.slice(-30).join('\n'))

const lastUpdateLine = computed(() => {
  const lines = store.update.lines
  for (let i = lines.length - 1; i >= 0; i--) {
    if (lines[i]) return lines[i]
  }
  return '正在处理…'
})
const updateDoneText = computed(() =>
  store.update.target ? `已切换到 aria2 ${store.update.target}` : 'aria2 已卸载',
)
const progressStyle = computed(() => {
  if (!updateBusy.value) return { width: '100%' }
  if (store.update.percent != null) return { width: store.update.percent + '%' }
  return {}
})

watch(updateLog, async () => {
  await nextTick()
  if (updateLogRef.value) updateLogRef.value.scrollTop = updateLogRef.value.scrollHeight
})

// 与后端 CompareVersions 保持一致的点分版本号比较
function compareVersions(a, b) {
  const pa = String(a || '').split('.')
  const pb = String(b || '').split('.')
  const n = Math.max(pa.length, pb.length)
  for (let i = 0; i < n; i++) {
    const va = parseInt(pa[i], 10) || 0
    const vb = parseInt(pb[i], 10) || 0
    if (va !== vb) return va > vb ? 1 : -1
  }
  return 0
}

const releases = computed(() => {
  const cur = store.detect.version
  return store.versionCheck.releases.map((r) => {
    const cmp = cur ? compareVersions(r.version, cur) : 1
    return { ...r, isCurrent: cmp === 0, isNewer: cmp > 0, date: (r.publishedAt || '').slice(0, 10) }
  })
})

const versionSummary = computed(() => {
  const vc = store.versionCheck
  if (vc.checking) return '正在检查更新…'
  if (!vc.checked) return '从 GitHub 检查新版本，可更新、回退或卸载'
  const cur = store.detect.version
  const stable = vc.releases.find((r) => !r.prerelease)
  const beta = vc.releases.find((r) => r.prerelease)
  const parts = []
  if (cur && stable && compareVersions(stable.version, cur) > 0) {
    parts.push(`发现新版本 ${stable.version}`)
  } else if (stable) {
    parts.push(`最新正式版 ${stable.version}`)
  }
  if (beta) parts.push(`最新测试版 ${beta.version}`)
  return parts.join(' · ') || '未获取到版本信息'
})

async function onCheck() {
  await store.checkAria2Updates()
}

async function onSwitch(r) {
  const action = r.isNewer ? '更新' : '回退'
  try {
    await ElMessageBox.confirm(
      `确定${action}到 aria2 ${r.version}${r.prerelease ? '（测试版）' : ''}吗？正在运行的 aria2 将会重启。`,
      `${action}确认`,
      { confirmButtonText: action, cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  await store.updateAria2To(r.version)
}

async function onUninstall() {
  try {
    await ElMessageBox.confirm(
      '确定卸载 aria2 吗？将停止下载内核，并删除 AriaDesk 下载的版本文件。',
      '卸载确认',
      { confirmButtonText: '卸载', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }
  await store.uninstallAria2()
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
</script>
