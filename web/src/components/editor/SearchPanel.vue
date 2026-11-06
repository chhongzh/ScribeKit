<script setup lang="ts">
import { mdiChevronDown, mdiChevronUp, mdiClose } from '@mdi/js'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { searchDocument } from '../../editor/search'
import type { ArticleSession } from '../../editor/session'
import { useNotifyStore } from '../../stores/notify'

const props = defineProps<{ session: ArticleSession }>()
const replace = defineModel<boolean>('replace', { default: false })
const emit = defineEmits<{ close: [] }>()
const notify = useNotifyStore()
const query = ref('')
const replacement = ref('')
const matchCase = ref(false)
const regex = ref(false)
const active = ref(0)
const input = ref<{ focus(): void; select(): void } | null>(null)
const document = computed(() => { void props.session.ui.revision; return props.session.doc })
const result = computed(() => searchDocument(document.value, query.value, replacement.value, matchCase.value, regex.value))

watch([query, matchCase, regex, document], () => {
  active.value = Math.min(active.value, Math.max(0, result.value.matches.length - 1))
  props.session.showSearch(result.value.matches, active.value)
})
watch(replacement, () => props.session.showSearch(result.value.matches, active.value))
onBeforeUnmount(() => props.session.showSearch([], -1))

function step(direction: number) {
  const length = result.value.matches.length
  if (!length) return
  active.value = (active.value + direction + length) % length
  props.session.showSearch(result.value.matches, active.value, true)
}

function replaceMatches(all: boolean) {
  const matches = all ? result.value.matches : [result.value.matches[active.value]].filter((match) => !!match)
  props.session.replaceMatches(matches)
  if (all) notify.push(`已替换 ${matches.length} 处。Ctrl+Z 可撤销。`)
}

async function focus() { await nextTick(); input.value?.focus(); input.value?.select() }
defineExpose({ focus })
</script>

<template>
  <v-card class="sk-search-panel" elevation="4" aria-label="查找与替换" @keydown.esc.stop.prevent="emit('close')">
    <v-card-text class="pa-3">
      <div class="d-flex align-center ga-1">
        <v-btn :icon="replace ? mdiChevronUp : mdiChevronDown" size="small" :aria-label="replace ? '收起替换' : '展开替换'" @click="replace = !replace" />
        <v-text-field ref="input" v-model="query" label="查找" autofocus hide-details :error="!!result.error" @keydown.enter.prevent="step($event.shiftKey ? -1 : 1)" />
        <v-btn size="small" :variant="matchCase ? 'tonal' : 'text'" :aria-pressed="matchCase" title="区分大小写" aria-label="区分大小写" @click="matchCase = !matchCase">Aa</v-btn>
        <v-btn size="small" :variant="regex ? 'tonal' : 'text'" :aria-pressed="regex" title="使用正则表达式" aria-label="使用正则表达式" @click="regex = !regex">.*</v-btn>
        <v-btn :icon="mdiClose" size="small" aria-label="关闭查找" @click="emit('close')" />
      </div>
      <div class="d-flex align-center ga-1 mt-2">
        <span class="text-caption flex-grow-1" role="status">{{ result.error || (result.matches.length ? `${active + 1} / ${result.matches.length}` : query ? '没有匹配项' : '输入要查找的文字') }}</span>
        <v-btn :icon="mdiChevronUp" size="small" aria-label="上一个匹配项" :disabled="!result.matches.length" @click="step(-1)" />
        <v-btn :icon="mdiChevronDown" size="small" aria-label="下一个匹配项" :disabled="!result.matches.length" @click="step(1)" />
      </div>
      <div v-if="replace" class="d-flex align-center flex-wrap ga-2 mt-2">
        <v-text-field v-model="replacement" label="替换为" hide-details @keydown.enter.prevent="replaceMatches(false)" />
        <v-btn :disabled="!result.matches.length" @click="replaceMatches(false)">替换</v-btn>
        <v-btn :disabled="!result.matches.length" @click="replaceMatches(true)">全部替换</v-btn>
      </div>
    </v-card-text>
  </v-card>
</template>

<style scoped>
.sk-search-panel { position: absolute; top: 8px; right: 16px; z-index: 3; width: min(520px, calc(100% - 32px)); }
</style>
