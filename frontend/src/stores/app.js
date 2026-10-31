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
  historyMax: 1000,
  maxConcurrent: 0,
  browserExtEnabled: true,
  autoShowOnPush: true,
})

export const useAppStore = defineStore('app', {
  state: () => ({
    loading: true,
    settings: defaultSettings(),
    detect: { installed: false, version: '', path: '' },
    aria2State: { running: false, pid: 0, port: 0, connected: false },
    stat: null,
    tasks: { active: [], waiting: [], stopped: [] },
    history: [],
    tokenMasked: '',
    connectError: '',
    connecting: false,
    install: { running: false, phase: '', lines: [], error: '', percent: null },
    versionCheck: { checking: false, checked: false, releases: [], error: '' },
    update: { target: '', phase: '', lines: [], error: '', percent: null },
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
        const [settings, detect, state, token, history] = await Promise.all([
          App.GetSettings().catch(() => null),
          App.DetectAria2().catch(() => null),
          App.GetAria2State().catch(() => null),
          App.GetTokenMasked().catch(() => ''),
          App.GetHistory().catch(() => []),
        ])
        if (settings) this.settings = settings
        if (detect) this.detect = detect
        if (state) this.aria2State = state
        this.tokenMasked = token || ''
        this.history = history || []
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
        if (data.phase === 'done') {
          // 安装是异步的，完成后必须重新检测，否则界面停留在安装引导页
          this.refreshDetect()
          this.refreshState()
        }
      })
      EventsOn('aria2:update', (data) => {
        if (!data) return
        this.update.phase = data.phase || ''
        if (data.line) this.update.lines.push(data.line)
        if (typeof data.percent === 'number' && data.percent >= 0) {
          this.update.percent = data.percent
        }
        if (data.phase === 'error') this.update.error = data.error || '操作失败'
        if (data.phase === 'done') {
          this.update.percent = 100
          ElMessage.success(this.update.target ? `已切换到 aria2 ${this.update.target}` : 'aria2 已卸载')
          this.refreshDetect()
          this.refreshState()
        }
      })
      EventsOn('task:completed', (data) => {
        if (data?.name) ElMessage.success(`下载完成：${data.name}`)
      })
      EventsOn('history', (data) => {
        this.history = data || []
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
      this.install = { running: true, phase: 'running', lines: [], error: '', percent: null }
      // 重新安装后，此前卸载/更新的结果状态不再有效，避免残留显示
      this.update = { target: '', phase: '', lines: [], error: '', percent: null }
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

    async checkAria2Updates() {
      if (this.versionCheck.checking) return
      this.versionCheck.checking = true
      this.versionCheck.error = ''
      try {
        this.versionCheck.releases = await App.CheckAria2Updates() || []
        this.versionCheck.checked = true
      } catch (e) {
        this.versionCheck.error = String(e)
      } finally {
        this.versionCheck.checking = false
      }
    },

    async updateAria2To(version) {
      if (this.update.phase === 'running') return
      this.update = { target: version, phase: 'running', lines: [], error: '', percent: null }
      try {
        await App.UpdateAria2ToVersion(version)
      } catch (e) {
        this.update.error = this.update.error || String(e)
        this.update.phase = 'error'
      }
    },

    async uninstallAria2() {
      if (this.update.phase === 'running') return
      this.update = { target: '', phase: 'running', lines: [], error: '', percent: null }
      // 卸载后此前的“安装成功”状态不再有效，重置避免安装引导页误显示已安装
      this.install = { running: false, phase: '', lines: [], error: '', percent: null }
      try {
        await App.UninstallAria2()
      } catch (e) {
        this.update.error = this.update.error || String(e)
        this.update.phase = 'error'
      }
    },

    async startAria2() {
      await App.StartAria2()
      // 启动成功说明 aria2 可用，同步刷新检测结果，让管理页从安装引导切换到正常界面
      await Promise.all([this.refreshDetect(), this.refreshState()])
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
    },

    async getExtInfo() {
      return await App.GetExtInfo()
    },

    async revealExtToken() {
      return await App.RevealExtToken()
    },

    async regenerateExtToken() {
      await App.RegenerateExtToken()
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
    async openLocation(path) { await App.OpenLocation(path) },
    async deleteHistoryRecord(gid) { await App.DeleteHistoryRecord(gid) },

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

    async getAria2ConfSections() {
      return await App.GetAria2ConfSections()
    },

    async saveAria2ConfValues(values) {
      await App.SaveAria2ConfValues(values)
    },

    async resetAria2Conf() {
      await App.ResetAria2Conf()
    },

    async updateBtTrackers() {
      return await App.UpdateBtTrackers()
    },

    async openDataDir() { await App.OpenDataDir() },
    async openDownloadDir() { await App.OpenDownloadDir() },
  },
})
