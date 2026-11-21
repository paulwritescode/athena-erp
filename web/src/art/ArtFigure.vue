// ArtFigure mounts the hairline figure a placement key names. The figure
// loads lazily, answers the pointer quietly (intensity 0.3, play off), and
// is destroyed with the component. A key that is not listed, or that names
// ASCII, renders nothing.
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { Figure as HairlineFigure } from '@lucasmarkes/hairline'
import { artFigureOptions, useArt, type FigurePlacement } from './registry'
import './art.css'

const props = defineProps<{ artKey: string }>()

const figureEntry = computed<FigurePlacement | undefined>(() => {
  const found = useArt(props.artKey)
  return found?.kind === 'figure' ? found : undefined
})
const host = ref<HTMLDivElement | null>(null)
let figure: HairlineFigure | null = null
let cancelled = false

onMounted(async () => {
  const entry = figureEntry.value
  if (!entry || host.value === null) return
  const create = await entry.load()
  if (cancelled || host.value === null) return
  figure = create(host.value, artFigureOptions(entry))
})

onUnmounted(() => {
  cancelled = true
  figure?.destroy()
  figure = null
})
</script>

<template>
  <div
    v-if="figureEntry"
    class="art-figure art-figure-enter"
    :aria-hidden="figureEntry.label === undefined ? true : undefined"
    :style="{ '--art-width': `${figureEntry.width}px`, '--art-width-mobile': `${figureEntry.mobileWidth}px` }"
  >
    <div ref="host" />
  </div>
</template>
