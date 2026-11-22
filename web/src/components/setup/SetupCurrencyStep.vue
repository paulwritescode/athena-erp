// Setup wizard step 3: money (B11 slice 2). Saves the company's base
// money if it is not kept yet, then sets it on the company. Needs
// ledger:read to list, ledger:write to save; without either it explains
// and moves on, and readiness shows the base money missing.
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { toast } from 'vue-sonner'
import { ledgerClient } from '@/api'
import { classify, describeFailure } from '@/apiErrors'
import HelpHint from '@/copy/HelpHint.vue'
import ActionButton from '@/components/ActionButton.vue'
import LoadingSkeleton from '@/components/LoadingSkeleton.vue'
import NumberField from '@/components/fields/NumberField.vue'
import TextField from '@/components/fields/TextField.vue'
import SelectField from '@/components/fields/SelectField.vue'
import SetupFooter from './SetupFooter.vue'
import { checkCurrencyCode, checkExponent, checkLabel } from './setupModel'
import type { Company, Currency } from '@/gen/athena/v1/ledger_pb'

const props = defineProps<{
  companyName: string
  companyLabel: string
  canRead: boolean
  canWrite: boolean
}>()
const emit = defineEmits<{
  back: []
  done: [result: { baseCurrency: string } | null]
}>()

const loaded = ref(false)
const loadError = ref('')
const saving = ref(false)
const currencies = ref<Currency[]>([])
const company = ref<Company | null>(null)
const creating = ref(false)
const selected = ref('')
const code = ref('')
const label = ref('')
const exponent = ref('2')
const codeProblem = ref('')
const labelProblem = ref('')
const exponentProblem = ref('')
const saveProblem = ref('')

const active = computed(() => currencies.value.filter((currency) => !currency.disabled))
const options = computed(() => active.value.map((currency) => ({ value: currency.code, label: `${currency.code} — ${currency.label}` })))
const base = computed(() => company.value?.baseCurrency ?? '')

async function refresh() {
  loaded.value = false
  loadError.value = ''
  try {
    const [listed, companies] = await Promise.all([ledgerClient.listCurrencies({}), ledgerClient.listCompanies({})])
    currencies.value = listed.currencies
    company.value = companies.companies.find((entry) => entry.name === props.companyName) ?? null
    if (!active.value.some((currency) => currency.code === selected.value)) {
      selected.value = active.value[0]?.code ?? ''
    }
    creating.value = active.value.length === 0
  } catch (err) {
    loadError.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not load the money.'
  } finally {
    loaded.value = true
  }
}

onMounted(() => {
  if (props.canRead) void refresh()
  else loaded.value = true
})

const useExisting = computed(() => !creating.value && active.value.length > 0)

const primaryLabel = computed(() => {
  if (base.value) return 'Continue'
  if (!props.canRead || !props.canWrite) return 'Continue'
  if (useExisting.value) return selected.value ? `Use ${selected.value} & continue` : 'Continue'
  return code.value.trim() ? `Save ${code.value.trim().toUpperCase()} & continue` : 'Save money & continue'
})

const primaryDisabledReason = computed(() => {
  if (base.value || !props.canRead || !props.canWrite) return null
  if (useExisting.value && !selected.value) return 'Choose money first.'
  return null
})

async function submit() {
  if (base.value) {
    emit('done', { baseCurrency: base.value })
    return
  }
  if (!props.canRead || !props.canWrite) {
    emit('done', null)
    return
  }
  saveProblem.value = ''
  if (useExisting.value) {
    if (!selected.value) return
    await setBase(selected.value)
    return
  }
  codeProblem.value = checkCurrencyCode(code.value.trim().toUpperCase()) ?? ''
  labelProblem.value = checkLabel(label.value.trim()) ?? ''
  exponentProblem.value = checkExponent(exponent.value) ?? ''
  if (codeProblem.value || labelProblem.value || exponentProblem.value) return
  saving.value = true
  try {
    const saved = await ledgerClient.saveCurrency({
      currency: { code: code.value.trim().toUpperCase(), label: label.value.trim(), exponent: Number(exponent.value.trim()), disabled: false },
      expectedRevision: BigInt(0),
    })
    toast.success(`Saved ${saved.currency?.code ?? code.value.trim()}.`)
    await setBase(saved.currency?.code ?? code.value.trim().toUpperCase())
  } catch (err) {
    saveProblem.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not save the money.'
  } finally {
    saving.value = false
  }
}

async function setBase(code: string) {
  if (!company.value) {
    saveProblem.value = 'The company is gone. Go back and pick another.'
    return
  }
  saving.value = true
  try {
    const res = await ledgerClient.saveCompany({
      company: { name: company.value.name, label: company.value.label, baseCurrency: code, disabled: company.value.disabled },
      expectedRevision: company.value.revision,
    })
    toast.success(`${res.company?.label ?? company.value.label} now counts in ${code}.`)
    emit('done', { baseCurrency: code })
  } catch (err) {
    // A stale revision just needs a fresh read before trying again.
    try {
      const companies = await ledgerClient.listCompanies({})
      company.value = companies.companies.find((entry) => entry.name === props.companyName) ?? null
    } catch {
      // Keep the original error below when the refresh fails too.
    }
    saveProblem.value = err instanceof ConnectError ? describeFailure(classify(err)) : 'Could not set the base money.'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div>
    <h2>Choose the base money <HelpHint term="currency" /></h2>
    <p class="text-muted-foreground mt-1 text-sm">The money {{ companyLabel || 'the company' }} counts in. Save it here if it is not kept yet.</p>
    <LoadingSkeleton v-if="!loaded" class="mt-6" />
    <div v-else-if="!canRead" class="mt-6">
      <p class="text-sm">You need the books permission to see money. Ask your administrator.</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="emit('done', null)" />
    </div>
    <div v-else-if="loadError" class="mt-6">
      <p class="text-sm" role="alert">{{ loadError }}</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="emit('done', null)">
        <ActionButton kind="secondary" label="Try again" icon="refresh-ccw" @press="refresh" />
      </SetupFooter>
    </div>
    <div v-else-if="base" class="mt-6">
      <p class="text-sm">{{ base }} is the base money for {{ companyLabel || 'the company' }}.</p>
      <SetupFooter primary-label="Continue" @back="emit('back')" @primary="submit" />
    </div>
    <div v-else class="mt-6 space-y-4">
      <p v-if="!canWrite" class="muted-label">You need the books permission to save money.</p>
      <template v-if="useExisting">
        <SelectField
          id="setup-currency"
          label="Saved money"
          :options="options"
          :model-value="selected"
          :disabled="!canWrite"
          @update:model-value="selected = $event"
        />
        <div v-if="canWrite">
          <ActionButton kind="quiet" label="Save new money" icon="plus" @press="creating = true" />
        </div>
      </template>
      <template v-else-if="canWrite">
        <TextField
          id="setup-currency-code"
          label="Code"
          help="Three capital letters, like USD."
          placeholder="USD"
          autocomplete="off"
          :error="codeProblem || undefined"
          :model-value="code"
          @update:model-value="code = $event.toUpperCase(); codeProblem = ''"
        />
        <TextField
          id="setup-currency-name"
          label="Name"
          help="The full name people see."
          placeholder="US dollar"
          autocomplete="off"
          :error="labelProblem || undefined"
          :model-value="label"
          @update:model-value="label = $event; labelProblem = ''"
        />
        <NumberField
          id="setup-currency-exponent"
          label="Decimal places"
          help="Most money uses 2."
          placeholder="2"
          :error="exponentProblem || undefined"
          :model-value="exponent"
          @update:model-value="exponent = $event; exponentProblem = ''"
        />
        <div v-if="active.length > 0">
          <ActionButton kind="quiet" label="Use saved money" icon="undo-2" @press="creating = false" />
        </div>
      </template>
      <p v-if="saveProblem" class="text-sm text-destructive" role="alert">{{ saveProblem }}</p>
      <SetupFooter
        :primary-label="primaryLabel"
        :primary-icon="primaryLabel === 'Continue' ? 'chevron-right' : 'save'"
        :primary-busy="saving"
        :primary-disabled-reason="primaryDisabledReason"
        @back="emit('back')"
        @primary="submit"
      />
    </div>
  </div>
</template>
