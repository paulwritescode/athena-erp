<script setup lang="ts">
import { Button } from '@/components/ui/button'
</script>
<template>
  <section class="page-content"><div class="page-title-row"><div><h1>Rules</h1><p class="lead">Configure conditions and bounded actions over Record fields.</p></div><Button disabled>＋ New Rule</Button></div>
    <div class="notice-row info-notice">Go code owns trusted lifecycle behavior, controls, permissions, and invariants. Starlark conditions will reference named Records, fields, and Module relationships through a restricted API.</div>
    <div class="empty-builder"><div class="card-icon">⌘</div><h2>No Rules yet</h2><p>Starlark execution, event selection, field-aware rule editing, validation, and sandbox limits are not implemented.</p></div>
  </section>
</template>
import { Button } from '@/components/ui/button'
