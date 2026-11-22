// Setup wizard step 6: readiness (B11 slice 2). The wizard checklist with
// fix actions back to each step, the server books checks the wizard does
// not cover itself, and the B5 identity section for review.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { ledgerClient } from '@/api'
import { classify, describeFailure } from '@/apiErrors'
import ActionButton from '@/components/ActionButton.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import IdentitySection from '@/components/settings/IdentitySection.vue'
import SetupFooter from './SetupFooter.vue'
import { CircleCheck, Minus, TriangleAlert } from '@/icons'
import {
  buildLedgerChecks,
  buildWizardChecks,
  firstMissingStep,
  setupStepWords,
  type ServerCheck,
  type SetupSnapshot,
  type SetupStepId,
} from './setupModel'
import type { Page } from '@/types'

const props = defineProps<{
  snapshot: SetupSnapshot
  skipped: ReadonlySet<SetupStepId>
  canReadLedger: boolean
  canEditIdentity: boolean
}>()
const emit = defineEmits<{
  back: []
  goto: [step: SetupStepId]
  navigate: [page: Page]
}>()

const booksLoaded = ref(false)
const booksError = ref('')
const booksChecks = ref<ServerCheck[]>([])

const wizardChecks = computed(() => buildWizardChecks(props.snapshot, props.skipped))
const missingStep = computed(() => firstMissingStep(wizardChecks.value))
const missingBooks = computed(() => booksChecks.value.filter((check) => !check.ok))
const complete = computed(() => !missingStep.value && booksLoaded.value && !booksError.value && missingBooks.value.length === 0)

async function refreshBooks() {
  booksLoaded.value = false
  booksError.value = ''
  try {
    const res = await ledgerClient.getReadiness({ company: props.snapshot.companyName })
    booksChecks.value = buildLedgerChecks(res.checks)
  } catch (err) {
    booksError.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not load the books checks.'
  } finally {
    booksLoaded.value = true
  }
}

onMounted(() => {
  if (props.canReadLedger && props.snapshot.companyName) void refreshBooks()
  else booksLoaded.value = true
})
</script>

<template>
  <div>
    <h2>Is the workspace ready?</h2>
    <p class="text-muted-foreground mt-1 text-sm">Everything the desk needs before real work starts.</p>
    <div class="mt-6 space-y-8">
      <section aria-label="Setup checklist">
        <h3>Setup checklist</h3>
        <ul class="mt-3 space-y-1">
          <li v-for="check in wizardChecks" :key="check.id" class="flex flex-wrap items-center gap-x-3 gap-y-1 py-2">
            <CircleCheck v-if="check.state === 'ready'" class="size-5 shrink-0 text-success" aria-hidden="true" />
            <TriangleAlert v-else-if="check.state === 'missing'" class="size-5 shrink-0 text-warning" aria-hidden="true" />
            <Minus v-else class="size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <div class="min-w-0 flex-1">
              <p class="text-sm font-medium">
                {{ check.word }}
                <span class="text-muted-foreground font-normal">
                  {{ check.state === 'ready' ? '· ready' : check.state === 'missing' ? '· missing' : '· skipped' }}
                </span>
              </p>
              <p class="text-muted-foreground text-sm">{{ check.detail }}</p>
            </div>
            <ActionButton
              v-if="check.step && check.state !== 'ready'"
              kind="quiet"
              density="row"
              :label="check.state === 'missing' ? `Go to ${setupStepWords[check.step]}` : 'Review'"
              icon="chevron-right"
              @press="check.step ? emit('goto', check.step) : undefined"
            />
          </li>
        </ul>
      </section>
      <section aria-label="Books checks">
        <h3>Books checks</h3>
        <LoadingSkeleton v-if="!booksLoaded" :rows="2" class="mt-3" />
        <p v-else-if="!canReadLedger" class="text-muted-foreground mt-3 text-sm">Books checks need the books permission.</p>
        <p v-else-if="!snapshot.companyName" class="text-muted-foreground mt-3 text-sm">Save a company first.</p>
        <div v-else-if="booksError" class="mt-3">
          <p class="text-sm" role="alert">{{ booksError }}</p>
          <div class="mt-2">
            <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refreshBooks" />
          </div>
        </div>
        <div v-else class="mt-3">
          <ul class="space-y-1">
            <li v-for="check in booksChecks" :key="check.name" class="flex flex-wrap items-center gap-x-3 gap-y-1 py-2">
              <CircleCheck v-if="check.ok" class="size-5 shrink-0 text-success" aria-hidden="true" />
              <TriangleAlert v-else class="size-5 shrink-0 text-warning" aria-hidden="true" />
              <div class="min-w-0 flex-1">
                <p class="text-sm font-medium">
                  {{ check.name }} <span class="text-muted-foreground font-normal">· {{ check.ok ? 'ready' : 'missing' }}</span>
                </p>
                <p v-if="check.detail" class="text-muted-foreground font-mono text-xs">{{ check.detail }}</p>
              </div>
            </li>
          </ul>
          <div v-if="missingBooks.length > 0" class="mt-2">
            <ActionButton kind="quiet" label="Open Ledger setup" icon="book-open" @press="emit('navigate', 'ledger')" />
          </div>
        </div>
      </section>
      <section aria-label="Workspace identity">
        <h3>Workspace identity</h3>
        <div class="mt-3">
          <IdentitySection :can-edit="canEditIdentity" />
        </div>
      </section>
      <p v-if="complete" class="flex items-center gap-2 text-sm">
        <CircleCheck class="size-5 shrink-0 text-success" aria-hidden="true" />
        Everything is ready. Open the overview to start working.
      </p>
      <SetupFooter
        :show-back="true"
        :primary-label="missingStep ? 'Continue setup' : 'Open overview'"
        :primary-icon="missingStep ? 'chevron-right' : 'house'"
        @back="emit('back')"
        @primary="missingStep ? emit('goto', missingStep) : emit('navigate', 'overview')"
      />
    </div>
  </div>
</template>
