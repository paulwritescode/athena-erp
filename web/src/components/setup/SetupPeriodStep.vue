// Setup wizard step 4: period (B11 slice 2). Opens the first working
// month on the main book; its financial year opens with it. Needs
// ledger:read to list, ledger:write to save; without either it explains
// and moves on, and readiness shows the period missing.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { ledgerClient } from '@/api'
import { classify, describeFailure } from '@/apiErrors'
import HelpHint from '@/copy/HelpHint.vue'
import ActionButton from '@/components/ActionButton.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import DateField from '@/components/fields/DateField.vue'
import SetupFooter from './SetupFooter.vue'
import {
  checkDate,
  checkDateOrder,
  coversToday,
  currentMonthBounds,
  firstPeriodLabel,
  firstPeriodName,
  fiscalYearName,
  shortDateRange,
  yearBounds,
} from './setupModel'
import type { FiscalYear, Period } from '@/gen/athena/v1/ledger_pb'

const props = defineProps<{
  companyName: string
  companyLabel: string
  canRead: boolean
  canWrite: boolean
}>()
const emit = defineEmits<{
  back: []
  done: [result: { label: string; dates: string } | null]
}>()

const loaded = ref(false)
const loadError = ref('')
const saving = ref(false)
const periods = ref<Period[]>([])
const years = ref<FiscalYear[]>([])
const bounds = currentMonthBounds()
const start = ref(bounds.start)
const end = ref(bounds.end)
const startProblem = ref('')
const endProblem = ref('')
const saveProblem = ref('')

// The server counts today in UTC; match it so the same period shows open.
const today = new Date().toISOString().slice(0, 10)

const openNow = computed(() => periods.value.find((period) => coversToday(period, today)) ?? null)
const openDates = computed(() => (openNow.value ? shortDateRange(openNow.value.startDate, openNow.value.endDate) : ''))
const derivedYear = computed(() => fiscalYearName(start.value || bounds.start))
const derivedYearBounds = computed(() => yearBounds(start.value || bounds.start))
const derivedPeriod = computed(() => firstPeriodName(start.value || bounds.start))
const derivedLabel = computed(() => firstPeriodLabel(start.value || bounds.start))

async function refresh() {
  loaded.value = false
  loadError.value = ''
  try {
    const [listed, fiscal] = await Promise.all([
      ledgerClient.listPeriods({ company: props.companyName, book: 'main' }),
      ledgerClient.listFiscalYears({ company: props.companyName }),
    ])
    periods.value = listed.periods
    years.value = fiscal.fiscalYears
  } catch (err) {
    loadError.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not load the periods.'
  } finally {
    loaded.value = true
  }
}

onMounted(() => {
  if (props.canRead) void refresh()
  else loaded.value = true
})

async function submit() {
  if (openNow.value) {
    emit('done', { label: openNow.value.label, dates: openDates.value })
    return
  }
  if (!props.canRead || !props.canWrite) {
    emit('done', null)
    return
  }
  saveProblem.value = ''
  startProblem.value = checkDate('Start', start.value) ?? ''
  endProblem.value = checkDate('End', end.value) ?? ''
  if (!startProblem.value && !endProblem.value) {
    const order = checkDateOrder(start.value, end.value)
    if (order) endProblem.value = order
  }
  if (startProblem.value || endProblem.value) return
  saving.value = true
  try {
    const yearName = fiscalYearName(start.value)
    const yearDates = yearBounds(start.value)
    const existing = years.value.find((year) => year.name === yearName)
    if (!existing) {
      await ledgerClient.saveFiscalYear({
        fiscalYear: { name: yearName, label: yearName, company: props.companyName, startDate: yearDates.start, endDate: yearDates.end },
        expectedRevision: BigInt(0),
      })
    }
    const saved = await ledgerClient.savePeriod({
      period: {
        company: props.companyName,
        book: 'main',
        name: firstPeriodName(start.value),
        label: firstPeriodLabel(start.value),
        fiscalYear: yearName,
        startDate: start.value,
        endDate: end.value,
      },
      expectedRevision: BigInt(0),
    })
    const label = saved.period?.label ?? derivedLabel.value
    toast.success(`Opened period ${label}.`)
    emit('done', { label, dates: shortDateRange(start.value, end.value) })
  } catch (err) {
    saveProblem.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not open the period.'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <h2>Open the first period <HelpHint term="period" /></h2>
    <p class="text-muted-foreground mt-1 text-sm">The first working month for {{ companyLabel || 'the company' }}. Its financial year opens with it.</p>
    <LoadingSkeleton v-if="!loaded" class="mt-6" />
    <div v-else-if="!canRead" class="mt-6">
      <p class="text-sm">You need the books permission to see periods. Ask your administrator.</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="emit('done', null)" />
    </div>
    <div v-else-if="loadError" class="mt-6">
      <p class="text-sm" role="alert">{{ loadError }}</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="emit('done', null)">
        <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refresh" />
      </SetupFooter>
    </div>
    <div v-else-if="openNow" class="mt-6">
      <p class="text-sm">{{ openNow.label }} is open ({{ openDates }}).</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="submit" />
    </div>
    <div v-else class="mt-6 space-y-4">
      <p v-if="!canWrite" class="muted-label">You need the books permission to open a period.</p>
      <div class="grid gap-4 sm:grid-cols-2">
        <DateField
          id="setup-period-start"
          label="Start"
          :error="startProblem || undefined"
          :model-value="start"
          :disabled="!canWrite"
          @update:model-value="start = $event; startProblem = ''"
        />
        <DateField
          id="setup-period-end"
          label="End"
          :error="endProblem || undefined"
          :model-value="end"
          :disabled="!canWrite"
          @update:model-value="end = $event; endProblem = ''"
        />
      </div>
      <p v-if="canWrite" class="muted-label">
        Opens {{ derivedPeriod }} · {{ derivedLabel }}, with financial year {{ derivedYear }} ({{ derivedYearBounds.start }} – {{ derivedYearBounds.end }}).
      </p>
      <p v-if="saveProblem" class="text-sm text-destructive" role="alert">{{ saveProblem }}</p>
      <SetupFooter
        primary-label="Open first period & continue"
        primary-icon="calendar-plus"
        :primary-busy="saving"
        :primary-disabled-reason="!canWrite ? 'You need the books permission to open a period.' : null"
        @back="emit('back')"
        @primary="submit"
      />
    </div>
  </div>
</template>
