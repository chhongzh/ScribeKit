import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, describeError } from '../api/http'
import type { Settings } from '../api/types'
import { defaultThemePair, themeFromMtb, type ThemePair } from '../core/theme'
import { vuetify } from '../plugins/vuetify'

const MODE_KEY = 'scribekit.themeMode'

// 启动时记下 Vuetify 内置默认主题，恢复默认时用它，而不是在代码里写颜色。
const builtin = {
  light: { ...vuetify.theme.themes.value.light!.colors } as Record<string, string>,
  dark: { ...vuetify.theme.themes.value.dark!.colors } as Record<string, string>,
}
const builtinVars = {
  light: { ...vuetify.theme.themes.value.light!.variables },
  dark: { ...vuetify.theme.themes.value.dark!.variables },
}

function applyPair(pair: ThemePair) {
  const themes = vuetify.theme.themes.value
  const light = themes.light!
  const dark = themes.dark!
  light.colors = { ...pair.light.colors } as typeof light.colors
  dark.colors = { ...pair.dark.colors } as typeof dark.colors
  light.variables = { ...builtinVars.light, ...pair.light.variables }
  dark.variables = { ...builtinVars.dark, ...pair.dark.variables }
}

applyPair(defaultThemePair(builtin))

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<Settings | null>(null)
  const loading = ref(false)
  const error = ref('')
  const mode = ref<'light' | 'dark'>((localStorage.getItem(MODE_KEY) as 'light' | 'dark' | null) ?? 'light')
  const themeError = ref('')

  vuetify.theme.global.name.value = mode.value

  const modelReady = computed(
    () => !!settings.value && settings.value.apiKeySet && !!settings.value.scaffolderModel && !!settings.value.analyzerModel,
  )
  const customTheme = computed(() => !!settings.value?.theme)

  function applyTheme(raw: unknown) {
    themeError.value = ''
    if (!raw) {
      applyPair(defaultThemePair(builtin))
      return
    }
    const r = themeFromMtb(raw)
    if (r.ok) applyPair(r.pair)
    else {
      themeError.value = `已保存的主题无法使用：${r.error}。当前显示默认主题。`
      applyPair(defaultThemePair(builtin))
    }
  }

  async function load() {
    loading.value = true
    error.value = ''
    try {
      settings.value = await api.getSettings()
      applyTheme(settings.value.theme)
    } catch (e) {
      error.value = describeError(e)
    } finally {
      loading.value = false
    }
  }

  /** 保存。apiKey 为空字符串表示不修改（SPEC §4）。 */
  async function save(patch: Partial<Settings>) {
    if (!settings.value) throw new Error('设置尚未加载')
    const next: Settings = { ...settings.value, apiKey: '', ...patch }
    const saved = await api.putSettings(next)
    settings.value = saved
    applyTheme(saved.theme)
    return saved
  }

  function setMode(m: 'light' | 'dark') {
    mode.value = m
    vuetify.theme.global.name.value = m
    localStorage.setItem(MODE_KEY, m)
  }

  return { settings, loading, error, mode, themeError, modelReady, customTheme, load, save, setMode, applyTheme }
})
