// Reports (stub): saved lists and totals from Records. The real page lands
// in a later batch; until then this holds the nav slot with an empty state.
<script setup lang="ts">
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Reports" description="Lists and totals from your records." />
    <EmptyState art="empty-reports" title="No reports yet" body="Build a report to rerun a list or total any time. This page is on the way." />
  </div>
</template>
