// Tracks the browser's own online state, so the desk can show the offline
// page the moment the connection drops and return when it is back.
import { onUnmounted, ref } from 'vue'

export function useOnlineStatus() {
  const online = ref(navigator.onLine)

  function update() {
    online.value = navigator.onLine
  }

  window.addEventListener('online', update)
  window.addEventListener('offline', update)
  onUnmounted(() => {
    window.removeEventListener('online', update)
    window.removeEventListener('offline', update)
  })

  return { online }
}
