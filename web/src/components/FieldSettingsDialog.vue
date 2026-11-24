<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Field, RecordType } from '../types'
import ActionButton from '@/components/ActionButton.vue'
import DetailSheet from '@/components/DetailSheet.vue'
import { Checkbox } from '@/components/ui/checkbox'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ChevronDown } from '@/icons'
import { gloss } from '@/copy/glossary'
import { validateExpressionText } from '../fieldRules'
import {
  changeType,
  childTypesOf,
  composeTotal,
  fieldTypes,
  isTotalTarget,
  totalFunctions,
  totalSourceFields,
  updateFieldLabel,
} from './fieldSettings'

// Edits one field in place in a right sheet (a bottom drawer on phones);
// the Record Types page owns the field and saves it. The server enforces
// every attribute and rejects bad combinations on save.
const props = defineProps<{ field: Field | null; lifecycle: boolean; recordTypes: RecordType[]; currentTypeName: string }>()
const emit = defineEmits<{ close: [] }>()

const type = computed(() => props.field?.type ?? '')
const indexable = computed(() => type.value !== 'Long text' && type.value !== 'Checkbox' && type.value !== 'Image')
const bounded = computed(() => ['Number', 'Currency', 'Money', 'Quantity', 'Date'].includes(type.value))
const patterned = computed(() => ['Text', 'Long text', 'Phone'].includes(type.value))
const lengthLimited = computed(() => ['Text', 'Long text', 'Email', 'Phone'].includes(type.value))

const siblings = computed(() => props.recordTypes.find((item) => item.name === props.currentTypeName)?.fields ?? [])

// Basics open; Rules and Advanced start collapsed.
const basicsOpen = ref(true)
const rulesOpen = ref(false)
const advancedOpen = ref(false)

function setLabel(value: string | number) {
  if (!props.field) return
  updateFieldLabel(props.field, value, siblings.value)
}

function setType(value: unknown) {
  if (!props.field || typeof value !== 'string' || !fieldTypes.includes(value)) return
  props.field.type = value
  changeType(props.field)
  syncChoiceText()
}

function setRequired(value: boolean | 'indeterminate') {
  if (!props.field) return
  props.field.required = value === true
}

// Link targets exclude the edited type; a target that no longer lists stays
// selectable under its key so saving can report it.
const linkTargets = computed(() => {
  const targets = props.recordTypes
    .filter((item) => item.name !== props.currentTypeName)
    .map((item) => ({ name: item.name, label: item.label }))
  const current = props.field?.target_record_type
  if (current && !targets.some((item) => item.name === current)) targets.unshift({ name: current, label: current })
  return targets
})

function setTarget(value: unknown) {
  if (!props.field || typeof value !== 'string') return
  if (value === '') delete props.field.target_record_type
  else props.field.target_record_type = value
}

const permissionOptions = Array.from({ length: 10 }, (_, level) => level)

function setPermission(value: unknown) {
  if (!props.field || typeof value !== 'string') return
  const level = Number(value)
  if (!Number.isInteger(level) || level < 0 || level > 9) return
  if (level === 0) delete props.field.permission_level
  else props.field.permission_level = level
}

// Totals fill the formula from a total over child lines. The picks reset to
// the first valid choice whenever the options change (a type switch can
// empty them); the preview always composes from resolved picks.
const totalFn = ref('sum')
const totalChild = ref('')
const totalField = ref('')
const totalChildren = computed(() => childTypesOf(props.recordTypes, props.currentTypeName).filter((child) => totalSourceFields(child.fields, type.value).length > 0))
const showTotals = computed(() => isTotalTarget(type.value) && totalChildren.value.length > 0)
// Picks that no longer list snap back to the first valid choice.
function resetTotalPicks() {
  if (!totalFunctions.some((item) => item.value === totalFn.value)) totalFn.value = 'sum'
  if (!totalChildren.value.some((item) => item.name === totalChild.value)) totalChild.value = totalChildren.value[0]?.name ?? ''
  const fields = totalSourceFields(totalChildren.value.find((item) => item.name === totalChild.value)?.fields ?? [], type.value)
  if (!fields.some((item) => item.name === totalField.value)) totalField.value = fields[0]?.name ?? ''
}
watch([type, totalChildren], resetTotalPicks, { immediate: true })
const totalChildFields = computed(() => totalSourceFields(totalChildren.value.find((item) => item.name === totalChild.value)?.fields ?? [], type.value))
const totalPreview = computed(() => {
  if (!showTotals.value || totalChild.value === '' || totalField.value === '') return ''
  return composeTotal(totalFn.value, totalChild.value, totalField.value)
})

function useTotal() {
  if (totalPreview.value) setExpression('formula', totalPreview.value)
}

// One choice per line as "value: Label"; the text is kept while typing.
const choiceText = ref('')
function syncChoiceText() {
  choiceText.value = (props.field?.values ?? []).map((choice) => `${choice.value}: ${choice.label}`).join('\n')
}
// The opener keeps this component mounted while it swaps fields, so a new
// field resets the sections and the totals picks.
watch(() => props.field, () => {
  syncChoiceText()
  basicsOpen.value = true
  rulesOpen.value = false
  advancedOpen.value = false
  totalFn.value = 'sum'
  totalChild.value = ''
  totalField.value = ''
  resetTotalPicks()
}, { immediate: true })

function editChoices(text: string) {
  if (!props.field) return
  choiceText.value = text
  const values = text.split('\n').map((line) => line.trim()).filter(Boolean).map((line) => {
    const [value = '', ...rest] = line.split(':')
    const label = rest.join(':').trim()
    return { value: value.trim(), label: label || value.trim() }
  })
  props.field.values = values
}

// Text inputs store an absent attribute as undefined so the field saves as it would without it.
function setText(name: 'default' | 'min' | 'max' | 'pattern' | 'currency' | 'uom', value: string | number) {
  if (!props.field) return
  const text = String(value)
  if (text === '') delete props.field[name]
  else props.field[name] = name === 'currency' ? text.toUpperCase() : text
}

const expressionNames = ['formula', 'required_if', 'read_only_if', 'validate', 'depends_on'] as const
type ExpressionName = (typeof expressionNames)[number]
// An expression is stored as written; the server parses and type-checks it when the Record Type is saved.
function setExpression(name: ExpressionName, value: string | number) {
  if (!props.field) return
  const text = String(value)
  if (text.trim() === '') delete props.field[name]
  else props.field[name] = text
}
function expressionProblem(name: ExpressionName): string {
  return validateExpressionText(props.field?.[name] ?? '')
}
function setNumber(name: 'max_length' | 'scale' | 'permission_level', value: string | number) {
  if (!props.field) return
  const text = String(value).trim()
  if (text === '') delete props.field[name]
  else props.field[name] = Number(text)
}

function setFlag(name: 'unique' | 'index' | 'read_only' | 'set_once' | 'no_copy' | 'allow_on_submit', value: boolean | 'indeterminate') {
  if (!props.field) return
  if (value === true) props.field[name] = true
  else delete props.field[name]
}

function onOpenChange(open: boolean) {
  if (!open) emit('close')
}
</script>

<template>
  <DetailSheet v-if="field" :open="field !== null" :title="`${field.label || 'Field'} settings`" description="The server checks these on every save, so they hold for the API and the command line too." @update:open="onOpenChange">
    <div class="grid gap-6">
      <Collapsible :open="basicsOpen" @update:open="basicsOpen = $event">
        <CollapsibleTrigger as-child>
          <button type="button" class="flex w-full items-center justify-between gap-2 py-1 text-left" :aria-expanded="basicsOpen ? 'true' : 'false'">
            <h2>Basics</h2>
            <ChevronDown :class="['size-4 shrink-0 transition-transform', basicsOpen ? undefined : '-rotate-90']" aria-hidden="true" />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent class="pt-3">
          <div class="settings-grid">
            <div class="settings-field"><Label for="fs-label">Label</Label><Input id="fs-label" :model-value="field.label" placeholder="Customer name" @update:model-value="setLabel" /><p class="settings-note">The name people see on the form.</p></div>
            <div class="settings-field">
              <Label for="fs-type">Type</Label>
              <Select :model-value="field.type" @update:model-value="setType">
                <SelectTrigger id="fs-type"><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem v-for="option in fieldTypes" :key="option" :value="option">{{ option }}</SelectItem></SelectContent>
              </Select>
              <p class="settings-note">What kind of value the field holds. Changing it clears the settings below that no longer apply.</p>
            </div>
            <div class="settings-flags col-span-full">
              <label><Checkbox :model-value="field.required" @update:model-value="setRequired($event)" />Required <small>People must fill it in before the record saves.</small></label>
            </div>
            <div v-if="type !== 'Link' && type !== 'Image'" class="settings-field"><Label for="fs-default">Default value</Label><Input id="fs-default" :model-value="field.default ?? ''" :placeholder="type === 'Checkbox' ? 'true or false' : ''" @update:model-value="setText('default', $event)" /><p class="settings-note">Filled in for new records; people can change it.</p></div>
            <div v-if="type === 'Link'" class="settings-field">
              <Label for="fs-target">{{ gloss('linkTarget').label }}</Label>
              <Select :model-value="field.target_record_type ?? ''" @update:model-value="setTarget">
                <SelectTrigger id="fs-target"><SelectValue placeholder="Select a Record Type" /></SelectTrigger>
                <SelectContent><SelectItem v-for="target in linkTargets" :key="target.name" :value="target.name">{{ target.label }}</SelectItem></SelectContent>
              </Select>
              <p class="settings-note">{{ gloss('linkTarget').explanation }}</p>
            </div>
            <template v-if="type === 'Money'">
              <div class="settings-field"><Label for="fs-currency">Currency code</Label><Input id="fs-currency" :model-value="field.currency ?? ''" maxlength="3" placeholder="USD" @update:model-value="setText('currency', $event)" /><p class="settings-note">Three capital letters, like USD.</p></div>
              <div class="settings-field"><Label for="fs-scale">Decimal places</Label><Input id="fs-scale" type="number" min="0" max="9" :model-value="field.scale ?? ''" @update:model-value="setNumber('scale', $event)" /><p class="settings-note">Digits after the point.</p></div>
              <p class="settings-note">Stored as whole minor units, so 12.30 USD is 1230. Changing the currency or places clears stored values.</p>
            </template>
            <template v-if="type === 'Quantity'">
              <div class="settings-field"><Label for="fs-uom">Unit of measure</Label><Input id="fs-uom" :model-value="field.uom ?? ''" placeholder="kg" @update:model-value="setText('uom', $event)" /><p class="settings-note">A short key, like kg.</p></div>
              <div class="settings-field"><Label for="fs-qscale">Decimal places</Label><Input id="fs-qscale" type="number" min="0" max="6" :model-value="field.scale ?? ''" placeholder="6" @update:model-value="setNumber('scale', $event)" /><p class="settings-note">At most 6.</p></div>
            </template>
            <div v-if="type === 'Enum'" class="settings-field is-wide">
              <Label for="fs-choices">{{ gloss('fieldChoices').label }}</Label>
              <textarea id="fs-choices" class="settings-textarea" rows="5" :value="choiceText" placeholder="open: Open&#10;paid: Paid" spellcheck="false" autocomplete="off" autocapitalize="off" @input="editChoices(($event.target as HTMLTextAreaElement).value)" />
              <p class="settings-note">One per line as value: Label.</p>
              <p class="settings-note">{{ gloss('fieldChoices').explanation }} Removing a value clears it from stored records.</p>
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>

      <Collapsible :open="rulesOpen" @update:open="rulesOpen = $event">
        <CollapsibleTrigger as-child>
          <button type="button" class="flex w-full items-center justify-between gap-2 py-1 text-left" :aria-expanded="rulesOpen ? 'true' : 'false'">
            <h2>Rules for this field</h2>
            <ChevronDown :class="['size-4 shrink-0 transition-transform', rulesOpen ? undefined : '-rotate-90']" aria-hidden="true" />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent class="pt-3">
          <div class="settings-grid">
            <p class="settings-note">Rules calculate this field or say when it is required, read-only or shown. They run on the server when the record saves.</p>
            <div v-if="showTotals" class="settings-field is-wide">
              <p class="text-sm font-medium">Totals from lines</p>
              <p class="settings-note">Fill the formula from a total over the lines below.</p>
              <div class="settings-grid">
                <div class="settings-field">
                  <Label for="fs-total-fn">What</Label>
                  <Select v-model="totalFn">
                    <SelectTrigger id="fs-total-fn"><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem v-for="fn in totalFunctions" :key="fn.value" :value="fn.value">{{ fn.label }}</SelectItem></SelectContent>
                  </Select>
                </div>
                <div class="settings-field">
                  <Label for="fs-total-child">Lines</Label>
                  <Select v-model="totalChild">
                    <SelectTrigger id="fs-total-child"><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem v-for="child in totalChildren" :key="child.name" :value="child.name">{{ child.label }}</SelectItem></SelectContent>
                  </Select>
                </div>
                <div class="settings-field">
                  <Label for="fs-total-field">Field</Label>
                  <Select v-model="totalField">
                    <SelectTrigger id="fs-total-field"><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem v-for="source in totalChildFields" :key="source.name" :value="source.name">{{ source.label }}</SelectItem></SelectContent>
                  </Select>
                </div>
              </div>
              <p class="settings-note font-mono">{{ totalPreview }}</p>
              <div><ActionButton kind="secondary" density="row" label="Use this total" icon="plus" @press="useTotal" /></div>
            </div>
            <div v-if="type !== 'Link' && type !== 'Image'" class="settings-field is-wide"><Label for="fs-formula">{{ gloss('fieldFormula').label }}</Label><Input id="fs-formula" :model-value="field.formula ?? ''" class="font-mono" placeholder="doc.qty * doc.rate" spellcheck="false" autocomplete="off" autocapitalize="off" :aria-invalid="expressionProblem('formula') ? 'true' : undefined" @update:model-value="setExpression('formula', $event)" /><p v-if="expressionProblem('formula')" class="field-error">{{ expressionProblem('formula') }}</p><p v-else class="settings-note">{{ gloss('fieldFormula').explanation }} It cannot use the current date or time.</p></div>
            <div class="settings-field is-wide"><Label for="fs-required-if">Required when</Label><Input id="fs-required-if" :model-value="field.required_if ?? ''" class="font-mono" placeholder="doc.is_credit" spellcheck="false" autocomplete="off" autocapitalize="off" :aria-invalid="expressionProblem('required_if') ? 'true' : undefined" @update:model-value="setExpression('required_if', $event)" /><p v-if="expressionProblem('required_if')" class="field-error">{{ expressionProblem('required_if') }}</p><p v-else class="settings-note">Must be filled in while this is true.</p></div>
            <div class="settings-field is-wide"><Label for="fs-read-only-if">Read only when</Label><Input id="fs-read-only-if" :model-value="field.read_only_if ?? ''" class="font-mono" placeholder="doc.paid" spellcheck="false" autocomplete="off" autocapitalize="off" :aria-invalid="expressionProblem('read_only_if') ? 'true' : undefined" @update:model-value="setExpression('read_only_if', $event)" /><p v-if="expressionProblem('read_only_if')" class="field-error">{{ expressionProblem('read_only_if') }}</p><p v-else class="settings-note">Cannot be changed while this is true.</p></div>
            <div class="settings-field is-wide"><Label for="fs-validate">Must be true</Label><Input id="fs-validate" :model-value="field.validate ?? ''" class="font-mono" placeholder="doc.rate >= dec(0)" spellcheck="false" autocomplete="off" autocapitalize="off" :aria-invalid="expressionProblem('validate') ? 'true' : undefined" @update:model-value="setExpression('validate', $event)" /><p v-if="expressionProblem('validate')" class="field-error">{{ expressionProblem('validate') }}</p><p v-else class="settings-note">Saving stops unless this is true.</p></div>
            <div class="settings-field is-wide"><Label for="fs-depends-on">Show when</Label><Input id="fs-depends-on" :model-value="field.depends_on ?? ''" class="font-mono" placeholder="doc.is_stock_item" spellcheck="false" autocomplete="off" autocapitalize="off" :aria-invalid="expressionProblem('depends_on') ? 'true' : undefined" @update:model-value="setExpression('depends_on', $event)" /><p v-if="expressionProblem('depends_on')" class="field-error">{{ expressionProblem('depends_on') }}</p><p v-else class="settings-note">The desk hides the field while this is false.</p></div>
          </div>
        </CollapsibleContent>
      </Collapsible>

      <Collapsible :open="advancedOpen" @update:open="advancedOpen = $event">
        <CollapsibleTrigger as-child>
          <button type="button" class="flex w-full items-center justify-between gap-2 py-1 text-left" :aria-expanded="advancedOpen ? 'true' : 'false'">
            <h2>Advanced</h2>
            <ChevronDown :class="['size-4 shrink-0 transition-transform', advancedOpen ? undefined : '-rotate-90']" aria-hidden="true" />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent class="pt-3">
          <div class="settings-grid">
            <template v-if="bounded">
              <div class="settings-field"><Label for="fs-min">Minimum</Label><Input id="fs-min" :model-value="field.min ?? ''" :placeholder="type === 'Date' ? 'YYYY-MM-DD' : ''" @update:model-value="setText('min', $event)" /><p class="settings-note">Empty means no limit.</p></div>
              <div class="settings-field"><Label for="fs-max">Maximum</Label><Input id="fs-max" :model-value="field.max ?? ''" :placeholder="type === 'Date' ? 'YYYY-MM-DD' : ''" @update:model-value="setText('max', $event)" /><p class="settings-note">Empty means no limit.</p></div>
            </template>
            <div v-if="lengthLimited" class="settings-field"><Label for="fs-length">Maximum length</Label><Input id="fs-length" type="number" min="1" :model-value="field.max_length ?? ''" @update:model-value="setNumber('max_length', $event)" /><p class="settings-note">Empty means the type's own limit.</p></div>
            <div v-if="patterned" class="settings-field"><Label for="fs-pattern">{{ gloss('fieldPattern').label }}</Label><Input id="fs-pattern" :model-value="field.pattern ?? ''" class="font-mono" placeholder="[A-Z]{3}-[0-9]+" spellcheck="false" autocomplete="off" autocapitalize="off" @update:model-value="setText('pattern', $event)" /><p class="settings-note">{{ gloss('fieldPattern').explanation }}</p></div>
            <div class="settings-field">
              <Label for="fs-level">{{ gloss('permissionLevel').label }}</Label>
              <Select :model-value="String(field.permission_level ?? 0)" @update:model-value="setPermission">
                <SelectTrigger id="fs-level"><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem v-for="level in permissionOptions" :key="level" :value="String(level)">Level {{ level }}</SelectItem></SelectContent>
              </Select>
              <p class="settings-note">{{ gloss('permissionLevel').explanation }}</p>
            </div>
          </div>
          <div class="settings-flags">
            <label v-if="indexable"><Checkbox :model-value="field.unique ?? false" @update:model-value="setFlag('unique', $event)" />{{ gloss('unique').label }} <small>{{ gloss('unique').explanation }}</small></label>
            <label v-if="indexable"><Checkbox :model-value="field.index ?? false" @update:model-value="setFlag('index', $event)" />{{ gloss('indexed').label }} <small>{{ gloss('indexed').explanation }}</small></label>
            <label><Checkbox :model-value="field.read_only ?? false" @update:model-value="setFlag('read_only', $event)" />{{ gloss('readOnly').label }} <small>{{ gloss('readOnly').explanation }}</small></label>
            <label><Checkbox :model-value="field.set_once ?? false" @update:model-value="setFlag('set_once', $event)" />{{ gloss('setOnce').label }} <small>{{ gloss('setOnce').explanation }}</small></label>
            <label><Checkbox :model-value="field.no_copy ?? false" @update:model-value="setFlag('no_copy', $event)" />{{ gloss('noCopy').label }} <small>{{ gloss('noCopy').explanation }}</small></label>
            <label v-if="lifecycle"><Checkbox :model-value="field.allow_on_submit ?? false" @update:model-value="setFlag('allow_on_submit', $event)" />{{ gloss('allowOnSubmit').label }} <small>{{ gloss('allowOnSubmit').explanation }}</small></label>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
    <template #footer><ActionButton kind="primary" label="Done" icon="check" @press="emit('close')" /></template>
  </DetailSheet>
</template>
