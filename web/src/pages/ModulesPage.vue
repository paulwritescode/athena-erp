<script setup lang="ts">
import { Button } from '@/components/ui/button'
const emit = defineEmits<{ openRecords: [] }>()
</script>
<template>
  <section class="page-content"><div class="page-title-row"><div><div class="eyebrow">BUILD · COMPOSED CAPABILITIES</div><h1>Modules</h1><p class="lead">Connect Record definitions through relationships, then package them as installable capabilities.</p></div><Button disabled>＋ New Module</Button></div>
    <div class="notice-row info-notice">Modules are assembled from Records and explicit relationships. Creating relationship tables and module packages is the next implementation step.</div>
    <div class="empty-builder"><div class="card-icon">▦</div><h2>No Modules yet</h2><p>Once you have Records, compose a Module by selecting them and defining how their tables relate.</p><Button variant="outline" @click="emit('openRecords')">Open Records</Button></div>
  </section>
</template>
