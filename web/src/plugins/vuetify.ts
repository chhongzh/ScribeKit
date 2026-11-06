import 'vuetify/styles'
import { createVuetify } from 'vuetify'
import { md3 } from 'vuetify/blueprints'
import { aliases, mdi } from 'vuetify/iconsets/mdi-svg'
import { zhHans } from 'vuetify/locale'

export const vuetify = createVuetify({
  blueprint: md3,
  icons: { defaultSet: 'mdi', aliases, sets: { mdi } },
  locale: { locale: 'zhHans', messages: { zhHans } },
  theme: {
    defaultTheme: 'light',
    // 只开启 theme 变量，不注入额外色阶：所有颜色都来自默认主题或导入的 MTB 主题。
    variations: false,
  },
  defaults: {
    VBtn: { variant: 'text' },
    VTab: { variant: 'text', rounded: 0 },
    VTextField: { variant: 'outlined', density: 'compact', hideDetails: 'auto' },
    VTextarea: { variant: 'outlined', density: 'compact', hideDetails: 'auto' },
    VSelect: { variant: 'outlined', density: 'compact', hideDetails: 'auto' },
    VSwitch: { color: 'primary', density: 'compact', hideDetails: true, inset: false },
    VTooltip: { location: 'bottom', openDelay: 450 },
    VMenu: { transition: 'sk-menu' },
    VDialog: { transition: 'sk-dialog' },
    VSnackbar: { transition: 'sk-view' },
  },
})
