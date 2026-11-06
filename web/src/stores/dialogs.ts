import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface PromptRequest {
  kind: 'prompt'
  title: string
  label: string
  value: string
  confirm: string
  multiline?: boolean
  resolve: (v: string | null) => void
}

export interface ConfirmRequest {
  kind: 'confirm'
  title: string
  text: string
  confirm: string
  danger?: boolean
  resolve: (v: boolean) => void
}

/** 输入框与确认框。返回 Promise，调用处直接 await。 */
export const useDialogStore = defineStore('dialogs', () => {
  const current = ref<PromptRequest | ConfirmRequest | null>(null)

  function prompt(o: { title: string; label: string; value?: string; confirm?: string; multiline?: boolean }) {
    return new Promise<string | null>((resolve) => {
      current.value = {
        kind: 'prompt',
        title: o.title,
        label: o.label,
        value: o.value ?? '',
        confirm: o.confirm ?? '确定',
        multiline: o.multiline,
        resolve,
      }
    })
  }

  function confirm(o: { title: string; text: string; confirm: string; danger?: boolean }) {
    return new Promise<boolean>((resolve) => {
      current.value = { kind: 'confirm', ...o, resolve }
    })
  }

  function close() {
    current.value = null
  }

  return { current, prompt, confirm, close }
})
