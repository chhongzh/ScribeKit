<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, describeError } from '../../api/http'
import type { GuideDetail } from '../../api/types'
import { genreLabel } from '../../core/labels'
import { renderMarkdown } from '../../core/markdown'
import { useEditorStore } from '../../stores/editor'
import StateBlock from '../StateBlock.vue'

const props = defineProps<{ guideId: string }>()
const editor = useEditorStore()
const guide = ref<GuideDetail | null>(null)
const loading = ref(false)
const error = ref('')

const html = computed(() => (guide.value ? renderMarkdown(guide.value.body) : ''))

async function load() {
  loading.value = true
  error.value = ''
  try {
    guide.value = await api.getGuide(props.guideId)
    editor.guideTitles[props.guideId] = guide.value.title
  } catch (e) {
    error.value = describeError(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>

<template>
  <div class="sk-guide-wrap">
    <StateBlock v-if="loading && !guide" state="loading" text="正在读取指引" />
    <StateBlock v-else-if="error" state="error" :text="`指引读取失败：${error}`">
      <v-btn variant="outlined" size="small" @click="load">再试一次</v-btn>
    </StateBlock>
    <article v-else-if="guide" class="sk-guide-doc">
      <h1>{{ guide.title }}</h1>
      <p class="sk-muted text-body-2">适用文体：{{ guide.genres.map(genreLabel).join('、') || '通用' }}</p>
      <!-- eslint-disable-next-line vue/no-v-html -- 已经过 DOMPurify 净化 -->
      <div class="sk-md" v-html="html" />
      <section v-if="guide.sources.length" class="sk-guide-src">
        <h2>来源</h2>
        <ul>
          <li v-for="src in guide.sources" :key="src.url">
            <a :href="src.url" target="_blank" rel="noopener noreferrer">{{ src.title }}</a>
            <span class="sk-muted text-caption"> {{ src.url }}</span>
          </li>
        </ul>
      </section>
      <p v-else class="sk-muted text-caption">这条指引没有标注来源。</p>
    </article>
  </div>
</template>

<style scoped>
.sk-guide-wrap {
  height: 100%;
  overflow: auto;
}
.sk-guide-doc {
  max-width: 44em;
  margin: 0 auto;
  padding: 36px 32px 64px;
}
.sk-guide-doc h1 {
  font-family: var(--sk-font-body);
  font-size: 26px;
  margin: 0 0 4px;
}
.sk-md {
  font-family: var(--sk-font-body);
  font-size: 16px;
  line-height: 1.9;
  margin-top: 20px;
}
.sk-md :deep(h2) {
  font-size: 19px;
  margin: 28px 0 8px;
}
.sk-md :deep(h3) {
  font-size: 16px;
  margin: 20px 0 6px;
}
.sk-md :deep(a),
.sk-guide-src a {
  color: rgb(var(--v-theme-primary));
}
.sk-md :deep(blockquote) {
  margin: 12px 0;
  padding-left: 12px;
  border-left: 2px solid var(--sk-line);
  color: var(--sk-muted);
}
.sk-md :deep(code) {
  font-family: var(--sk-font-code);
  font-size: 0.9em;
}
.sk-guide-src {
  margin-top: 32px;
  border-top: 1px solid var(--sk-line);
  padding-top: 12px;
  font-size: 13px;
}
.sk-guide-src h2 {
  font-size: 14px;
  margin: 0 0 6px;
}
.sk-guide-src ul {
  padding-left: 18px;
  line-height: 1.9;
}
</style>
