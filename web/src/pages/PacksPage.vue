<script setup lang="ts">
import ActionButton from '@/components/ActionButton.vue'
const emit = defineEmits<{ openRecordTypes: [] }>()
</script>

<template>
  <section class="page-content"><div class="page-title-row"><div><h1>Packs</h1><p class="lead">Connect Record Types through relationships, then package them as installable Packs.</p></div><ActionButton kind="primary" label="New Pack" icon="plus" disabled-reason="Packs arrive in a later beta." /></div>
    <div class="notice-row info-notice">Packs are assembled from Record Types and explicit relationships. Creating relationship tables and Pack packages is the next implementation step.</div>
    <div class="empty-builder"><div class="card-icon">▦</div><h2>No Packs yet</h2><p>Once you have Record Types, compose a Pack by selecting them and defining how their tables relate.</p><ActionButton kind="secondary" label="Open Record Types" icon="layout-template" @press="emit('openRecordTypes')" /></div>
  </section>
</template>
