<script setup lang="ts">
import { Button } from '@/components/ui/button'
const emit = defineEmits<{ openRecordTypes: [] }>()
</script>

<template>
  <section class="page-content"><div class="page-title-row"><div><h1>Packs</h1><p class="lead">Connect Record Types through relationships, then package them as installable Packs.</p></div><Button disabled>＋ New Pack</Button></div>
    <div class="notice-row info-notice">Packs are assembled from Record Types and explicit relationships. Creating relationship tables and Pack packages is the next implementation step.</div>
    <div class="empty-builder"><div class="card-icon">▦</div><h2>No Packs yet</h2><p>Once you have Record Types, compose a Pack by selecting them and defining how their tables relate.</p><Button variant="outline" @click="emit('openRecordTypes')">Open Record Types</Button></div>
  </section>
</template>
