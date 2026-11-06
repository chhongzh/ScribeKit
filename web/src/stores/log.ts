import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Envelope } from '../api/ws'

export interface LogEntry {
  seq: number
  time: string
  dir: 'out' | 'in' | 'sys'
  text: string
  detail?: string
}

const LIMIT = 500

/** 底部 Output 面板：Agent 请求日志（WebSocket 收发）。 */
export const useLogStore = defineStore('log', () => {
  const entries = ref<LogEntry[]>([])
  let seq = 0

  function add(dir: LogEntry['dir'], text: string, msg?: Envelope) {
    seq++
    const now = new Date()
    const time = now.toLocaleTimeString('zh-CN', { hour12: false }) + '.' + String(now.getMilliseconds()).padStart(3, '0')
    let detail: string | undefined
    if (msg) {
      const s = JSON.stringify(msg.payload)
      detail = s.length > 600 ? s.slice(0, 600) + `…（共 ${s.length} 字符）` : s
    }
    entries.value.push({ seq, time, dir, text, detail })
    if (entries.value.length > LIMIT) entries.value.splice(0, entries.value.length - LIMIT)
  }

  function clear() {
    entries.value = []
  }

  return { entries, add, clear }
})
