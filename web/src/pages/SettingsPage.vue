// Settings: Workspace preferences in stacked sections. Appearance, Security,
// Workspace identity and Background work are live; notification preferences
// arrive in a later batch.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PageHeader from '@/components/PageHeader.vue'
import FormSection from '@/components/FormSection.vue'
import EmptyState from '@/components/EmptyState.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import AppearanceSection from '@/components/settings/AppearanceSection.vue'
import IdentitySection from '@/components/settings/IdentitySection.vue'
import SecuritySection from '@/components/settings/SecuritySection.vue'
import ActionButton from '@/components/ActionButton.vue'
import { Bell, CircleAlert, CircleCheck } from '@/icons'
import { eventClient, jobClient } from '../api'
import type { Job, OutboxRow } from '../gen/athena/v1/events_pb'
import type { Page } from '@/types'

const props = defineProps<{ permissions: string[] }>()
const emit = defineEmits<{ navigate: [page: Page] }>()

// Background work: the recent failures of the durable queues, for builders.
// Secret settings stay out: retention and key custody are undecided (Z4).
const canViewWork = computed(() => props.permissions.includes('build'))
const deadJobs = ref<Job[]>([])
const deadRows = ref<OutboxRow[]>([])
const workLoading = ref(false)
const workError = ref('')

async function loadWork() {
  workLoading.value = true
  workError.value = ''
  try {
    const [jobs, rows] = await Promise.all([
      jobClient.listJobs({ status: 'dead', pageSize: 5 }),
      eventClient.listOutbox({ status: 'dead', pageSize: 5 }),
    ])
    deadJobs.value = jobs.jobs
    deadRows.value = rows.rows
  } catch {
    workError.value = 'Could not load the failed work.'
  } finally {
    workLoading.value = false
  }
}

onMounted(() => {
  if (canViewWork.value) void loadWork()
})
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Settings" description="Workspace preferences." />
    <FormSection title="Appearance" description="The Workspace accent, worn by buttons, the focus ring, the selected page and charts.">
      <AppearanceSection :can-edit="props.permissions.includes('settings:appearance')" />
    </FormSection>
    <FormSection title="Security" description="How long sign-ins last and how sign-in is protected.">
      <SecuritySection :permissions="props.permissions" />
    </FormSection>
    <FormSection title="Workspace identity" description="The name and logo people see.">
      <IdentitySection :can-edit="props.permissions.includes('settings:workspace')" />
    </FormSection>
    <FormSection
      v-if="canViewWork"
      title="Background work"
      description="Queued jobs and deliveries run themselves. Recent failures appear here; the Jobs page retries them."
    >
      <LoadingSkeleton v-if="workLoading" :rows="2" />
      <div v-else-if="workError" class="settings-field">
        <p class="files-hint">{{ workError }}</p>
        <ActionButton kind="secondary" label="Retry" @press="loadWork()" />
      </div>
      <div v-else-if="deadJobs.length === 0 && deadRows.length === 0" class="flex items-center gap-2">
        <CircleCheck class="text-muted-foreground size-4" aria-hidden="true" />
        <p class="muted-label">Everything ran. No failed jobs or deliveries.</p>
      </div>
      <div v-else class="space-y-3">
        <ul class="space-y-2">
          <li v-for="job in deadJobs" :key="job.id" class="flex items-start gap-2">
            <CircleAlert class="text-destructive mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <p><strong>{{ job.kind }}</strong> failed{{ job.lastError ? `: ${job.lastError}` : '.' }}</p>
          </li>
          <li v-for="row in deadRows" :key="row.id" class="flex items-start gap-2">
            <CircleAlert class="text-destructive mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <p><strong>{{ row.eventType }}</strong> stopped after {{ row.attempts }} {{ row.attempts === 1 ? 'try' : 'tries' }}{{ row.lastError ? `: ${row.lastError}` : '.' }}</p>
          </li>
        </ul>
        <ActionButton kind="secondary" label="Open Jobs" @press="emit('navigate', 'jobs')" />
      </div>
    </FormSection>
    <FormSection title="Notification preferences" description="Which notifications arrive and where.">
      <EmptyState :icon="Bell" title="Notification preferences are on the way" body="Per-kind notification switches will live here." />
    </FormSection>
  </div>
</template>
