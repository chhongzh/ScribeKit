export type GlobalShortcut = 'explorer' | 'sidebar' | 'panel' | 'inspector' | 'settings' | 'palette' | 'save' | 'find' | 'replace' | 'fullscreen'

export function globalShortcut(event: Pick<KeyboardEvent, 'code' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'altKey' | 'isComposing' | 'keyCode'>): GlobalShortcut | null {
  if (event.isComposing || event.keyCode === 229) return null
  if (event.code === 'F11' && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey) return 'fullscreen'
  if (!(event.ctrlKey || event.metaKey)) return null
  if (event.altKey) return event.code === 'KeyB' && !event.shiftKey ? 'inspector' : null
  if (event.shiftKey) {
    if (event.code === 'KeyE') return 'explorer'
    if (event.code === 'KeyK') return 'palette'
    return null
  }
  switch (event.code) {
    case 'KeyB': return 'sidebar'
    case 'KeyJ': return 'panel'
    case 'Comma': return 'settings'
    case 'KeyK': return 'palette'
    case 'KeyS': return 'save'
    case 'KeyF': return 'find'
    case 'KeyH': return 'replace'
    default: return null
  }
}
