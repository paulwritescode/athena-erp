// WorkspaceMark shows the Workspace logo at a fixed height, falling back to
// the Athena mark (black on light, white on dark). A lone light logo serves
// both themes. Images never stretch: wide logos letterbox inside the cap. A
// logo that fails to load falls back to the mark.
<script setup lang="ts">
import { computed } from 'vue'
import markDark from '@/assets/brand/athena-white.png'
import markLight from '@/assets/brand/athena-black.png'
import { identity, logoUrl } from '@/identity'

const props = withDefaults(defineProps<{ height?: number }>(), { height: 24 })

const light = computed(() => logoUrl(identity.value, 'light') || markLight)
const dark = computed(() => logoUrl(identity.value, 'dark') || markDark)

function onError(event: Event, fallback: string) {
  const img = event.target as HTMLImageElement | null
  if (!img || img.dataset.fbk) return
  img.dataset.fbk = '1'
  img.src = fallback
}
</script>

<template>
  <span class="inline-flex shrink-0 items-center" :style="{ height: `${props.height}px` }">
    <img
      :src="light"
      alt=""
      class="h-full w-auto max-w-36 rounded-md object-contain dark:hidden"
      @error="onError($event, markLight)"
    />
    <img
      :src="dark"
      alt=""
      class="hidden h-full w-auto max-w-36 rounded-md object-contain dark:block"
      @error="onError($event, markDark)"
    />
  </span>
</template>
