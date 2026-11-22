// One realistic Appearance preview card (§18): an invoices page drawn with
// the draft tokens — header with actions, a focused search input, three
// rows with status badges, a selected sidebar item and a six-month bar
// chart with a legend. Every element is something Athena actually shows.
<script setup lang="ts">
import { computed } from 'vue'
import { FileText } from '@/icons'
import { Badge } from '@/components/ui/badge'

const props = withDefaults(
  defineProps<{
    force: 'light' | 'dark' | 'auto'
    accent?: string
    charts?: string
    customVars?: Record<string, string>
    caption: string
  }>(),
  { accent: undefined, charts: undefined, customVars: undefined },
)

const rows = [
  { name: 'Invoice INV-1042', badge: 'Draft', variant: 'secondary' },
  { name: 'Invoice INV-1041', badge: 'Submitted', variant: 'outline' },
  { name: 'Invoice INV-1040', badge: 'Paid', variant: 'default' },
] as const

const months = [
  { short: 'Jan', height: '45%', color: 'var(--chart-1)' },
  { short: 'Feb', height: '70%', color: 'var(--chart-2)' },
  { short: 'Mar', height: '55%', color: 'var(--chart-3)' },
  { short: 'Apr', height: '85%', color: 'var(--chart-4)' },
  { short: 'May', height: '60%', color: 'var(--chart-5)' },
  { short: 'Jun', height: '75%', color: 'var(--chart-1)' },
]

const forceClass = computed(() => (props.force === 'light' ? 'preview-light' : props.force === 'dark' ? 'dark' : undefined))
</script>

<template>
  <figure
    role="img"
    :aria-label="`Preview of ${caption}`"
    :data-accent="accent"
    :data-charts="charts"
    :class="['rounded-[var(--radius)] border border-border bg-background p-4 text-foreground', forceClass]"
    :style="customVars"
  >
    <div class="flex flex-wrap items-center gap-2">
      <p class="mr-auto text-base font-semibold">Invoices</p>
      <span class="inline-flex h-8 items-center rounded-full bg-primary px-3 text-sm font-medium text-primary-foreground">New invoice</span>
      <span class="inline-flex h-8 items-center rounded-full bg-secondary px-3 text-sm font-medium text-secondary-foreground">Export</span>
    </div>
    <div class="mt-4 space-y-3">
      <input
        type="search"
        value=""
        placeholder="Search invoices…"
        readonly
        tabindex="-1"
        aria-hidden="true"
        class="h-9 w-full rounded-full border border-ring bg-background px-3 text-sm text-foreground ring-2 ring-ring/30 outline-none placeholder:text-muted-foreground"
      />
      <ul class="divide-y divide-border rounded-[var(--radius)] border border-border">
        <li v-for="row in rows" :key="row.name" class="flex items-center gap-2 px-3 py-2">
          <span class="min-w-0 flex-1 truncate text-sm">{{ row.name }}</span>
          <Badge :variant="row.variant">{{ row.badge }}</Badge>
        </li>
      </ul>
      <p class="flex h-9 items-center gap-2 rounded-xl bg-sidebar-primary px-3 text-sm font-medium text-sidebar-primary-foreground">
        <FileText class="size-4" aria-hidden="true" />
        Invoices
      </p>
      <div>
        <div class="flex h-24 items-end gap-2" aria-hidden="true">
          <div v-for="month in months" :key="month.short" class="flex h-full min-w-0 flex-1 flex-col items-stretch justify-end gap-1">
            <i class="block w-full rounded-sm" :style="{ height: month.height, background: month.color }" />
            <span class="block text-center text-[11px] text-muted-foreground">{{ month.short }}</span>
          </div>
        </div>
        <div class="mt-2 flex flex-wrap gap-x-3 gap-y-1" aria-hidden="true">
          <span v-for="month in months" :key="month.short" class="inline-flex items-center gap-1 text-[11px] text-muted-foreground">
            <i class="size-2 rounded-full" :style="{ background: month.color }" />
            {{ month.short }}
          </span>
        </div>
      </div>
    </div>
  </figure>
</template>
