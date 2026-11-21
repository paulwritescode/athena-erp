// Settings → Security: the athena security values. Any signed-in user with
// access:sessions or settings:security reads them; editing needs
// settings:security, and saving retries through step-up (the api interceptor
// asks for the password when the server demands it). Bounds come from the
// server and are checked before saving. The section stays a placeholder on
// servers whose client predates the RPC.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { authClient } from '../../api'
import { classify, describeFailure } from '../../apiErrors'
import ActionButton from '@/components/ActionButton.vue'
import EmptyState from '@/components/EmptyState.vue'
import NumberField from '@/components/fields/NumberField.vue'
import { Fingerprint } from '@/icons'
import type { SecuritySetting } from '../../gen/athena/v1/auth_pb'

const props = defineProps<{ permissions: string[] }>()

const LABELS: Record<string, string> = {
  'security.session.idle_minutes': 'Idle sign-out',
  'security.session.build_idle_minutes': 'Builder idle sign-out',
  'security.session.absolute_hours': 'Absolute sign-out',
  'security.rate.ip_per_minute': 'Sign-ins per minute per address',
  'security.rate.username_per_15_minutes': 'Sign-ins per 15 minutes per username',
  'security.rate.writes_per_minute': 'Changes per minute per session',
  'security.rate.reports_per_minute': 'Report checks per minute per session',
  'security.rate.exports_per_minute': 'Exports per minute per session',
}

function labelFor(key: string): string {
  const known = LABELS[key]
  if (known) return known
  const last = key.split('.').pop() ?? key
  return last.split('_').map((part) => part.charAt(0).toUpperCase() + part.slice(1)).join(' ')
}

function supported(): boolean {
  const methods = authClient as unknown as Record<string, unknown>
  return typeof methods.getSecuritySettings === 'function' && typeof methods.setSecuritySettings === 'function'
}

const canEdit = computed(() => props.permissions.includes('settings:security'))
const loaded = ref(false)
const loadError = ref('')
const denied = ref(false)
const saving = ref(false)
const settings = ref<SecuritySetting[]>([])
const drafts = ref<Record<string, string>>({})

async function refresh() {
  loaded.value = false
  loadError.value = ''
  denied.value = false
  try {
    const res = await authClient.getSecuritySettings({})
    settings.value = [...res.settings].sort((a, b) => a.key.localeCompare(b.key))
    const next: Record<string, string> = {}
    for (const setting of settings.value) next[setting.key] = String(setting.value)
    drafts.value = next
  } catch (err) {
    if (err instanceof ConnectError) {
      const failure = classify(err)
      if (failure.kind === 'denied') {
        denied.value = true
        return
      }
      loadError.value = describeFailure(failure)
    } else {
      loadError.value = 'Could not load the security settings.'
    }
  } finally {
    loaded.value = true
  }
}

onMounted(() => {
  if (supported()) void refresh()
  else loaded.value = true
})

function problemFor(setting: SecuritySetting): string | null {
  const raw = (drafts.value[setting.key] ?? '').trim()
  if (!/^-?\d+$/.test(raw)) return 'Enter a whole number.'
  const value = Number(raw)
  if (value < setting.minValue || value > setting.maxValue) return `Between ${setting.minValue} and ${setting.maxValue}.`
  return null
}

const problems = computed(() => {
  const out: Record<string, string> = {}
  for (const setting of settings.value) {
    const problem = problemFor(setting)
    if (problem) out[setting.key] = problem
  }
  return out
})

const changed = computed(() => {
  const out: Record<string, number> = {}
  for (const setting of settings.value) {
    const raw = (drafts.value[setting.key] ?? '').trim()
    if (/^-?\d+$/.test(raw) && Number(raw) !== setting.value) out[setting.key] = Number(raw)
  }
  return out
})

async function save() {
  const updates = changed.value
  if (Object.keys(problems.value).length > 0 || Object.keys(updates).length === 0) return
  saving.value = true
  try {
    const res = await authClient.setSecuritySettings({ settings: updates })
    settings.value = [...res.settings].sort((a, b) => a.key.localeCompare(b.key))
    const next: Record<string, string> = {}
    for (const setting of settings.value) next[setting.key] = String(setting.value)
    drafts.value = next
    toast.success('Security settings saved.')
  } catch (err) {
    toast.error(err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not save the security settings.')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <EmptyState
    v-if="!supported()"
    :icon="Fingerprint"
    title="Security settings need a newer server"
    body="This Athena server does not offer them yet. Ask the person who runs it to update, then come back."
  />
  <div v-else-if="!loaded" class="settings-loading" aria-label="Loading security settings">
    <div class="h-9 w-full animate-pulse rounded-3xl bg-muted" />
    <div class="mt-3 h-9 w-full animate-pulse rounded-3xl bg-muted" />
  </div>
  <div v-else-if="denied" class="empty-state">
    <p>You need the access or security permission to see these values.</p>
  </div>
  <div v-else-if="loadError" class="empty-state">
    <p>{{ loadError }}</p>
    <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refresh" />
  </div>
  <div v-else class="space-y-4">
    <p v-if="!canEdit" class="muted-label">You need the security permission to change these values.</p>
    <div class="grid gap-x-4 gap-y-5 min-[640px]:grid-cols-2">
      <NumberField
        v-for="setting in settings"
        :key="setting.key"
        :id="`security-${setting.key}`"
        :label="labelFor(setting.key)"
        :help="`${setting.description} Between ${setting.minValue} and ${setting.maxValue} ${setting.unit}; default ${setting.defaultValue}.`"
        :model-value="drafts[setting.key] ?? ''"
        :error="problems[setting.key]"
        :disabled="!canEdit"
        @update:model-value="drafts[setting.key] = $event"
      />
    </div>
    <div v-if="canEdit" class="flex flex-wrap gap-2 pt-2">
      <ActionButton
        kind="primary"
        label="Save security settings"
        icon="save"
        :busy="saving"
        :disabled-reason="Object.keys(problems).length > 0 ? 'Fix the highlighted values.' : Object.keys(changed).length === 0 ? 'No changes to save.' : null"
        @press="save"
      />
    </div>
  </div>
</template>
