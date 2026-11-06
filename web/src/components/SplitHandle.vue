<script setup lang="ts">
// 可拖动的分隔条。键盘：聚焦后方向键每次 16px，Home/End 到最小/最大。
const props = defineProps<{
  direction: 'vertical' | 'horizontal'
  min: number
  max: number
  label: string
  /** 拖动方向与尺寸增长方向相反（右侧检查器、底部面板） */
  invert?: boolean
}>()
const size = defineModel<number>({ required: true })

function clamp(v: number) {
  return Math.round(Math.min(props.max, Math.max(props.min, v)))
}

function onPointerDown(e: PointerEvent) {
  const el = e.currentTarget as HTMLElement
  el.setPointerCapture(e.pointerId)
  const start = props.direction === 'vertical' ? e.clientX : e.clientY
  const initial = size.value
  const move = (ev: PointerEvent) => {
    const cur = props.direction === 'vertical' ? ev.clientX : ev.clientY
    const delta = (cur - start) * (props.invert ? -1 : 1)
    size.value = clamp(initial + delta)
  }
  const up = () => {
    el.removeEventListener('pointermove', move)
    el.removeEventListener('pointerup', up)
    el.removeEventListener('pointercancel', up)
    document.body.style.cursor = ''
  }
  document.body.style.cursor = props.direction === 'vertical' ? 'col-resize' : 'row-resize'
  el.addEventListener('pointermove', move)
  el.addEventListener('pointerup', up)
  el.addEventListener('pointercancel', up)
}

function onKey(e: KeyboardEvent) {
  const grow = props.direction === 'vertical' ? ['ArrowRight'] : ['ArrowDown']
  const shrink = props.direction === 'vertical' ? ['ArrowLeft'] : ['ArrowUp']
  let d = 0
  if (grow.includes(e.key)) d = 16
  else if (shrink.includes(e.key)) d = -16
  else if (e.key === 'Home') size.value = props.min
  else if (e.key === 'End') size.value = props.max
  else return
  e.preventDefault()
  if (d) size.value = clamp(size.value + d * (props.invert ? -1 : 1))
}
</script>

<template>
  <div
    class="sk-split"
    :class="`sk-split-${direction}`"
    role="separator"
    tabindex="0"
    :aria-orientation="direction"
    :aria-label="label"
    :aria-valuenow="size"
    :aria-valuemin="min"
    :aria-valuemax="max"
    @pointerdown="onPointerDown"
    @keydown="onKey"
  />
</template>

<style scoped>
.sk-split {
  flex: none;
  position: relative;
  z-index: 2;
  touch-action: none;
}
.sk-split-vertical {
  width: 5px;
  margin: 0 -2px;
  cursor: col-resize;
}
.sk-split-horizontal {
  height: 5px;
  margin: -2px 0;
  cursor: row-resize;
}
.sk-split:hover,
.sk-split:focus-visible {
  background-color: rgba(var(--v-theme-primary), 0.5);
  outline: none;
}
</style>
