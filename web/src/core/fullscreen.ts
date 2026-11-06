export async function toggleFullscreen(): Promise<void> {
  if (document.fullscreenElement) await document.exitFullscreen()
  else await document.documentElement.requestFullscreen()
}
