<script setup lang="ts">
const emit = defineEmits<{ openForms: [] }>()
const modules = [
  { label: 'Core', description: 'Shared records and platform features', forms: 1, status: 'Installed' },
  { label: 'Invoicing', description: 'Invoices, payments, and accounting workflows', forms: 1, status: 'Draft' },
  { label: 'Support', description: 'Customer support and ticket tracking', forms: 1, status: 'Draft' },
]
</script>
<template>
  <section class="page-content"><div class="page-title-row"><div><div class="eyebrow">BUILD · BUSINESS CAPABILITIES</div><h1>Modules</h1><p class="lead">Bundle related records and workflows as reusable business features.</p></div><button class="button primary" disabled>＋ New Module</button></div>
    <div class="notice-row info-notice">Modules are installable packages. This starter shows their structure; package installation and versioning are not implemented yet.</div>
    <div class="module-grid"><article v-for="mod in modules" :key="mod.label" class="module-card"><div class="module-card-top"><span class="list-icon">▦</span><span class="status-chip" :class="{ installed: mod.status === 'Installed' }">{{ mod.status }}</span></div><h2>{{ mod.label }}</h2><p>{{ mod.description }}</p><div class="module-stats"><span>{{ mod.forms }} Form{{ mod.forms === 1 ? '' : 's' }}</span><span>0 Rules</span><span>0 Reports</span></div><button class="text-button" @click="emit('openForms')">View included Forms →</button></article></div>
  </section>
</template>
