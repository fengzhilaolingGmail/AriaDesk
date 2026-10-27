import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElDialog from 'element-plus/es/components/dialog/index'
import 'element-plus/es/components/base/style/css'
import 'element-plus/es/components/overlay/style/css'
import 'element-plus/es/components/dialog/style/css'
import 'element-plus/es/components/message/style/css'
import App from './App.vue'
import router from './router'
import './styles/main.css'

createApp(App).use(createPinia()).use(router).use(ElDialog).mount('#app')
