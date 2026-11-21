// ArtPlaceholder stands in for an art block until the B10 art registry lands:
// a quiet dashed panel. The renderer and the builder preview share it so the
// placeholder looks the same in both.
<script setup lang="ts">
import { Image as ImageIcon } from '@/icons'
</script>

<template>
  <div class="layout-art" role="note"><ImageIcon class="size-4" aria-hidden="true" /><span>Artwork appears here</span></div>
</template>
