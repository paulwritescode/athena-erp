// OfflinePage takes the whole desk while the browser reports no connection.
// The desk state underneath stays untouched, so coming back online returns
// to the same screen; "Try again" refreshes the desk once the connection is
// back, in case the server is down too.
<script setup lang="ts">
import ArtFigure from '@/art/ArtFigure.vue'
import ActionButton from '@/components/ActionButton.vue'

const emit = defineEmits<{ retry: [] }>()
</script>

<template>
  <main class="offline-page">
    <ArtFigure art-key="offline" />
    <h1>You are offline</h1>
    <p>Check your connection, then try again.</p>
    <ActionButton kind="primary" class="offline-retry" label="Try again" icon="refresh-ccw" @press="emit('retry')" />
  </main>
</template>
