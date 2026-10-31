<template>
  <div class="container">
    <InstallGuide v-if="!store.loading && !store.detect.installed" />

    <template v-else-if="store.loading">
      <div class="stats">
        <div v-for="i in 5" :key="i" class="skeleton-block skeleton-stat"></div>
      </div>
      <div class="task-list">
        <div v-for="i in 5" :key="i" class="skeleton-row">
          <div class="sk-icon"></div>
          <div class="sk-lines">
            <div class="sk-line w60"></div>
            <div class="sk-line w35"></div>
          </div>
          <div class="sk-bar"></div>
        </div>
      </div>
    </template>

    <template v-else>
      <!-- 连接区 -->
      <div class="conn-bar">
        <div class="field">
          <label>RPC 地址</label>
          <div class="input readonly w-addr">localhost</div>
        </div>
        <div class="field">
          <label>端口</label>
          <input class="input mono w-port" :value="store.settings.rpcPort" @change="onPortChange" />
        </div>
        <div class="field grow">
          <label>RPC 密钥 (TOKEN)</label>
          <div class="input readonly w-token"><span class="mono">{{ store.tokenMasked || '••••••••••••' }}</span></div>
        </div>
        <button class="btn btn-primary" :disabled="store.connecting" @click="onConnect">
          <svg v-if="!store.connecting" width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path d="M2.5 7a4.5 4.5 0 014.5-4.5h4M11.5 7a4.5 4.5 0 01-4.5 4.5h-4" stroke="#fff" stroke-width="1.4" stroke-linecap="round"/>
            <path d="M9.8 1.2l1.9 1.4-1.9 1.4M4.2 12.8l-1.9-1.4 1.9-1.4" stroke="#fff" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          <span v-else class="spinner"></span>
          {{ store.connected ? '刷新' : '连接' }}
        </button>
        <div class="badge" :class="badgeClass">
          <span class="dot"></span>{{ badgeText }}
        </div>
      </div>

      <!-- 连接失败横幅 -->
      <div v-if="store.connectError" class="conn-error">
        <span class="msg">连接失败：{{ store.connectError }}</span>
        <button class="btn btn-default" @click="onConnect">重试</button>
      </div>

      <StatCards />

      <!-- 任务工具条 -->
      <div class="task-toolbar">
        <div class="tabs">
          <div
            v-for="t in tabs"
            :key="t.key"
            class="tab"
            :class="{ active: activeTab === t.key }"
            @click="switchTab(t.key)"
          >{{ t.label }}</div>
        </div>
        <div class="toolbar-actions" :title="store.connected ? '' : 'aria2 未连接，请先连接'">
          <button class="btn btn-default" :disabled="!store.connected" @click="onPauseAll">暂停全部</button>
          <button class="btn btn-default" :disabled="!store.connected" @click="onUnpauseAll">恢复全部</button>
          <button class="btn btn-default" :disabled="!store.connected" @click="onPurge">清理已完成</button>
          <button class="btn btn-primary" :disabled="!store.connected" @click="newTaskVisible = true">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
              <path d="M7 2.5v9M2.5 7h9" stroke="#fff" stroke-width="1.6" stroke-linecap="round"/>
            </svg>
            新建任务
          </button>
        </div>
      </div>

      <TaskList
        :tasks="currentTasks"
        :list-key="activeTab"
        @pause="(t) => run(() => store.pauseTask(t.gid))"
        @resume="(t) => run(() => store.unpauseTask(t.gid))"
        @remove="askRemove"
        @openDir="(t) => run(() => (t.history ? store.openLocation(t.filePath || t.dir) : store.openTaskDir(t.gid)))"
      />
    </template>

    <NewTaskDialog v-model="newTaskVisible" />

    <!-- 删除确认 -->
    <el-dialog v-model="removeVisible" title="删除任务" width="400px">
      <p style="font-size: 13px; color: var(--text-1);">删除该任务？</p>
      <p v-if="removeTarget" class="mono" style="font-size: 12px; color: var(--text-2); margin-top: 6px; word-break: break-all;">
        {{ taskName(removeTarget) }}
      </p>
      <label style="display: flex; align-items: center; gap: 6px; margin-top: 12px; font-size: 13px; color: var(--text-1); cursor: pointer;">
        <input type="checkbox" v-model="removeWithFiles" />
        同时删除已下载的文件
      </label>
      <template #footer>
        <button class="btn btn-default" @click="removeVisible = false">取消</button>
        <button class="btn btn-danger" @click="confirmRemove">删除</button>
      </template>
    </el-dialog>

    <!-- 清理确认 -->
    <el-dialog v-model="purgeVisible" title="清理已完成" width="400px">
      <p style="font-size: 13px; color: var(--text-1);">将清理 {{ purgeCount }} 条已完成 / 停止的下载记录（含历史记录）。</p>
      <p style="font-size: 12px; color: var(--text-2); margin-top: 6px;">仅清除列表记录，已下载的文件不会被删除。</p>
      <template #footer>
        <button class="btn btn-default" @click="purgeVisible = false">取消</button>
        <button class="btn btn-danger" @click="confirmPurge">清理</button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { useAppStore } from '../stores/app'
import { taskName, toNum } from '../utils/format'
import InstallGuide from '../components/InstallGuide.vue'
import StatCards from '../components/StatCards.vue'
import TaskList from '../components/TaskList.vue'
import NewTaskDialog from '../components/NewTaskDialog.vue'

const store = useAppStore()

const tabs = [
  { key: 'all', label: '全部' },
  { key: 'active', label: '进行中' },
  { key: 'waiting', label: '等待中' },
  { key: 'paused', label: '暂停' },
  { key: 'stopped', label: '停止' },
  { key: 'complete', label: '已完成' },
]
const activeTab = ref('all')
const newTaskVisible = ref(false)
const removeVisible = ref(false)
const removeTarget = ref(null)
const removeWithFiles = ref(false)
const purgeVisible = ref(false)

const currentTasks = computed(() => {
  const t = store.tasks
  const waiting = t.waiting || []
  const stopped = t.stopped || []
  // 本地历史与 aria2 内存中的 stopped 记录按 gid 去重，优先展示实时记录
  const liveGids = new Set(stopped.map((x) => x.gid))
  const hist = (store.history || [])
    .filter((h) => !liveGids.has(h.gid))
    .map((h) => ({ ...h, history: true }))
  switch (activeTab.value) {
    case 'active':
      return t.active || []
    case 'waiting':
      return waiting.filter((x) => x.status === 'waiting')
    case 'paused':
      return waiting.filter((x) => x.status === 'paused')
    case 'stopped':
      return [...stopped.filter((x) => x.status === 'error' || x.status === 'removed'), ...hist.filter((h) => h.status !== 'complete')]
    case 'complete':
      return [...stopped.filter((x) => x.status === 'complete'), ...hist.filter((h) => h.status === 'complete')]
    default:
      return [...(t.active || []), ...waiting, ...stopped, ...hist]
  }
})

const badgeClass = computed(() => {
  if (!store.detect.installed) return 'uninstalled'
  return store.connected ? 'connected' : 'disconnected'
})
const badgeText = computed(() => {
  if (!store.detect.installed) return '未安装 aria2'
  if (!store.connected) return '未连接'
  const n = store.stat ? toNum(store.stat.numActive) : 0
  return `已连接 (${n} 个进行中)`
})

function switchTab(key) {
  activeTab.value = key
  store.fetchAllTasks()
}

watch(() => store.connected, (v) => {
  if (v) store.fetchAllTasks()
}, { immediate: true })

async function onConnect() {
  await store.connect()
}

async function onPortChange(e) {
  const port = parseInt(e.target.value, 10)
  if (!port || port < 1 || port > 65535 || port === store.settings.rpcPort) {
    e.target.value = store.settings.rpcPort
    return
  }
  try {
    await store.saveSettings({ rpcPort: port })
    ElMessage.success('端口已保存，需重启 aria2 生效')
  } catch (err) {
    ElMessage.error(`保存失败：${err}`)
    e.target.value = store.settings.rpcPort
  }
}

async function run(fn) {
  try {
    await fn()
  } catch (e) {
    ElMessage.error(String(e))
  }
}

function onPauseAll() { run(() => store.pauseAll()) }
function onUnpauseAll() { run(() => store.unpauseAll()) }

const purgeCount = computed(() => {
  const stopped = store.tasks.stopped || []
  const liveGids = new Set(stopped.map((x) => x.gid))
  const histOnly = (store.history || []).filter((h) => !liveGids.has(h.gid)).length
  const live = store.stat ? toNum(store.stat.numStopped) : 0
  return live + histOnly
})

function onPurge() {
  if (purgeCount.value === 0) {
    ElMessage.info('没有可清理的记录')
    return
  }
  purgeVisible.value = true
}

async function confirmPurge() {
  const n = purgeCount.value
  purgeVisible.value = false
  await run(async () => {
    await store.purgeCompleted()
    ElMessage.success(`已清理 ${n} 条记录`)
  })
}

function askRemove(task) {
  removeTarget.value = task
  removeWithFiles.value = false
  removeVisible.value = true
}

async function confirmRemove() {
  const task = removeTarget.value
  const withFiles = removeWithFiles.value
  removeVisible.value = false
  if (!task) return
  if (task.history) {
    await run(() => store.deleteHistoryRecord(task.gid, withFiles))
    return
  }
  // 已完成/出错/已移除的记录走 removeDownloadResult，其余走 remove
  const stopped = ['complete', 'error', 'removed'].includes(task.status)
  await run(() => store.removeTask(task.gid, stopped, withFiles))
}
</script>
