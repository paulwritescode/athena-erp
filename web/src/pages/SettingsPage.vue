// Settings (stub): workspace preferences. The real page lands in a later
// batch; until then this holds the nav slot with an empty state.
<script setup lang="ts">
import { Settings } from '@/icons'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Settings" description="Workspace preferences." />
    <EmptyState :icon="Settings" title="Nothing to change yet" body="Workspace preferences will live here. This page is on the way." />
  </div>
</template>
