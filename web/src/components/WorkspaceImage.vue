// WorkspaceImage shows one Workspace image file through FileService: the 256 px
// thumbnail in lists and forms, the original on request. Loading shows a quiet
// box; a file that cannot be read shows a muted placeholder, never a broken
// image. Callers size the box with a class; the image fills it. Bytes ride a
// data: URL because the desk's Content-Security-Policy allows img-src data:
// but not blob:.
<script setup lang="ts">
import { ref, watch } from 'vue'
import { fileClient } from '@/api'
import { imageDataUrl } from '@/images'
import { Image as ImageIcon } from '@/icons'

const props = withDefaults(defineProps<{ path: string; thumbnail?: boolean; alt?: string; contain?: boolean }>(), { thumbnail: false, alt: '', contain: false })

const url = ref('')
const state = ref<'loading' | 'ready' | 'missing'>('loading')
let request = 0

async function load() {
  const at = ++request
  state.value = 'loading'
  url.value = ''
  try {
    const res = await fileClient.getFile({ path: props.path, thumbnail: props.thumbnail })
    if (at !== request) return
    url.value = imageDataUrl(props.path, res.content)
    state.value = 'ready'
  } catch {
    if (at === request) state.value = 'missing'
  }
}

watch(() => [props.path, props.thumbnail] as const, load, { immediate: true })
</script>

<template>
  <div class="workspace-image" :class="{ 'is-contain': contain, 'is-missing': state === 'missing' }">
    <img v-if="state === 'ready'" :src="url" :alt="alt" loading="lazy" />
    <span v-else-if="state === 'missing'" class="workspace-image-missing"><ImageIcon class="size-4" aria-hidden="true" />Image unavailable</span>
    <span v-else class="workspace-image-loading animate-pulse" aria-hidden="true" />
  </div>
</template>
