<script setup lang="ts">
import ActionButton from '@/components/ActionButton.vue'
import EmptyState from '@/components/EmptyState.vue'
const emit = defineEmits<{ openRecordTypes: [] }>()
</script>

<template>
  <section class="page-content"><div class="page-title-row"><div><h1>Packs</h1><p class="lead">Connect Record Types through relationships, then package them as installable Packs.</p></div><ActionButton kind="primary" label="New Pack" icon="plus" disabled-reason="Packs arrive in a later beta." /></div>
    <div class="notice-row info-notice">Packs are assembled from Record Types and explicit relationships. Creating relationship tables and Pack packages is the next implementation step.</div>
    <EmptyState
      art="empty-packs"
      title="No Packs yet"
      body="Once you have Record Types, compose a Pack by selecting them and defining how their tables relate."
    >
      <template #action>
        <ActionButton kind="secondary" label="Open Record Types" icon="layout-template" @press="emit('openRecordTypes')" />
      </template>
    </EmptyState>
  </section>
</template>
