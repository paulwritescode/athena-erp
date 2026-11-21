// PullToRefresh wraps a list: pulling down from the very top shows a spinner
// in the list header and reloads through refresh (§13.7). Touch only; mouse
// users scroll normally. The page disables the browser's own pull gesture on
// coarse pointers so only this one fires (see style.css).
<script setup lang="ts">
import { onMounted, onUnmounted, ref, useTemplateRef } from 'vue'
import { LoaderCircle } from '@/icons'

const PULL_PX = 64

const emit = defineEmits<{ refresh: [] }>()
const refreshing = defineModel<boolean>('refreshing', { default: false })

const root = useTemplateRef('root')
const pulling = ref(false)
const pulled = ref(0)
let startY = 0
let active = false

function onStart(event: TouchEvent) {
  if (refreshing.value || event.touches.length !== 1 || window.scrollY > 0) return
  const touch = event.touches[0]
  if (!touch) return
  active = true
  startY = touch.clientY
  pulled.value = 0
}

function onMove(event: TouchEvent) {
  if (!active || event.touches.length !== 1) return
  const touch = event.touches[0]
  if (!touch) return
  const dy = touch.clientY - startY
  if (dy <= 0) {
    pulling.value = false
    pulled.value = 0
    return
  }
  pulling.value = true
  pulled.value = Math.min(dy, PULL_PX * 2)
}

function onEnd() {
  if (!active) return
  active = false
  const commit = pulling.value && pulled.value >= PULL_PX
  pulling.value = false
  pulled.value = 0
  if (commit && !refreshing.value) emit('refresh')
}

onMounted(() => {
  const el = root.value
  if (!el) return
  el.addEventListener('touchstart', onStart, { passive: true })
  el.addEventListener('touchmove', onMove, { passive: true })
  el.addEventListener('touchend', onEnd, { passive: true })
  el.addEventListener('touchcancel', onEnd)
})
onUnmounted(() => {
  const el = root.value
  if (!el) return
  el.removeEventListener('touchstart', onStart)
  el.removeEventListener('touchmove', onMove)
  el.removeEventListener('touchend', onEnd)
  el.removeEventListener('touchcancel', onEnd)
})
</script>

<template>
  <div ref="root">
    <div
      v-if="pulling || refreshing"
      class="flex items-center justify-center gap-2 overflow-hidden transition-[height] duration-athena-fast"
      :style="{ height: refreshing ? '40px' : `${Math.min(40, Math.round(pulled / 2))}px` }"
      role="status"
      aria-label="Refreshing list"
    >
      <LoaderCircle class="size-4 animate-spin" aria-hidden="true" />
      <span class="text-caption text-muted-foreground">{{ refreshing ? 'Refreshing…' : 'Pull to refresh' }}</span>
    </div>
    <slot />
  </div>
</template>
