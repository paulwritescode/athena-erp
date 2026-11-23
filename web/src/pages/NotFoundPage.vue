// NotFoundPage answers an address the desk cannot parse: one quiet figure,
// a plain sentence, and the way home. It shows inside the shell, after
// sign-in; signed out, the sign-in page comes first.
<script setup lang="ts">
import ArtFigure from '@/art/ArtFigure.vue'
import AsciiLogo from '@/art/AsciiLogo.vue'
import ActionButton from '@/components/ActionButton.vue'

const emit = defineEmits<{ goHome: [] }>()
</script>

<template>
  <div class="flex flex-col items-center px-6 py-12 text-center">
    <ArtFigure art-key="not-found" />
    <h1 class="mt-8">This page does not exist</h1>
    <p class="text-muted-foreground mt-1 max-w-sm text-base">The address is mistyped, or the page was moved.</p>
    <div class="mt-4">
      <ActionButton kind="primary" label="Back to home" icon="arrow-left" @press="emit('goHome')" />
    </div>
    <div class="mt-12">
      <AsciiLogo art-key="not-found-logo" />
    </div>
  </div>
</template>
