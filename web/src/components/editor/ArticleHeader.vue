<script setup lang="ts">
import { mdiChevronDown, mdiChevronRight } from '@mdi/js'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, describeError } from '../../api/http'
import type { Genre } from '../../api/types'
import { GENRES } from '../../core/labels'
import type { ArticleSession } from '../../editor/session'
import { blockedReason } from '../../editor/scaffold'
import { useWorkspaceStore } from '../../stores/workspace'

const props = defineProps<{ session: ArticleSession }>()
const emit = defineEmits<{ (e: 'enter-body'): void }>()

const workspace = useWorkspaceStore()
const meta = props.session.meta
const synopsisOpen = ref(!meta.synopsis.trim())
const saveState = ref<'idle' | 'saving' | 'error'>('idle')
const saveError = ref('')
let timer: ReturnType<typeof setTimeout> | null = null
let pending: Partial<{ title: string; synopsis: string; genre: Genre; subgenre: string }> = {}

const blocked = computed(() => blockedReason(props.session))
const synopsisPreview = computed(() => meta.synopsis.split('\n').find((l) => l.trim()) ?? '')

function queue(patch: typeof pending) {
  pending = { ...pending, ...patch }
  saveState.value = 'saving'
  if (timer) clearTimeout(timer)
  timer = setTimeout(flush, 800)
}

async function flush() {
  timer = null
  const body = pending
  pending = {}
  if (Object.keys(body).length === 0) return
  try {
    await api.updateArticle(meta.id, body)
    workspace.patchSummary(meta.id, body)
    saveState.value = Object.keys(pending).length ? 'saving' : 'idle'
  } catch (e) {
    pending = { ...body, ...pending }
    saveState.value = 'error'
    saveError.value = describeError(e)
  }
}

watch(
  () => meta.title,
  (v) => queue({ title: v }),
)
watch(
  () => meta.synopsis,
  (v) => queue({ synopsis: v }),
)
watch(
  () => meta.genre,
  (v) => queue({ genre: v }),
)
watch(
  () => meta.subgenre,
  (v) => queue({ subgenre: v }),
)

onBeforeUnmount(() => {
  if (timer) {
    clearTimeout(timer)
    void flush()
  }
})

function onTitleKey(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault()
    emit('enter-body')
  }
}
</script>

<template>
  <header class="sk-head">
    <input
      v-model="meta.title"
      class="sk-title"
      type="text"
      placeholder="标题"
      aria-label="标题"
      @keydown="onTitleKey"
    />
    <div class="sk-head-row">
      <v-select
        v-model="meta.genre"
        :items="GENRES"
        label="文体"
        density="compact"
        class="sk-genre"
        aria-label="文体"
      />
      <v-text-field v-model="meta.subgenre" label="细分" density="compact" class="sk-subgenre" placeholder="如：写人记叙文" />
      <span class="sk-head-save sk-muted text-caption" role="status">
        <template v-if="saveState === 'saving'">保存中</template>
        <span v-else-if="saveState === 'error'" class="text-error">标题或梗概没有保存：{{ saveError }}
          <button type="button" class="sk-plain text-decoration-underline" @click="flush">重试</button>
        </span>
      </span>
    </div>

    <div class="sk-synopsis">
      <button
        type="button"
        class="sk-synopsis-toggle"
        :aria-expanded="synopsisOpen"
        aria-controls="sk-synopsis-body"
        @click="synopsisOpen = !synopsisOpen"
      >
        <v-icon :icon="synopsisOpen ? mdiChevronDown : mdiChevronRight" size="16" />
        <span>梗概</span>
        <span v-if="!synopsisOpen" class="sk-synopsis-preview">{{ synopsisPreview || '还没写' }}</span>
      </button>
      <textarea
        v-show="synopsisOpen"
        id="sk-synopsis-body"
        v-model="meta.synopsis"
        class="sk-synopsis-text"
        rows="4"
        placeholder="草稿区，可以凌乱：想写什么，从哪里起笔，最后落到什么上。Scaffolder 只从这里和正文了解你的意图。"
        aria-label="梗概"
      />
    </div>

    <p v-if="blocked" class="sk-principle" role="note">{{ blocked }}。生成、补全、润色都会等你先动笔。</p>
  </header>
</template>

<style scoped>
.sk-head {
  margin-bottom: 28px;
}
.sk-title {
  width: 100%;
  font-family: var(--sk-font-body);
  font-size: 28px;
  font-weight: 600;
  line-height: 1.5;
  border: 0;
  background: none;
  color: rgb(var(--v-theme-on-surface));
  padding: 4px 0;
}
.sk-title::placeholder,
.sk-synopsis-text::placeholder {
  color: var(--sk-muted);
}
.sk-title:focus-visible {
  outline: none;
  box-shadow: 0 1px 0 rgb(var(--v-theme-primary));
}
.sk-head-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 0 12px;
}
.sk-genre {
  flex: 0 0 140px;
}
.sk-subgenre {
  flex: 0 0 200px;
}
.sk-head-save {
  flex: 1;
  text-align: right;
}
.sk-synopsis {
  border: 1px solid var(--sk-line);
  border-radius: 3px;
}
.sk-synopsis-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-height: 32px;
  padding: 0 8px;
  background: none;
  border: 0;
  color: inherit;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
  text-align: left;
}
.sk-synopsis-preview {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--sk-muted);
  font-family: var(--sk-font-body);
}
.sk-synopsis-text {
  display: block;
  width: 100%;
  resize: vertical;
  border: 0;
  border-top: 1px dashed var(--sk-line);
  padding: 8px 12px;
  font-family: var(--sk-font-body);
  font-size: 15px;
  line-height: 1.8;
  color: rgb(var(--v-theme-on-surface));
  background: none;
}
.sk-synopsis-text:focus-visible {
  outline-offset: -2px;
}
.sk-principle {
  margin: 12px 0 0;
  font-size: 13px;
  color: rgb(var(--v-theme-on-surface));
  padding-left: 10px;
  border-left: 2px solid rgb(var(--v-theme-primary));
}
</style>
