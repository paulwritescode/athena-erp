// Setup: the guided path to a working workspace (B11 slice 2). Six steps —
// company, logo, money, period, owner, ready — with the readiness
// checklist last. Lands on the first incomplete step; completed steps stay
// revisitable. Steps own their saves; the host owns the step, the furthest
// reached, and the snapshot the checklist reads.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { authClient, ledgerClient, workspaceClient } from '@/api'
import AsciiLogo from '@/art/AsciiLogo.vue'
import { isRootUsername } from '@/rootUser'
import PageHeader from '@/components/PageHeader.vue'
import ErrorState from '@/components/ErrorState.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import PermissionDenied from '@/components/PermissionDenied.vue'
import SetupSteps from '@/components/setup/SetupSteps.vue'
import SetupCompanyStep from '@/components/setup/SetupCompanyStep.vue'
import SetupLogoStep from '@/components/setup/SetupLogoStep.vue'
import SetupCurrencyStep from '@/components/setup/SetupCurrencyStep.vue'
import SetupPeriodStep from '@/components/setup/SetupPeriodStep.vue'
import SetupInviteStep from '@/components/setup/SetupInviteStep.vue'
import SetupReadinessStep from '@/components/setup/SetupReadinessStep.vue'
import { coversToday, setupSteps, shortDateRange, type SetupSnapshot, type SetupStepId } from '@/components/setup/setupModel'
import type { Page } from '@/types'

const props = defineProps<{ permissions: string[] }>()
const emit = defineEmits<{
  navigate: [page: Page]
  goHome: []
}>()

const canLedgerRead = computed(() => props.permissions.includes('ledger:read'))
const canLedgerWrite = computed(() => props.permissions.includes('ledger:write'))
const canEditIdentity = computed(() => props.permissions.includes('settings:workspace'))
const canAccess = computed(() => props.permissions.includes('access:users'))
const canSeeSetup = computed(() => canLedgerRead.value || canEditIdentity.value || canAccess.value)

const stepIndex = ref(0)
const furthest = ref(0)
const direction = ref(1)
const step = computed(() => setupSteps[stepIndex.value]?.id ?? 'company')

const starting = ref(true)
const startError = ref(false)
const companyName = ref('')
const companyLabel = ref('')
const hasLogo = ref(false)
const baseCurrency = ref('')
const openPeriod = ref<{ label: string; dates: string } | null>(null)
const ownerNames = ref<string[]>([])
const skipped = ref(new Set<SetupStepId>())

const snapshot = computed<SetupSnapshot>(() => ({
  companyName: companyName.value,
  companyLabel: companyLabel.value,
  baseCurrency: baseCurrency.value,
  hasLogo: hasLogo.value,
  openPeriodLabel: openPeriod.value?.label ?? '',
  openPeriodDates: openPeriod.value?.dates ?? '',
  ownerNames: ownerNames.value,
}))

const done = computed(() => {
  const reached = new Set<SetupStepId>()
  if (companyName.value) reached.add('company')
  if (hasLogo.value) reached.add('logo')
  if (baseCurrency.value) reached.add('currency')
  if (openPeriod.value) reached.add('period')
  if (ownerNames.value.length > 0) reached.add('invite')
  if (reached.size === 5) reached.add('ready')
  return reached
})

function land() {
  const order: SetupStepId[] = ['company', 'logo', 'currency', 'period', 'invite', 'ready']
  for (let index = 0; index < order.length; index++) {
    if (!done.value.has(order[index]!)) {
      stepIndex.value = index
      furthest.value = index
      return
    }
  }
  stepIndex.value = 5
  furthest.value = 5
}

async function start() {
  starting.value = true
  startError.value = false
  try {
    const [companies, identity, users] = await Promise.all([
      canLedgerRead.value ? ledgerClient.listCompanies({}) : Promise.resolve(null),
      workspaceClient.getIdentity({}),
      canAccess.value ? authClient.listUsers({}) : Promise.resolve(null),
    ])
    const first = companies?.companies.find((company) => !company.disabled) ?? null
    if (first) {
      companyName.value = first.name
      companyLabel.value = first.label
      baseCurrency.value = first.baseCurrency
      if (canLedgerRead.value) {
        const today = new Date().toISOString().slice(0, 10)
        const listed = await ledgerClient.listPeriods({ company: first.name, book: 'main' })
        const open = listed.periods.find((period) => coversToday(period, today)) ?? null
        if (open) openPeriod.value = { label: open.label, dates: shortDateRange(open.startDate, open.endDate) }
      }
    }
    hasLogo.value = (identity.identity?.logoLightUrl ?? '') !== '' || (identity.identity?.logoDarkUrl ?? '') !== ''
    ownerNames.value = (users?.users ?? []).filter((user) => user.owner && !isRootUsername(user.username)).map((user) => user.username)
  } catch {
    startError.value = true
  } finally {
    starting.value = false
    if (!startError.value) land()
  }
}

onMounted(() => {
  if (canSeeSetup.value) void start()
})

function advance() {
  direction.value = 1
  stepIndex.value = Math.min(stepIndex.value + 1, setupSteps.length - 1)
  furthest.value = Math.max(furthest.value, stepIndex.value)
}

function back() {
  direction.value = -1
  stepIndex.value = Math.max(stepIndex.value - 1, 0)
}

function gotoIndex(index: number) {
  if (index < 0 || index > furthest.value) return
  direction.value = index > stepIndex.value ? 1 : -1
  stepIndex.value = index
}

function gotoStep(id: SetupStepId) {
  const index = setupSteps.findIndex((step) => step.id === id)
  if (index >= 0) {
    direction.value = -1
    stepIndex.value = index
  }
}

function onCompany(company: { name: string; label: string } | null) {
  if (company) {
    companyName.value = company.name
    companyLabel.value = company.label
  }
  advance()
}

function onLogo(result: { hasLogo: boolean; skipped: boolean }) {
  hasLogo.value = result.hasLogo
  if (result.skipped) skipped.value.add('logo')
  else skipped.value.delete('logo')
  advance()
}

function onCurrency(result: { baseCurrency: string } | null) {
  if (result) baseCurrency.value = result.baseCurrency
  advance()
}

function onPeriod(result: { label: string; dates: string } | null) {
  if (result) openPeriod.value = result
  advance()
}

function onInvite(result: { owners: string[]; skipped: boolean }) {
  ownerNames.value = result.owners
  if (result.skipped) skipped.value.add('invite')
  else skipped.value.delete('invite')
  advance()
}
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Setup" description="Get the workspace ready." />
    <div v-if="!canSeeSetup">
      <PermissionDenied page-title="Setup" @go-home="emit('goHome')" />
    </div>
    <div v-else class="mx-auto w-full max-w-3xl">
      <LoadingSkeleton v-if="starting" />
      <ErrorState v-else-if="startError" title="Setup did not load" message="Check your connection and try again." @retry="start" />
      <template v-else>
        <SetupSteps :current="step" :done="done" :furthest="furthest" @goto="gotoIndex" />
        <Transition :name="direction > 0 ? 'setup-fwd' : 'setup-back'" mode="out-in">
          <div :key="step" class="mt-6">
            <SetupCompanyStep
              v-if="step === 'company'"
              :initial-name="companyName"
              :can-read="canLedgerRead"
              :can-write="canLedgerWrite"
              :show-back="stepIndex > 0"
              @back="back"
              @done="onCompany"
            />
            <SetupLogoStep v-else-if="step === 'logo'" :can-write="canEditIdentity" @back="back" @done="onLogo" @navigate="emit('navigate', $event)" />
            <SetupCurrencyStep
              v-else-if="step === 'currency'"
              :company-name="companyName"
              :company-label="companyLabel"
              :can-read="canLedgerRead"
              :can-write="canLedgerWrite"
              @back="back"
              @done="onCurrency"
            />
            <SetupPeriodStep
              v-else-if="step === 'period'"
              :company-name="companyName"
              :company-label="companyLabel"
              :can-read="canLedgerRead"
              :can-write="canLedgerWrite"
              @back="back"
              @done="onPeriod"
            />
            <SetupInviteStep v-else-if="step === 'invite'" :can-access="canAccess" @back="back" @done="onInvite" />
            <SetupReadinessStep
              v-else
              :snapshot="snapshot"
              :skipped="skipped"
              :can-read-ledger="canLedgerRead"
              :can-edit-identity="canEditIdentity"
              @back="back"
              @goto="gotoStep"
              @navigate="emit('navigate', $event)"
            />
          </div>
        </Transition>
        <div v-if="step === 'ready'" class="mt-12">
          <AsciiLogo art-key="first-run-logo" />
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.setup-fwd-enter-active,
.setup-fwd-leave-active,
.setup-back-enter-active,
.setup-back-leave-active {
  transition:
    opacity var(--duration-base) var(--ease-out),
    transform var(--duration-base) var(--ease-out);
}
.setup-fwd-enter-from,
.setup-back-leave-to {
  opacity: 0;
  transform: translateX(12px);
}
.setup-back-enter-from,
.setup-fwd-leave-to {
  opacity: 0;
  transform: translateX(-12px);
}
@media (prefers-reduced-motion: reduce) {
  .setup-fwd-enter-active,
  .setup-fwd-leave-active,
  .setup-back-enter-active,
  .setup-back-leave-active {
    transition: none;
  }
  .setup-fwd-enter-from,
  .setup-back-leave-to,
  .setup-back-enter-from,
  .setup-fwd-leave-to {
    transform: none;
  }
}
</style>
