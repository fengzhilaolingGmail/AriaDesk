import { createRouter, createWebHashHistory } from 'vue-router'
import HomeView from './views/HomeView.vue'
import ManageView from './views/ManageView.vue'
import ConfView from './views/ConfView.vue'
import SettingsView from './views/SettingsView.vue'

export default createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'home', component: HomeView },
    { path: '/manage', name: 'manage', component: ManageView },
    { path: '/conf', name: 'conf', component: ConfView },
    { path: '/settings', name: 'settings', component: SettingsView },
  ],
})
