// AsciiLogo draws the Athena logo a placement key names: one frame per
// step, top to bottom, then rests on the full mark. Both theme variants
// render and CSS shows the live one, as the workspace mark does. The mark
// is decoration, so it stays out of the access tree. Reduced motion
// renders the full mark at once with no timer. A key that is not listed,
// or that names a figure, renders nothing.
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useArt, type LogoPlacement } from './registry'
import './art.css'

const props = defineProps<{ artKey: string }>()

const logoEntry = computed<LogoPlacement | undefined>(() => {
  const found = useArt(props.artKey)
  return found?.kind === 'logo' ? found : undefined
})
const frame = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

function lastFrame(): number {
  const entry = logoEntry.value
  return entry ? entry.light.length - 1 : 0
}

function reducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

onMounted(() => {
  const entry = logoEntry.value
  if (!entry) return
  if (reducedMotion()) {
    frame.value = lastFrame()
    return
  }
  timer = setInterval(() => {
    if (frame.value >= lastFrame()) {
      if (timer !== null) clearInterval(timer)
      timer = null
      return
    }
    frame.value += 1
  }, entry.stepMs)
})

onUnmounted(() => {
  if (timer !== null) clearInterval(timer)
  timer = null
})
</script>

<template>
  <div v-if="logoEntry" class="ascii-logo" aria-hidden="true">
    <pre class="ascii-logo-frame dark:hidden"><span
      v-for="(line, row) in logoEntry.light[frame]"
      :key="row === frame - 1 ? `new-${frame}` : `row-${row}`"
      class="ascii-logo-line"
      :class="row === frame - 1 ? 'ascii-logo-line-new' : undefined"
    >{{ line === '' ? ' ' : line }}</span></pre>
    <pre class="ascii-logo-frame hidden dark:block"><span
      v-for="(line, row) in logoEntry.dark[frame]"
      :key="row === frame - 1 ? `new-${frame}` : `row-${row}`"
      class="ascii-logo-line"
      :class="row === frame - 1 ? 'ascii-logo-line-new' : undefined"
    >{{ line === '' ? ' ' : line }}</span></pre>
  </div>
</template>
