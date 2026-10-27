<template>
  <el-dialog
    :model-value="modelValue"
    title="新建任务"
    width="480px"
    :close-on-click-modal="false"
    @update:model-value="$emit('update:modelValue', $event)"
    @closed="reset"
  >
    <div class="form-row">
      <div class="form-label">下载链接</div>
      <textarea
        v-model="urls"
        class="input mono"
        rows="4"
        placeholder="每行一个，支持 http / https / magnet"
      ></textarea>
      <div v-if="error" class="form-error">{{ error }}</div>
    </div>
    <div class="form-row">
      <button class="btn btn-default" :disabled="submitting" @click="onPickTorrent">选择种子文件</button>
      <div class="form-tip">支持 .torrent 种子文件，选中后立即创建任务</div>
    </div>
    <template #footer>
      <button class="btn btn-default" :disabled="submitting" @click="$emit('update:modelValue', false)">取消</button>
      <button class="btn btn-primary" :disabled="submitting" @click="onSubmit">
        <span v-if="submitting" class="spinner"></span>
        新建
      </button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import { useAppStore } from '../stores/app'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const store = useAppStore()
const urls = ref('')
const error = ref('')
const submitting = ref(false)

function reset() {
  urls.value = ''
  error.value = ''
  submitting.value = false
}

async function onSubmit() {
  const lines = urls.value.split('\n').map((s) => s.trim()).filter(Boolean)
  if (!lines.length) {
    error.value = '请输入至少一个下载链接'
    return
  }
  error.value = ''
  submitting.value = true
  try {
    await store.addURIs(lines)
    ElMessage.success('已创建任务')
    emit('update:modelValue', false)
  } catch (e) {
    error.value = String(e)
  } finally {
    submitting.value = false
  }
}

async function onPickTorrent() {
  try {
    await store.addTorrent()
    ElMessage.success('已创建任务')
    emit('update:modelValue', false)
  } catch (e) {
    const msg = String(e)
    if (!/取消|cancel/i.test(msg)) error.value = msg
  }
}
</script>
