<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { api, describeError } from '../../api/http'
import type { Settings } from '../../api/types'
import { descLabel } from '../../core/labels'
import { COLORED_DESC, themeFromMtb } from '../../core/theme'
import { useAnalysisStore } from '../../stores/analysis'
import { useNotifyStore } from '../../stores/notify'
import { useSettingsStore } from '../../stores/settings'
import StateBlock from '../StateBlock.vue'

const store = useSettingsStore()
const notify = useNotifyStore()
const analysis = useAnalysisStore()

const form = reactive({
  provider: 'claude' as Settings['provider'],
  baseUrl: '',
  apiKey: '',
  analyzerModel: '',
  scaffolderModel: '',
  autoAnalyze: true,
})
const saving = ref(false)
const testing = ref(false)
const testResult = ref<{ ok: boolean; message: string } | null>(null)
const themeFile = ref<HTMLInputElement | null>(null)
const themeMsg = ref('')
const themeBusy = ref(false)

watch(
  () => store.settings,
  (s) => {
    if (!s) return
    form.provider = s.provider
    form.baseUrl = s.baseUrl
    form.apiKey = ''
    form.analyzerModel = s.analyzerModel
    form.scaffolderModel = s.scaffolderModel
    form.autoAnalyze = s.autoAnalyze
  },
  { immediate: true },
)

const providers = [
  { value: 'claude', title: 'Claude（Anthropic API）' },
  { value: 'openai', title: 'OpenAI 协议（兼容服务均可）' },
]

const dirty = computed(() => {
  const s = store.settings
  if (!s) return false
  return (
    form.provider !== s.provider ||
    form.baseUrl !== s.baseUrl ||
    form.apiKey !== '' ||
    form.analyzerModel !== s.analyzerModel ||
    form.scaffolderModel !== s.scaffolderModel ||
    form.autoAnalyze !== s.autoAnalyze
  )
})

async function save() {
  saving.value = true
  try {
    await store.save({ ...form })
    form.apiKey = ''
    testResult.value = null
    analysis.resume()
    notify.push('设置已保存。')
  } catch (e) {
    notify.error(`设置没有保存：${describeError(e)}`)
  } finally {
    saving.value = false
  }
}

async function test() {
  testing.value = true
  testResult.value = null
  try {
    if (dirty.value) await save()
    testResult.value = await api.testSettings()
  } catch (e) {
    testResult.value = { ok: false, message: describeError(e) }
  } finally {
    testing.value = false
  }
}

function pickTheme() {
  themeFile.value?.click()
}

async function onThemeFile(e: Event) {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) return
  themeMsg.value = ''
  let json: unknown
  try {
    json = JSON.parse(await f.text())
  } catch {
    themeMsg.value = `“${f.name}”不是合法的 JSON。`
    return
  }
  const r = themeFromMtb(json)
  if (!r.ok) {
    themeMsg.value = `没有导入：${r.error}`
    return
  }
  themeBusy.value = true
  try {
    await store.save({ theme: json })
    const missing = COLORED_DESC.filter((k) => !r.extended.includes(k))
    themeMsg.value = missing.length
      ? `已导入。没有找到这些描写类型的 extended color，按 MD3 角色回退：${missing.map(descLabel).join('、')}。`
      : '已导入，六种描写类型都用了文件里的 extended color。'
  } catch (err) {
    themeMsg.value = `主题没有保存：${describeError(err)}`
  } finally {
    themeBusy.value = false
  }
}

async function resetTheme() {
  themeBusy.value = true
  try {
    await store.save({ theme: null })
    themeMsg.value = '已恢复 VS Code 默认配色。'
  } catch (e) {
    themeMsg.value = `没有恢复：${describeError(e)}`
  } finally {
    themeBusy.value = false
  }
}
</script>

<template>
  <div class="sk-settings-wrap">
    <StateBlock v-if="store.loading && !store.settings" state="loading" text="正在读取设置" />
    <StateBlock v-else-if="store.error && !store.settings" state="error" :text="`设置读取失败：${store.error}`">
      <v-btn variant="outlined" size="small" @click="store.load()">再试一次</v-btn>
    </StateBlock>

    <div v-else-if="store.settings" class="sk-settings">
      <h1>设置</h1>

      <section aria-labelledby="sk-set-model">
        <h2 id="sk-set-model">模型</h2>
        <p class="sk-muted text-body-2">
          Analyzer 负责分析与高亮，用较快的模型即可；Scaffolder 负责生成建议，用写作能力更强的模型。
        </p>
        <div class="sk-form">
          <v-select v-model="form.provider" :items="providers" label="服务商" />
          <v-text-field v-model="form.baseUrl" label="Base URL（可选）" placeholder="留空使用服务商默认地址" />
          <v-text-field
            v-model="form.apiKey"
            label="API Key"
            type="password"
            autocomplete="off"
            :placeholder="store.settings.apiKeySet ? '已设置。留空表示不修改' : '还没有设置'"
            :hint="store.settings.apiKeySet ? '已设置，出于安全不回显' : '未设置：分析与生成都不可用'"
            persistent-hint
          />
          <v-text-field v-model="form.analyzerModel" label="Analyzer 模型" />
          <v-text-field v-model="form.scaffolderModel" label="Scaffolder 模型" />
          <v-switch v-model="form.autoAnalyze" label="停笔 1.2 秒后自动分析当前段落" />
        </div>
        <div class="d-flex align-center flex-wrap ga-2">
          <v-btn color="primary" variant="flat" :disabled="!dirty" :loading="saving" @click="save">保存设置</v-btn>
          <v-btn variant="outlined" :loading="testing" @click="test">测试连通性</v-btn>
          <span v-if="testResult" :class="testResult.ok ? '' : 'text-error'" class="text-body-2" role="status">
            {{ testResult.ok ? '连通：' : '失败：' }}{{ testResult.message }}
          </span>
        </div>
      </section>

      <section aria-labelledby="sk-set-theme">
        <h2 id="sk-set-theme">外观</h2>
        <div class="d-flex align-center flex-wrap ga-4">
          <span class="text-body-2">明暗</span>
          <v-btn-toggle
            :model-value="store.mode"
            mandatory
            density="compact"
            variant="outlined"
            divided
            aria-label="明暗模式"
            @update:model-value="(m: 'light' | 'dark') => store.setMode(m)"
          >
            <v-btn value="light">亮色</v-btn>
            <v-btn value="dark">暗色</v-btn>
          </v-btn-toggle>
        </div>
        <p class="sk-muted text-body-2">
          默认配色参考 VS Code Light+ / Dark+。也可从 Material Theme Builder 导入：Export 选 JSON，把文件传到这里。
          描写类型的高亮颜色读取文件里名为 appearance、speech、action、psychology、expression、environment 的 extended color，
          没有的按 Material 3 角色回退。
        </p>
        <input ref="themeFile" type="file" accept="application/json,.json" hidden @change="onThemeFile" />
        <div class="d-flex align-center flex-wrap ga-2">
          <v-btn variant="outlined" :loading="themeBusy" @click="pickTheme">导入主题 JSON</v-btn>
          <v-btn :disabled="!store.customTheme || themeBusy" @click="resetTheme">恢复默认主题</v-btn>
          <span class="text-body-2 sk-muted">{{ store.customTheme ? '当前：导入的主题' : '当前：VS Code 配色' }}</span>
        </div>
        <p v-if="themeMsg" class="text-body-2" role="status">{{ themeMsg }}</p>
        <p v-if="store.themeError" class="text-body-2 text-error" role="alert">{{ store.themeError }}</p>
      </section>
    </div>
  </div>
</template>

<style scoped>
.sk-settings-wrap {
  height: 100%;
  overflow: auto;
}
.sk-settings {
  max-width: 720px;
  margin: 0 auto;
  padding: 24px 24px 48px;
}
.sk-settings h1 {
  font-size: 22px;
  margin: 0;
}
.sk-settings h2 {
  font-size: 15px;
  margin: 0;
}
.sk-settings section { display: flex; flex-direction: column; gap: 16px; margin-top: 24px; }
.sk-settings p { margin: 0; line-height: 1.6; }
.sk-settings :deep(.v-input__details) { padding-inline: 0; margin-top: 8px; }
.sk-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
