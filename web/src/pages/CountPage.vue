// Count (stub): stock counts and quick tallies. The real page lands in a
// later batch; until then this holds the nav slot with an empty state.
<script setup lang="ts">
import { ClipboardCheck } from '@/icons'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Count" description="Stock counts and quick tallies." />
    <EmptyState :icon="ClipboardCheck" title="No counts yet" body="Start a count to compare what the system says with what is on the shelf. This page is on the way." />
  </div>
</template>
