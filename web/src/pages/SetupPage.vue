// Setup (stub): the guided path to a working workspace. The real page lands
// in a later batch; until then this holds the nav slot with an empty state.
<script setup lang="ts">
import { ClipboardList } from '@/icons'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Setup" description="Get the workspace ready." />
    <EmptyState :icon="ClipboardList" title="Setup is on its way" body="A guided checklist for companies, periods and openings will live here." />
  </div>
</template>
