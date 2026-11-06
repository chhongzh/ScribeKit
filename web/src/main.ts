import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import { vuetify } from './plugins/vuetify'
import './styles/base.css'

createApp(App).use(createPinia()).use(vuetify).mount('#app')
