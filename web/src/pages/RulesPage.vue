<script setup lang="ts">
const rules = [
  { label: 'Require a due date', event: 'Before save · Sales Invoice', condition: 'If payment terms are set', status: 'Draft' },
  { label: 'Escalate urgent tickets', event: 'After save · Support Ticket', condition: 'If priority is Urgent', status: 'Draft' },
]
</script>
<template>
  <section class="page-content"><div class="page-title-row"><div><div class="eyebrow">BUILD · BUSINESS LOGIC</div><h1>Rules</h1><p class="lead">Configure conditions and actions around your business records.</p></div><button class="button primary" disabled>＋ New Rule</button></div>
    <div class="notice-row info-notice">Go owns trusted business logic and invariants. Starlark is planned for sandboxed, configurable conditions and actions.</div>
    <section class="panel rule-panel"><div class="rule-table-head"><span>RULE</span><span>WHEN</span><span>CONDITION</span><span>STATUS</span></div><div v-for="rule in rules" :key="rule.label" class="rule-row"><span><strong>{{ rule.label }}</strong><small>Starlark rule · not executable yet</small></span><span>{{ rule.event }}</span><span>{{ rule.condition }}</span><span class="status-chip">{{ rule.status }}</span></div></section>
  </section>
</template>
