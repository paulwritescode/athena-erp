// Setup wizard step 1: company (B11 slice 2). Continues with an existing
// company or saves a new one. Needs ledger:read to list, ledger:write to
// save; without either it explains and lets the wizard move on, and the
// readiness list shows the company missing.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { ledgerClient } from '@/api'
import { classify, describeFailure } from '@/apiErrors'
import HelpHint from '@/copy/HelpHint.vue'
import ActionButton from '@/components/ActionButton.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import TextField from '@/components/fields/TextField.vue'
import SelectField from '@/components/fields/SelectField.vue'
import SetupFooter from './SetupFooter.vue'
import { checkLabel, checkLedgerName } from './setupModel'
import type { Company } from '@/gen/athena/v1/ledger_pb'

const props = withDefaults(
  defineProps<{
    initialName: string
    canRead: boolean
    canWrite: boolean
    showBack?: boolean
  }>(),
  { showBack: true },
)
const emit = defineEmits<{
  back: []
  done: [company: { name: string; label: string } | null]
}>()

const loaded = ref(false)
const loadError = ref('')
const companies = ref<Company[]>([])
const creating = ref(false)
const saving = ref(false)
const selected = ref(props.initialName)
const code = ref('')
const label = ref('')
const codeProblem = ref('')
const labelProblem = ref('')
const saveProblem = ref('')

const active = computed(() => companies.value.filter((company) => !company.disabled))
const options = computed(() => active.value.map((company) => ({ value: company.name, label: `${company.label} (${company.name})` })))
const selectedCompany = computed(() => active.value.find((company) => company.name === selected.value) ?? null)

async function refresh() {
  loaded.value = false
  loadError.value = ''
  try {
    const res = await ledgerClient.listCompanies({})
    companies.value = res.companies
    if (!active.value.some((company) => company.name === selected.value)) {
      selected.value = active.value[0]?.name ?? ''
    }
    creating.value = active.value.length === 0
  } catch (err) {
    loadError.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not load the companies.'
  } finally {
    loaded.value = true
  }
}

onMounted(() => {
  if (props.canRead) void refresh()
  else loaded.value = true
})

function startCreating() {
  creating.value = true
  saveProblem.value = ''
}

function stopCreating() {
  creating.value = false
  code.value = ''
  label.value = ''
  codeProblem.value = ''
  labelProblem.value = ''
  saveProblem.value = ''
}

const primaryLabel = computed(() => {
  if (!props.canRead || !props.canWrite) return 'Continue'
  if (creating.value || active.value.length === 0) return 'Save company & continue'
  return selectedCompany.value ? `Continue with ${selectedCompany.value.label}` : 'Continue'
})

const primaryDisabledReason = computed(() => {
  if (!props.canRead) return null
  if (creating.value || active.value.length === 0) {
    if (!props.canWrite) return 'You need the books permission to save a company.'
    return null
  }
  if (!selectedCompany.value) return 'Choose a company first.'
  return null
})

async function submit() {
  if (!props.canRead) {
    emit('done', null)
    return
  }
  if (!creating.value && active.value.length > 0) {
    if (!selectedCompany.value) return
    emit('done', { name: selectedCompany.value.name, label: selectedCompany.value.label })
    return
  }
  saveProblem.value = ''
  codeProblem.value = checkLedgerName('Company', code.value.trim()) ?? ''
  labelProblem.value = checkLabel(label.value.trim()) ?? ''
  if (codeProblem.value || labelProblem.value) return
  saving.value = true
  try {
    const res = await ledgerClient.saveCompany({
      company: { name: code.value.trim(), label: label.value.trim(), baseCurrency: '', disabled: false },
      expectedRevision: BigInt(0),
    })
    toast.success(`Saved company ${res.company?.label ?? code.value.trim()}.`)
    emit('done', { name: res.company?.name ?? code.value.trim(), label: res.company?.label ?? label.value.trim() })
  } catch (err) {
    saveProblem.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not save the company.'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <h2>Which company is this setup for? <HelpHint term="company" /></h2>
    <p class="text-muted-foreground mt-1 text-sm">The money, periods and checks below all belong to one company.</p>
    <LoadingSkeleton v-if="!loaded" class="mt-6" />
    <div v-else-if="!canRead" class="mt-6">
      <p class="text-sm">You need the books permission to see companies. Ask your administrator.</p>
      <SetupFooter :show-back="showBack" primary-label="Continue" @back="emit('back')" @primary="submit" />
    </div>
    <div v-else-if="loadError" class="mt-6">
      <p class="text-sm" role="alert">{{ loadError }}</p>
      <SetupFooter :show-back="showBack" primary-label="Continue" @back="emit('back')" @primary="emit('done', null)">
        <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refresh" />
      </SetupFooter>
    </div>
    <div v-else class="mt-6 space-y-4">
      <template v-if="!creating && active.length > 0">
        <SelectField
          id="setup-company"
          label="Company"
          :options="options"
          :model-value="selected"
          @update:model-value="selected = $event"
        />
        <div v-if="canWrite">
          <ActionButton kind="quiet" label="Create a new company" icon="plus" @press="startCreating" />
        </div>
        <p v-else class="muted-label">You need the books permission to create a company.</p>
      </template>
      <template v-else>
        <TextField
          id="setup-company-code"
          label="Company code"
          help="Short and lowercase, like acme. It never changes."
          placeholder="acme"
          autocomplete="off"
          :error="codeProblem || undefined"
          :model-value="code"
          @update:model-value="code = $event; codeProblem = ''"
        />
        <TextField
          id="setup-company-name"
          label="Company name"
          help="The full name people see."
          placeholder="Acme Trading"
          autocomplete="off"
          :error="labelProblem || undefined"
          :model-value="label"
          @update:model-value="label = $event; labelProblem = ''"
        />
        <div v-if="active.length > 0">
          <ActionButton kind="quiet" label="Use an existing company" icon="undo-2" @press="stopCreating" />
        </div>
      </template>
      <p v-if="saveProblem" class="text-sm text-destructive" role="alert">{{ saveProblem }}</p>
      <SetupFooter
        :show-back="showBack"
        :primary-label="primaryLabel"
        :primary-icon="creating || active.length === 0 ? 'save' : 'chevron-right'"
        :primary-busy="saving"
        :primary-disabled-reason="primaryDisabledReason"
        @back="emit('back')"
        @primary="submit"
      />
    </div>
  </div>
</template>
