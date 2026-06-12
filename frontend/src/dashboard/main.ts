import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory } from 'vue-router'
import naive from 'naive-ui'
import DashboardApp from './App.vue'
import { routes } from './router'
import '@/styles/global.css'

const app = createApp(DashboardApp)

app.use(createPinia())
app.use(naive)

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior() {
    // Reset scroll position in .dashboard-content on every navigation
    const el = document.querySelector('.dashboard-content')
    if (el) el.scrollTop = 0
    return undefined
  },
})
app.use(router)

app.mount('#app')
