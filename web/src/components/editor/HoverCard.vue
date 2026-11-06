<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { descLabel, markLabel, rhetoricLabel } from '../../core/labels'
import { cpToU16, sliceCp } from '../../core/offsets'
import { halfAt, segment } from '../../core/textseg'
import type { ArticleSession } from '../../editor/session'

// 类似 LSP hover：鼠标在半句上停一会儿，显示 Analyzer 对它的识别结果。只陈述，不打分。

const props = defineProps<{ session: ArticleSession }>()

const viewportWidth = window.innerWidth
const shown = ref<typeof props.session.ui.hover>(null)
let timer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.session.ui.hover,
  (h) => {
    if (timer) clearTimeout(timer)
    shown.value = null
    if (h) timer = setTimeout(() => (shown.value = h), 550)
  },
)

const info = computed(() => {
  const h = shown.value
  void props.session.ui.revision
  if (!h) return null
  const f = props.session.find(h.key)
  if (!f) return null
  const text = f.node.textContent
  const a = props.session.validAnalysis(h.key, text)
  if (!a) return null
  const half = halfAt(segment(text), h.cp)
  if (!half) return null
  const ha = a.halves.find((x) => x.index === half.index)
  const marks = a.marks.filter((m) => h.cp >= m.start && h.cp < m.end)
  const diags = a.diagnostics.filter((d) => h.cp >= d.start && h.cp < d.end)
  const view = props.session.view
  let pos: { left: number; top: number } | null = null
  try {
    const c = view?.coordsAtPos(f.pos + 1 + cpToU16(text, half.start), 1)
    if (c) pos = { left: c.left, top: c.bottom + 6 }
  } catch {
    pos = null
  }
  if (!pos) return null
  return {
    pos,
    halfNo: half.index + 1,
    sentenceNo: half.sentence + 1,
    desc: ha?.desc ?? [],
    rhetoric: ha?.rhetoric ?? [],
    marks: marks.map((m) => ({
      label: markLabel(m.kind),
      note: m.note,
      ref: m.ref ? sliceCp(text, m.ref.start, m.ref.end) : null,
    })),
    diags,
  }
})
</script>

<template>
  <div
    v-if="info"
    class="sk-hover bg-surface"
    role="tooltip"
    :style="{ left: `${Math.max(8, Math.min(info.pos.left, viewportWidth - 352))}px`, top: `${info.pos.top}px` }"
  >
    <p class="sk-hover-title">第 {{ info.sentenceNo }} 句 · 第 {{ info.halfNo }} 半句</p>
    <p v-if="info.desc.length">描写：{{ info.desc.map(descLabel).join('、') }}</p>
    <p v-else class="sk-muted">没有识别出描写类型</p>
    <p v-if="info.rhetoric.length">修辞：{{ info.rhetoric.map(rhetoricLabel).join('、') }}</p>
    <p v-for="(m, i) in info.marks" :key="`m${i}`">
      {{ m.label }}<template v-if="m.ref">，指代“{{ m.ref }}”</template><template v-if="m.note">：{{ m.note }}</template>
    </p>
    <p v-for="(d, i) in info.diags" :key="`d${i}`" class="sk-hover-diag">{{ d.message }}</p>
  </div>
</template>

<style scoped>
.sk-hover {
  position: fixed;
  z-index: 1500;
  max-width: 340px;
  pointer-events: none;
  border: 1px solid var(--sk-line);
  border-radius: 4px;
  box-shadow: 0 4px 14px rgba(var(--v-theme-on-surface), 0.14);
  padding: 6px 10px;
  font-size: 12px;
  line-height: 1.6;
}
.sk-hover p {
  margin: 0;
}
.sk-hover-title {
  color: var(--sk-muted);
}
.sk-hover-diag {
  border-top: 1px solid var(--sk-line);
  margin-top: 4px !important;
  padding-top: 4px;
}
</style>
