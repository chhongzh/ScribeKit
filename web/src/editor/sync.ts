import { reactive } from 'vue'
import { api, describeError } from '../api/http'
import type { Paragraph } from '../api/types'
import { nextOp, type LocalRow, type Op, type ServerRow } from './plan'

// 把编辑器里的段落序列同步到后端。所有请求串行执行；每一步成功后更新本地记录的服务器状态，
// 下一步重新规划，所以用户在请求途中继续打字也不会错位。失败时停下，等用户选择重试或按服务器版本重新载入。

const TEXT_DEBOUNCE = 700
const MAX_STEPS = 400

export interface SyncState {
  status: 'idle' | 'saving' | 'error'
  error: string
  failedOp: string
}

export class SyncEngine {
  readonly state = reactive<SyncState>({ status: 'idle', error: '', failedOp: '' })
  private server: ServerRow[]
  private running: Promise<void> | null = null
  private rerun = false
  private textTimer: ReturnType<typeof setTimeout> | null = null
  private forceText = false
  private listeners = new Set<(rows: readonly ServerRow[]) => void>()

  constructor(
    rows: ServerRow[],
    private readonly articleId: number,
    private readonly readLocal: () => LocalRow[],
  ) {
    this.server = rows.slice()
  }

  get rows(): readonly ServerRow[] {
    return this.server
  }

  idOf(key: string): number | null {
    return this.server.find((r) => r.key === key)?.id ?? null
  }

  keyOf(id: number): string | null {
    return this.server.find((r) => r.id === id)?.key ?? null
  }

  serverText(key: string): string | null {
    return this.server.find((r) => r.key === key)?.text ?? null
  }

  onSynced(fn: (rows: readonly ServerRow[]) => void): () => void {
    this.listeners.add(fn)
    return () => this.listeners.delete(fn)
  }

  /** 编辑器每次文档变化时调用：结构变化立即同步，正文防抖。 */
  notify(): void {
    if (this.state.status === 'error') return
    this.kick(false)
    if (this.textTimer) clearTimeout(this.textTimer)
    this.textTimer = setTimeout(() => {
      this.textTimer = null
      this.kick(true)
    }, TEXT_DEBOUNCE)
  }

  /** 立即把全部改动写入后端，完成后 resolve；失败时 reject。分析、生成、导出前都要先调用。 */
  async flush(): Promise<void> {
    if (this.textTimer) {
      clearTimeout(this.textTimer)
      this.textTimer = null
    }
    if (this.state.status === 'error') throw new Error(this.state.error)
    this.kick(true)
    while (this.running) await this.running
    if ((this.state.status as SyncState['status']) === 'error') throw new Error(this.state.error)
  }

  get pending(): boolean {
    return this.running !== null || this.textTimer !== null
  }

  /** 按服务器版本重新开始（放弃本地未保存的改动时用）。状态对象保持同一个，界面上的绑定不失效。 */
  reset(rows: ServerRow[]): void {
    if (this.textTimer) clearTimeout(this.textTimer)
    this.textTimer = null
    this.server = rows.slice()
    this.rerun = false
    this.forceText = false
    this.state.status = 'idle'
    this.state.error = ''
    this.state.failedOp = ''
    this.emit()
  }

  retry(): void {
    this.state.status = 'idle'
    this.state.error = ''
    this.state.failedOp = ''
    this.kick(true)
  }

  /** 外部（例如合入建议）已经改了服务器上的段落，把结果并入服务器侧记录。 */
  absorb(paragraphs: Paragraph[], keyFor: (p: Paragraph) => string): void {
    for (const p of paragraphs) {
      const existing = this.server.find((r) => r.id === p.id)
      if (existing) {
        existing.text = p.text
        existing.kind = p.kind
        existing.position = p.position
      } else {
        this.server.push({ key: keyFor(p), id: p.id, kind: p.kind, text: p.text, position: p.position })
      }
    }
    this.server.sort((a, b) => a.position - b.position)
    this.emit()
  }

  private emit(): void {
    for (const fn of this.listeners) fn(this.server)
  }

  private kick(withText: boolean): void {
    if (withText) this.forceText = true
    if (this.running) {
      this.rerun = true
      return
    }
    this.running = this.loop().finally(() => {
      this.running = null
      if (this.rerun && this.state.status !== 'error') {
        this.rerun = false
        this.kick(false)
      }
    })
  }

  private async loop(): Promise<void> {
    for (let step = 0; step < MAX_STEPS; step++) {
      const op = nextOp(this.server, this.readLocal(), { text: this.forceText })
      if (!op) {
        this.forceText = false
        if (this.state.status === 'saving') this.state.status = 'idle'
        this.emit()
        return
      }
      this.state.status = 'saving'
      try {
        await this.exec(op)
      } catch (e) {
        this.state.status = 'error'
        this.state.error = describeError(e)
        this.state.failedOp = describeOp(op, this.server)
        this.emit()
        return
      }
    }
    this.state.status = 'error'
    this.state.error = '同步步骤过多，已停止。请按服务器版本重新载入。'
  }

  private index(key: string): number {
    const i = this.server.findIndex((r) => r.key === key)
    if (i < 0) throw new Error(`内部错误：段落 ${key} 不在服务器记录里`)
    return i
  }

  private row(key: string): ServerRow {
    return this.server[this.index(key)]!
  }

  private async exec(op: Op): Promise<void> {
    switch (op.t) {
      case 'text': {
        const r = this.row(op.key)
        await api.updateParagraph(r.id, { text: op.text })
        // 记录我们写入的值而不是返回值：后端若做了规范化，也不会因为对不上而反复提交。
        r.text = op.text
        return
      }
      case 'kind': {
        const r = this.row(op.key)
        await api.updateParagraph(r.id, { kind: op.kind })
        r.kind = op.kind
        return
      }
      case 'delete': {
        const r = this.row(op.key)
        await api.deleteParagraph(r.id)
        this.server.splice(this.index(op.key), 1)
        return
      }
      case 'merge': {
        const r = this.row(op.key)
        const p = await api.mergeNext(r.id)
        r.text = p.text
        r.kind = p.kind
        r.position = p.position
        this.server.splice(this.index(op.removed), 1)
        return
      }
      case 'split': {
        const r = this.row(op.key)
        const [a, b] = await api.splitParagraph(r.id, op.offset)
        r.text = a.text
        r.kind = a.kind
        r.position = a.position
        this.server.splice(this.index(op.key) + 1, 0, {
          key: op.newKey,
          id: b.id,
          kind: b.kind,
          text: b.text,
          position: b.position,
        })
        return
      }
      case 'create': {
        const afterId = op.afterKey === null ? null : this.row(op.afterKey).id
        const p = await api.createParagraph(this.articleId, { afterId, kind: op.kind, text: op.text })
        const at = op.afterKey === null ? 0 : this.index(op.afterKey) + 1
        this.server.splice(at, 0, { key: op.key, id: p.id, kind: p.kind, text: p.text, position: p.position })
        return
      }
      case 'move': {
        const r = this.row(op.key)
        const afterId = op.afterKey === null ? null : this.row(op.afterKey).id
        const p = await api.moveParagraph(r.id, afterId)
        r.position = p.position
        this.server.splice(this.index(op.key), 1)
        const at = op.afterKey === null ? 0 : this.index(op.afterKey) + 1
        this.server.splice(at, 0, r)
        return
      }
    }
  }
}

function describeOp(op: Op, rows: ServerRow[]): string {
  const n = (key: string) => {
    const i = rows.findIndex((r) => r.key === key)
    return i >= 0 ? `第 ${i + 1} 段` : '新段落'
  }
  switch (op.t) {
    case 'text':
      return `保存${n(op.key)}的正文`
    case 'kind':
      return `修改${n(op.key)}的类型`
    case 'delete':
      return `删除${n(op.key)}`
    case 'merge':
      return `把下一段并入${n(op.key)}`
    case 'split':
      return `拆分${n(op.key)}`
    case 'create':
      return '新建段落'
    case 'move':
      return `移动${n(op.key)}`
  }
}
