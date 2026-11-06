<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { descLabel, markLabel, rhetoricLabel } from '../../core/labels'
import { cpToU16, sliceCp } from '../../core/offsets'
import { halfAt, segment } from '../../core/textseg'
import type { ArticleSession } from '../../editor/session'

const props = defineProps<{ session: ArticleSession; disabled?: boolean }>()
const shown = ref<typeof props.session.ui.hover>(null)
let timer: ReturnType<typeof setTimeout> | null = null
onBeforeUnmount(() => { if (timer) clearTimeout(timer) })

watch(() => [props.session.ui.hover, props.disabled] as const, ([hover, disabled]) => {
  if (timer) clearTimeout(timer)
  shown.value = null
  if (hover && !disabled) timer = setTimeout(() => { shown.value = hover }, 550)
})

const info = computed(() => {
  void props.session.ui.revision
  const hover = shown.value
  if (!hover) return null
  const found = props.session.find(hover.key)
  if (!found) return null
  const text = found.node.textContent
  const analysis = props.session.validAnalysis(hover.key, text)
  const half = halfAt(segment(text), hover.cp)
  if (!analysis || !half) return null
  const classified = analysis.halves.find((item) => item.index === half.index)
  try {
    const coords = props.session.view?.coordsAtPos(found.pos + 1 + cpToU16(text, half.start), 1)
    if (!coords) return null
    return {
      at: [coords.left, coords.bottom + 4] as [number, number],
      half: half.index + 1, sentence: half.sentence + 1,
      desc: classified?.desc ?? [], rhetoric: classified?.rhetoric ?? [],
      marks: analysis.marks.filter((item) => hover.cp >= item.start && hover.cp < item.end).map((item) => ({ label: markLabel(item.kind), note: item.note, reference: item.ref ? sliceCp(text, item.ref.start, item.ref.end) : '' })),
      diagnostics: analysis.diagnostics.filter((item) => hover.cp >= item.start && hover.cp < item.end),
    }
  } catch { return null }
})
</script>

<template>
  <v-overlay v-if="info" :model-value="true" :target="info.at" location-strategy="connected" location="bottom start" scroll-strategy="close" :scrim="false" :close-on-back="false" :persistent="true" :content-props="{ style: { pointerEvents: 'none' } }" max-width="min(360px, calc(100vw - 24px))">
    <v-card role="tooltip" elevation="4">
      <v-card-text class="text-body-2">
        <p class="text-caption sk-muted mb-2">第 {{ info.sentence }} 句 · 第 {{ info.half }} 半句</p>
        <p class="mb-1">{{ info.desc.length ? `描写：${info.desc.map(descLabel).join('、')}` : '没有识别出描写类型' }}</p>
        <p v-if="info.rhetoric.length" class="mb-1">修辞：{{ info.rhetoric.map(rhetoricLabel).join('、') }}</p>
        <p v-for="(mark, index) in info.marks" :key="index" class="mb-1">{{ mark.label }}<template v-if="mark.reference">，指代“{{ mark.reference }}”</template><template v-if="mark.note">：{{ mark.note }}</template></p>
        <template v-if="info.diagnostics.length"><v-divider class="my-2" /><p v-for="(diagnostic, index) in info.diagnostics" :key="index" class="mb-1">{{ diagnostic.message }}</p></template>
      </v-card-text>
    </v-card>
  </v-overlay>
</template>
