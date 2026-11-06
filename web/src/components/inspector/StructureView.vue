<script setup lang="ts">
import { computed } from 'vue'
import { kindLabel } from '../../core/labels'
import { sliceCp } from '../../core/offsets'
import type { ArticleSession } from '../../editor/session'
import { useAnalysisStore } from '../../stores/analysis'
import StateBlock from '../StateBlock.vue'

// 文章级结构分析：Analyzer 认出的段落作用、套用结构的对照、点题位置。对照不打分。

const props = defineProps<{ session: ArticleSession }>()
const analysis = useAnalysisStore()

const st = computed(() => analysis.st(props.session.articleId))
const result = computed(() => analysis.byArticle[props.session.articleId] ?? null)

const paragraphs = computed(() => {
  void props.session.ui.revision
  const r = result.value
  if (!r) return []
  return r.paragraphs.map((p) => {
    const key = props.session.sync.keyOf(p.paragraphId)
    const f = key ? props.session.find(key) : null
    return {
      ...p,
      key,
      index: f ? f.index : null,
      userKind: f ? (f.node.attrs.kind as string) : null,
    }
  })
})

const hits = computed(() => {
  void props.session.ui.revision
  const r = result.value
  if (!r) return []
  return r.themeHits.map((h, i) => {
    const key = props.session.sync.keyOf(h.paragraphId)
    const f = key ? props.session.find(key) : null
    const text = f?.node.textContent ?? ''
    return { i, key, index: f?.index ?? null, start: h.start, end: h.end, word: f ? sliceCp(text, h.start, h.end) : '' }
  })
})
</script>

<template>
  <div class="sk-struct">
    <div class="d-flex align-center ga-2 mb-2">
      <v-btn size="small" variant="outlined" :loading="st.articleLoading" @click="analysis.analyzeArticle(session)">
        {{ result ? '重新分析全文结构' : '分析全文结构' }}
      </v-btn>
    </div>

    <StateBlock v-if="st.articleLoading && !result" state="loading" text="Analyzer 正在通读全文" />
    <StateBlock v-else-if="st.error && !result" state="error" :text="st.error" />
    <StateBlock
      v-else-if="!result"
      state="empty"
      text="还没有全文结构分析。它会给出每段起的作用、点题字眼出现的位置，以及与所选文体常见结构的对照。"
    />

    <template v-else>
      <section v-if="result.structure.name">
        <h3>结构对照：{{ result.structure.name }}</h3>
        <p>{{ result.structure.fit }}</p>
        <ul v-if="result.structure.gaps.length">
          <li v-for="(g, i) in result.structure.gaps" :key="i">{{ g }}</li>
        </ul>
      </section>

      <section>
        <h3>点题</h3>
        <p v-if="result.themeWords.length">点题字眼：{{ result.themeWords.join('、') }}</p>
        <p v-else class="sk-muted">Analyzer 没有认出点题字眼。</p>
        <ul v-if="hits.length" class="sk-hits">
          <li v-for="h in hits" :key="h.i">
            <button v-if="h.key" type="button" class="sk-plain text-decoration-underline" @click="session.reveal(h.key, h.start, h.end)">
              第 {{ (h.index ?? 0) + 1 }} 段 “{{ h.word }}”
            </button>
            <span v-else class="sk-muted">所在段落已删除</span>
          </li>
        </ul>
        <p v-else-if="result.themeWords.length" class="sk-muted">正文里没有找到点题字眼出现的位置。</p>
      </section>

      <section>
        <h3>各段作用</h3>
        <ol class="sk-roles">
          <li v-for="p in paragraphs" :key="p.paragraphId">
            <button v-if="p.key" type="button" class="sk-plain" @click="session.reveal(p.key, 0)">
              <strong>第 {{ (p.index ?? 0) + 1 }} 段</strong> {{ p.role }}
            </button>
            <span v-else class="sk-muted">已删除的段落：{{ p.role }}</span>
            <p v-if="p.userKind && p.detectedKind !== p.userKind" class="sk-muted text-caption mb-0">
              你标的是{{ kindLabel(p.userKind) }}，Analyzer 读起来像{{ kindLabel(p.detectedKind) }}。
            </p>
            <p v-if="p.note" class="sk-muted text-caption mb-0">{{ p.note }}</p>
          </li>
        </ol>
      </section>

      <section v-if="result.notes.length">
        <h3>其他</h3>
        <ul>
          <li v-for="(n, i) in result.notes" :key="i">{{ n }}</li>
        </ul>
      </section>
      <p class="sk-muted text-caption">全文结构分析不会随打字自动更新，正文改动较多后重新分析一次。</p>
    </template>
  </div>
</template>

<style scoped>
.sk-struct {
  padding: 10px 12px 16px;
  font-size: 13px;
  line-height: 1.7;
}
.sk-struct h3 {
  font-size: 13px;
  margin: 14px 0 4px;
}
.sk-struct p {
  margin: 0 0 4px;
}
.sk-struct ul,
.sk-struct ol {
  padding-left: 18px;
  margin: 0;
}
.sk-roles li + li {
  margin-top: 4px;
}
.sk-roles button {
  text-align: left;
}
</style>
