<script setup lang="ts">
import { computed } from 'vue'
import { kindLabel } from '../../core/labels'
import { undecidedCount } from '../../core/review'
import type { ArticleSession } from '../../editor/session'
import { useSuggestionStore } from '../../stores/suggestions'
import { useUiStore } from '../../stores/ui'
import StateBlock from '../StateBlock.vue'
import SuggestionReview from './SuggestionReview.vue'

const props = defineProps<{ session: ArticleSession }>()
const sg = useSuggestionStore()
const ui = useUiStore()

const items = computed(() => sg.list(props.session.articleId))
const streams = computed(() => sg.streams.filter((x) => x.articleId === props.session.articleId))
const load = computed(() => sg.loadState[props.session.articleId])
const current = computed(() => items.value.find((x) => x.id === ui.reviewing) ?? null)

const OP: Record<string, string> = { replace: '替换', insert_paragraph: '新段落', advice: '只读建议' }
</script>

<template>
  <div class="sk-sglist">
    <section v-if="streams.length" class="sk-streams" aria-label="生成中">
      <div v-for="st in streams" :key="st.requestId" class="sk-stream">
        <div class="d-flex align-center">
          <span class="flex-1-1 text-body-2">{{ st.title }} · 生成中</span>
          <v-btn size="x-small" variant="outlined" @click="sg.cancel(st.requestId)">取消（Esc）</v-btn>
        </div>
        <p class="sk-stream-text">{{ st.text || '等待第一段文字' }}</p>
      </div>
    </section>

    <SuggestionReview v-if="current" :key="current.id" :suggestion="current" :session="session" @close="ui.reviewing = null" />

    <template v-else>
      <StateBlock v-if="load?.loading && items.length === 0" state="loading" text="正在读取待评审的建议" />
      <StateBlock v-else-if="load?.error" state="error" :text="`建议读取失败：${load.error}`">
        <v-btn variant="outlined" size="small" @click="sg.load(session.articleId)">重新读取</v-btn>
      </StateBlock>
      <StateBlock
        v-else-if="items.length === 0 && streams.length === 0"
        state="empty"
        text="没有待评审的建议。在正文里按 Shift+Enter 或 Ctrl+K，Scaffolder 的建议会先出现在这里，逐半句确认后才写入正文。"
      />
      <ul v-else class="sk-sg-items">
        <li v-for="s in items" :key="s.id">
          <button type="button" class="sk-sg-item" @click="ui.review(s.id)">
            <span class="sk-sg-item-head">
              <span>{{ s.instruction || '建议' }}</span>
              <span class="sk-muted text-caption">{{ OP[s.op] }}<template v-if="s.op !== 'advice'"> · {{ kindLabel(s.paragraphKind) }}</template></span>
            </span>
            <span class="sk-sg-item-text">{{ s.op === 'advice' ? (s.rationale || s.text) : s.text }}</span>
            <span v-if="s.op !== 'advice'" class="sk-muted text-caption">
              {{ undecidedCount(s.halves) ? `还有 ${undecidedCount(s.halves)} 个半句未决定` : '全部半句已决定，可以合入' }}
            </span>
          </button>
        </li>
      </ul>
    </template>
  </div>
</template>

<style scoped>
.sk-streams {
  border-bottom: 1px solid var(--sk-line);
  padding: 8px 12px;
}
.sk-stream + .sk-stream {
  margin-top: 8px;
}
.sk-stream-text {
  margin: 4px 0 0;
  font-family: var(--sk-font-body);
  font-size: 14px;
  line-height: 1.8;
  white-space: pre-wrap;
}
.sk-sg-items {
  list-style: none;
  margin: 0;
  padding: 4px 0;
}
.sk-sg-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  padding: 8px 12px;
  background: none;
  border: 0;
  border-bottom: 1px solid var(--sk-line);
  color: inherit;
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.sk-sg-item:hover {
  background-color: rgba(var(--v-theme-on-surface), var(--v-hover-opacity));
}
.sk-sg-item:focus-visible {
  outline-offset: -2px;
}
.sk-sg-item-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.sk-sg-item-text {
  font-family: var(--sk-font-body);
  font-size: 14px;
  line-height: 1.7;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
