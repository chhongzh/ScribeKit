import { reactive } from 'vue'

export type TaskStatus = 'preparing' | 'running' | 'completed' | 'cancelled' | 'failed'
export type TaskKind = 'paragraphs' | 'article' | 'inference' | 'text' | 'scaffold'

export interface BackgroundTask {
  id: string
  articleId: number
  title: string
  kind: TaskKind
  status: TaskStatus
  startedAt: number
  finishedAt?: number
  error: string
  cancel: () => void
  open?: () => void
}

export class BackgroundTasks {
  readonly items = reactive<BackgroundTask[]>([])

  get active(): BackgroundTask[] {
    return this.items.filter((task) => task.status === 'preparing' || task.status === 'running')
  }

  start(input: Pick<BackgroundTask, 'id' | 'articleId' | 'title' | 'kind' | 'cancel' | 'open'>): BackgroundTask {
    const existing = this.items.find((task) => task.id === input.id)
    if (existing) return existing
    this.items.unshift({ ...input, status: 'preparing', startedAt: Date.now(), error: '' })
    this.prune()
    return this.items[0]!
  }

  running(id: string): void {
    const task = this.active.find((item) => item.id === id)
    if (task) task.status = 'running'
  }

  finish(id: string, status: 'completed' | 'cancelled' | 'failed', error = ''): void {
    const task = this.active.find((item) => item.id === id)
    if (!task) return
    task.status = status
    task.error = error
    task.finishedAt = Date.now()
    this.prune()
  }

  cancelArticle(articleId: number): void {
    for (const task of this.active.filter((item) => item.articleId === articleId)) task.cancel()
    for (const task of this.items.filter((item) => item.articleId === articleId)) task.open = undefined
  }

  clearFinished(): void {
    this.items.splice(0, this.items.length, ...this.active)
  }

  private prune(): void {
    let finished = this.items.length - this.active.length
    for (let index = this.items.length - 1; index >= 0 && finished > 100; index--) {
      const task = this.items[index]!
      if (task.status === 'preparing' || task.status === 'running') continue
      this.items.splice(index, 1)
      finished--
    }
  }
}
