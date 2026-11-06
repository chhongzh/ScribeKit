import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface Notice {
  id: number
  text: string
  tone: 'info' | 'error'
  action?: { label: string; run: () => void }
  timeout: number
}

/** 全局提示条。一次只显示一条，后来的排队。 */
export const useNotifyStore = defineStore('notify', () => {
  const queue = ref<Notice[]>([])
  let seq = 0

  function push(text: string, opts: Partial<Omit<Notice, 'id' | 'text'>> = {}) {
    seq++
    const n: Notice = { id: seq, text, tone: opts.tone ?? 'info', action: opts.action, timeout: opts.timeout ?? 5000 }
    // 同一句话连续出现只保留一条。
    if (queue.value.some((q) => q.text === text)) return
    queue.value.push(n)
  }

  function error(text: string, action?: Notice['action']) {
    push(text, { tone: 'error', action, timeout: action ? 9000 : 7000 })
  }

  function shift() {
    queue.value.shift()
  }

  return { queue, push, error, shift }
})
