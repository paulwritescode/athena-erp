// AsciiMark renders the ASCII block a placement key names, in caption mono
// (§9.6). Marks are decoration, so they stay out of the access tree. A key
// that is not listed, or that names a figure, renders nothing.
<script setup lang="ts">
import { computed } from 'vue'
import { useArt, type AsciiPlacement } from './registry'
import './art.css'

const props = defineProps<{ artKey: string }>()

const asciiEntry = computed<AsciiPlacement | undefined>(() => {
  const found = useArt(props.artKey)
  return found?.kind === 'ascii' ? found : undefined
})
</script>

<template>
  <pre v-if="asciiEntry" class="ascii-mark" aria-hidden="true">{{ asciiEntry.block }}</pre>
</template>
