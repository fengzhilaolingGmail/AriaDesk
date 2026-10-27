<template>
  <div class="task-list">
    <template v-if="tasks.length">
      <div v-for="(task, i) in pagedTasks" :key="task.gid" class="task-row" :style="{ animationDelay: `${Math.min(i, 10) * 30}ms` }">
        <div class="task-type" :class="typeClass(task)">
          <!-- 进行中 -->
          <svg v-if="task.status === 'active'" width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M8 3v7M8 10L5.2 7.4M8 10l2.8-2.6M3.5 13h9" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          <!-- 已完成 -->
          <svg v-else-if="task.status === 'complete'" width="16" height="16" viewBox="0 0 16 16" fill="none">
            <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.4"/>
            <path d="M5 8.2l2 2 3.5-4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          <!-- 出错 / 已移除 -->
          <svg v-else-if="task.status === 'error' || task.status === 'removed'" width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M3.5 3.5h9v9h-9z" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/>
            <path d="M6.5 6.5l3 3M9.5 6.5l-3 3" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
          </svg>
          <!-- 暂停 / 等待 -->
          <svg v-else width="16" height="16" viewBox="0 0 16 16" fill="none">
            <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="1.4"/>
            <path d="M6 5.5v5M10 5.5v5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>
          </svg>
        </div>

        <div class="task-main">
          <div class="task-name" :title="taskName(task)">{{ taskName(task) }}</div>
          <div class="task-meta">
            <template v-if="task.status === 'active'">
              <template v-if="isSeeding(task)">
                <span>做种中</span>
                <span class="mono">↑ {{ formatSpeed(task.uploadSpeed) }}</span>
                <span>已上传 {{ formatSize(task.uploadLength) }}</span>
              </template>
              <template v-else>
                <span class="mono">{{ formatSpeed(task.downloadSpeed) }}</span>
                <span>{{ formatSize(task.completedLength) }} / {{ formatSize(task.totalLength) }}</span>
                <span>{{ toNum(task.connections) }} 连接</span>
              </template>
            </template>
            <template v-else-if="task.status === 'paused'">
              <span>已暂停</span>
              <span class="mono">{{ formatSize(task.completedLength) }} / {{ formatSize(task.totalLength) }}</span>
            </template>
            <template v-else-if="task.status === 'waiting'">
              <span>等待中</span>
              <span class="mono">{{ formatSize(task.totalLength) }}</span>
            </template>
            <template v-else-if="task.status === 'complete'">
              <span>已完成</span>
              <span class="mono">{{ formatSize(task.totalLength) }}</span>
              <span v-if="task.completedAt">{{ formatTime(task.completedAt) }}</span>
            </template>
            <template v-else>
              <span>{{ task.status === 'error' ? (task.errorMessage || '出错') : '已停止' }}</span>
              <span class="mono">{{ formatSize(task.completedLength) }} / {{ formatSize(task.totalLength) }}</span>
              <span v-if="task.completedAt">{{ formatTime(task.completedAt) }}</span>
            </template>
          </div>
        </div>

        <div class="progress">
          <div class="fill" :class="fillClass(task)" :style="{ width: pct(task) + '%' }"></div>
        </div>
        <div class="task-pct">{{ pct(task) }}%</div>

        <div class="task-ops">
          <button v-if="task.status === 'active' || task.status === 'waiting'" class="op-btn" title="暂停" @click="$emit('pause', task)">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M5 3v8M9 3v8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
          </button>
          <button v-if="task.status === 'paused'" class="op-btn" title="恢复" @click="$emit('resume', task)">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M5 3.5l5.5 3.5L5 10.5V3.5z" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>
          </button>
          <button v-if="task.status === 'complete'" class="op-btn" title="打开目录" @click="$emit('openDir', task)">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M2 4.5A1.5 1.5 0 013.5 3h2L7 4.5h3.5A1.5 1.5 0 0112 6v4a1.5 1.5 0 01-1.5 1.5h-7A1.5 1.5 0 012 10V4.5z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/></svg>
          </button>
          <button class="op-btn danger" title="删除" @click="$emit('remove', task)">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M3 4h8M6 4V2.8h2V4M4 4l.5 7h5L10 4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>
          </button>
        </div>
      </div>
    </template>

    <div v-else class="task-empty">
      <svg width="32" height="32" viewBox="0 0 20 20" fill="none">
        <path d="M10 3.5v9M10 12.5L6.3 9.2M10 12.5l3.7-3.3M4.5 16h11" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
      <div class="empty-text">此列表中没有下载任务</div>
    </div>

    <div v-if="tasks.length" class="task-pager">
      <span class="pager-total">共 {{ tasks.length }} 条</span>
      <button class="pager-btn" :disabled="page <= 1" title="上一页" @click="page--">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M8.5 3.5L5 7l3.5 3.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </button>
      <template v-for="(p, i) in pageNumbers" :key="i">
        <span v-if="p === '…'" class="pager-ellipsis">…</span>
        <button v-else class="pager-btn" :class="{ current: p === page }" @click="page = p">{{ p }}</button>
      </template>
      <button class="pager-btn" :disabled="page >= pageCount" title="下一页" @click="page++">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none"><path d="M5.5 3.5L9 7l-3.5 3.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>
      </button>
      <select class="pager-select" v-model.number="pageSize">
        <option v-for="n in [5, 10, 30, 50, 100]" :key="n" :value="n">{{ n }} 条/页</option>
      </select>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { formatPercent, formatSize, formatSpeed, formatTime, taskName, toNum } from '../utils/format'

const props = defineProps({
  tasks: { type: Array, default: () => [] },
  // 列表标识（当前 Tab），变化时回到第一页
  listKey: { type: String, default: '' },
})
defineEmits(['pause', 'resume', 'remove', 'openDir'])

const pageSize = ref(5)
const page = ref(1)
const pageCount = computed(() => Math.max(1, Math.ceil(props.tasks.length / pageSize.value)))
const pagedTasks = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return props.tasks.slice(start, start + pageSize.value)
})
// 页码窗口：≤7 页全显示，否则当前页前后各 1 页 + 首尾 + 省略号
const pageNumbers = computed(() => {
  const n = pageCount.value
  const cur = page.value
  if (n <= 7) return Array.from({ length: n }, (_, i) => i + 1)
  const pages = [1]
  const lo = Math.max(2, cur - 1)
  const hi = Math.min(n - 1, cur + 1)
  if (lo > 2) pages.push('…')
  for (let i = lo; i <= hi; i++) pages.push(i)
  if (hi < n - 1) pages.push('…')
  pages.push(n)
  return pages
})
watch([pageSize, () => props.listKey], () => { page.value = 1 })
watch(pageCount, (n) => { if (page.value > n) page.value = n })

function pct(task) {
  return formatPercent(task.completedLength, task.totalLength)
}

// BT 任务下载完后 aria2 默认继续做种（--seed-ratio=1.0），任务保持 active 状态
function isSeeding(task) {
  const total = toNum(task.totalLength)
  return (
    task.status === 'active' &&
    !!task?.bittorrent?.info?.name &&
    total > 0 &&
    toNum(task.completedLength) >= total
  )
}

function typeClass(task) {
  if (task.status === 'complete') return 'done'
  if (task.status === 'paused' || task.status === 'waiting') return 'paused'
  if (task.status === 'error' || task.status === 'removed') return 'error'
  return ''
}

function fillClass(task) {
  if (task.status === 'complete') return 'done'
  if (task.status === 'paused') return 'paused'
  return ''
}
</script>
