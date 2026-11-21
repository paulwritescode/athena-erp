// Work queue (stub): the signed-in user's work across the workspace. The
// real page lands in a later batch; until then this holds the nav slot with
// an empty state.
<script setup lang="ts">
import { Inbox } from '@/icons'
import PageHeader from '@/components/PageHeader.vue'
import EmptyState from '@/components/EmptyState.vue'
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Work queue" description="Your work across the workspace, in one list." />
    <EmptyState :icon="Inbox" title="All caught up" body="Records assigned to you and Records waiting on your step will show up here. This page is on the way." />
  </div>
</template>
