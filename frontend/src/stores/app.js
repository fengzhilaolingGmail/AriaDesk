import { defineStore } from 'pinia'
import { ElMessage } from 'element-plus/es/components/message/index'
import * as App from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

const defaultSettings = () => ({
  aria2Path: '',
  rpcPort: 6800,
  downloadDir: '',
  autoStartAria2: false,
  autoStartApp: false,
  closeToTray: true,
})

export const useAppStore = defineStore('app', {
  state: () => ({
    loading: true,
    settings: defaultSettings(),
    detect: { installed: false, version: '', path: '' },
    aria2State: { running: false, pid: 0, port: 0, connected: false },
    stat: null,
    tasks: { active: [], waiting: [], stopped: [] },
    tokenMasked: '',
    connectError: '',
    connecting: false,
    install: { running: false, phase: '', lines: [], error: '' },
    about: null,
    eventsRegistered: false,
  }),
  getters: {
    connected: (s) => !!s.aria2State.connected,
    running: (s) => !!s.aria2State.running,
  },
  actions: {
    async init() {
      try {
        const [settings, detect, state, token] = await Promise.all([
          App.GetSettings().catch(() => null),
          App.DetectAria2().catch(() => null),
          App.GetAria2State().catch(() => null),
          App.GetTokenMasked().catch(() => ''),
        ])
        if (settings) this.settings = settings
        if (detect) this.detect = detect
        if (state) this.aria2State = state
        this.tokenMasked = token || ''
      } finally {
        this.loading = false
      }
    },

    registerEvents() {
      if (this.eventsRegistered) return
      this.eventsRegistered = true

      EventsOn('poll', (data) => {
        if (!data) return
        this.stat = data.stat || null
        this.tasks.active = data.active || []
        this.tasks.waiting = data.waiting || []
        this.tasks.stopped = data.stopped || []
        if (data.connected) {
          this.aria2State.connected = true
          this.connectError = ''
        }
      })
      EventsOn('aria2:state', (data) => {
        if (!data) return
        this.aria2State.running = !!data.running
        this.aria2State.connected = !!data.connected
        if (!data.connected) this.stat = null
      })
      EventsOn('aria2:install', (data) => {
        if (!data) return
        this.install.phase = data.phase || ''
        if (data.line) this.install.lines.push(data.line)
        if (data.phase === 'error') this.install.error = data.error || '安装失败'
      })
      EventsOn('task:completed', (data) => {
        if (data?.name) ElMessage.success(`下载完成：${data.name}`)
      })
    },

    async connect() {
      if (this.connecting) return
      this.connecting = true
      this.connectError = ''
      try {
        await App.Connect()
        this.aria2State = await App.GetAria2State()
      } catch (e) {
        this.connectError = String(e)
        this.aria2State.connected = false
      } finally {
        this.connecting = false
      }
    },

    async saveSettings(patch) {
      this.settings = { ...this.settings, ...patch }
      await App.SaveSettings({ ...this.settings })
    },

    async refreshDetect() {
      try {
        this.detect = await App.DetectAria2()
      } catch { /* keep old state */ }
    },

    async refreshState() {
      try {
        this.aria2State = await App.GetAria2State()
      } catch { /* keep old state */ }
    },

    async installAria2() {
      if (this.install.running) return
      this.install = { running: true, phase: 'running', lines: [], error: '' }
      try {
        await App.InstallAria2()
        await this.refreshDetect()
      } catch (e) {
        this.install.error = this.install.error || String(e)
        this.install.phase = 'error'
      } finally {
        this.install.running = false
      }
    },

    async startAria2() {
      await App.StartAria2()
      await this.refreshState()
    },

    async stopAria2() {
      await App.StopAria2()
      await this.refreshState()
    },

    async refreshToken() {
      this.tokenMasked = await App.GetTokenMasked().catch(() => '')
    },

    async revealToken() {
      return await App.RevealToken()
    },

    async regenerateToken() {
      await App.RegenerateToken()
      await this.refreshToken()
      ElMessage.success('已重新生成，需重启 aria2 生效')
    },

    async fetchTasks(tab) {
      if (!this.connected) return
      try {
        this.tasks[tab] = await App.GetTasks(tab) || []
      } catch { /* poll 事件会兜底刷新 */ }
    },

    async fetchAllTasks() {
      if (!this.connected) return
      await Promise.all(['active', 'waiting', 'stopped'].map((t) => this.fetchTasks(t)))
    },

    async addURIs(uris) {
      for (const uri of uris) {
        await App.AddURI(uri)
      }
    },

    async addTorrent() {
      await App.AddTorrentFile()
    },

    async pauseTask(gid) { await App.PauseTask(gid) },
    async unpauseTask(gid) { await App.UnpauseTask(gid) },
    async removeTask(gid, stopped) { await App.RemoveTask(gid, stopped) },
    async pauseAll() { await App.PauseAll() },
    async unpauseAll() { await App.UnpauseAll() },
    async purgeCompleted() { await App.PurgeCompleted() },
    async openTaskDir(gid) { await App.OpenTaskDir(gid) },

    async browseDownloadDir() {
      return await App.BrowseDownloadDir()
    },

    async loadAbout() {
      if (this.about) return this.about
      this.about = await App.GetAbout()
      return this.about
    },

    async readLog() {
      return await App.ReadAria2Log()
    },

    async openDataDir() { await App.OpenDataDir() },
    async openDownloadDir() { await App.OpenDownloadDir() },
  },
})
