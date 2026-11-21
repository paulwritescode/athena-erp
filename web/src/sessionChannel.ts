// Cross-tab session sync (B7, R7.6): sign-out, Workspace change, permission
// change and activity reach every tab within 2 seconds over
// BroadcastChannel("athena-session"), falling back to the storage event
// where BroadcastChannel is missing.
export type SessionChannelKind = 'signed-out' | 'workspace-changed' | 'permissions-changed' | 'activity'
export type SessionChannelMessage = { kind: SessionChannelKind; at: number }

const CHANNEL = 'athena-session'
const STORAGE_KEY = 'athena-session-message'

type Listener = (message: SessionChannelMessage) => void

function readable(value: unknown): SessionChannelMessage | null {
  if (typeof value !== 'object' || value === null) return null
  const kind = (value as Record<string, unknown>).kind
  const at = (value as Record<string, unknown>).at
  if ((kind === 'signed-out' || kind === 'workspace-changed' || kind === 'permissions-changed' || kind === 'activity') && typeof at === 'number') {
    return { kind, at }
  }
  return null
}

export function postSessionMessage(kind: SessionChannelKind): void {
  const message: SessionChannelMessage = { kind, at: Date.now() }
  if (typeof BroadcastChannel !== 'undefined') {
    new BroadcastChannel(CHANNEL).postMessage(message)
    return
  }
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(message))
  } catch {
    // A blocked store degrades to single-tab behavior.
  }
}

export function onSessionMessage(listener: Listener): () => void {
  if (typeof BroadcastChannel !== 'undefined') {
    const channel = new BroadcastChannel(CHANNEL)
    const onMessage = (event: MessageEvent) => {
      const message = readable(event.data)
      if (message) listener(message)
    }
    channel.addEventListener('message', onMessage)
    return () => {
      channel.removeEventListener('message', onMessage)
      channel.close()
    }
  }
  const onStorage = (event: StorageEvent) => {
    if (event.key !== STORAGE_KEY || !event.newValue) return
    try {
      const message = readable(JSON.parse(event.newValue))
      if (message) listener(message)
    } catch {
      // Ignore a corrupt payload.
    }
  }
  window.addEventListener('storage', onStorage)
  return () => window.removeEventListener('storage', onStorage)
}
