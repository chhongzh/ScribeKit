<script setup lang="ts">
import { mdiChevronDown, mdiChevronRight } from '@mdi/js'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, describeError } from '../../api/http'
import type { Genre } from '../../api/types'
import { GENRES } from '../../core/labels'
import type { ArticleSession } from '../../editor/session'
import { blockedReason } from '../../editor/scaffold'
import { useWorkspaceStore } from '../../stores/workspace'
import ExpandableTextarea from '../ExpandableTextarea.vue'

const props = defineProps<{ session: ArticleSession }>()
const emit = defineEmits<{ (e: 'enter-body'): void }>()

const workspace = useWorkspaceStore()
const meta = props.session.meta
const synopsisOpen = ref(!meta.synopsis.trim())
const save = props.session.metadataSave
props.session.retryMetadata = flush
let timer: ReturnType<typeof setTimeout> | null = null
let pending: Partial<{ title: string; synopsis: string; genre: Genre; subgenre: string }> = {}

const blocked = computed(() => blockedReason(props.session))
const synopsisPreview = computed(() => meta.synopsis.split('\n').find((l) => l.trim()) ?? '')

function queue(patch: typeof pending) {
  pending = { ...pending, ...patch }
  save.status = 'saving'
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
    save.status = Object.keys(pending).length ? 'saving' : 'idle'
    save.error = ''
  } catch (e) {
    pending = { ...body, ...pending }
    save.status = 'error'
    save.error = describeError(e)
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
  props.session.retryMetadata = null
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
    <v-text-field
      v-model="meta.title"
      class="sk-title mb-3"
      type="text"
      placeholder="标题"
      label="标题"
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
    </div>

    <div class="sk-synopsis">
      <v-btn
        type="button"
        block
        class="justify-start"
        :aria-expanded="synopsisOpen"
        aria-controls="sk-synopsis-body"
        @click="synopsisOpen = !synopsisOpen"
      >
        <v-icon :icon="synopsisOpen ? mdiChevronDown : mdiChevronRight" size="16" />
        <span>梗概</span>
        <span v-if="!synopsisOpen" class="sk-synopsis-preview">{{ synopsisPreview || '还没写' }}</span>
      </v-btn>
      <ExpandableTextarea
        v-show="synopsisOpen"
        id="sk-synopsis-body"
        v-model="meta.synopsis"
        class="mt-2"
        :rows="4"
        auto-grow
        label="梗概"
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
  min-width: 0;
}
.sk-title :deep(input) {
  font-family: var(--sk-font-body);
  font-size: 1.55em;
  font-weight: 600;
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
  flex-wrap: wrap;
  margin: 8px 0 12px;
}
.sk-genre {
  flex: 1 1 140px;
}
.sk-subgenre {
  flex: 1 1 160px;
}
.sk-synopsis {
  min-width: 0;
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
.sk-principle {
  margin: 12px 0 0;
  font-size: 13px;
  color: rgb(var(--v-theme-on-surface));
  padding-left: 10px;
  border-left: 2px solid rgb(var(--v-theme-primary));
}
</style>
