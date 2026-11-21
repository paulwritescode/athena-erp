// Jobs (stub): imports, exports and scheduled work. The real page lands in
// a later batch; until then this holds the nav slot with an empty state.
<script setup lang="ts">
import { ListChecks } from '@/icons'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Jobs" description="Imports, exports and scheduled work." />
    <EmptyState :icon="ListChecks" title="No jobs yet" body="Imports, exports and scheduled runs will appear here with their results. This page is on the way." />
  </div>
</template>
