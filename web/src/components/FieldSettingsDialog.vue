<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { EnumChoice, Field } from '../types'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { validateExpressionText } from '../fieldRules'

// Edits the attributes of one field in place; the Record Types page owns the
// field and saves it. The server enforces every attribute and rejects bad
// combinations on save.
const props = defineProps<{ field: Field | null; lifecycle: boolean }>()
const emit = defineEmits<{ close: [] }>()

const type = computed(() => props.field?.type ?? '')
const indexable = computed(() => type.value !== 'Long text' && type.value !== 'Checkbox')
const bounded = computed(() => ['Number', 'Currency', 'Money', 'Quantity', 'Date'].includes(type.value))
const patterned = computed(() => ['Text', 'Long text', 'Phone'].includes(type.value))
const lengthLimited = computed(() => ['Text', 'Long text', 'Email', 'Phone'].includes(type.value))

// One choice per line as "value: Label"; the text is kept while typing.
const choiceText = ref('')
watch(() => props.field, (field) => {
  choiceText.value = (field?.values ?? []).map((choice) => `${choice.value}: ${choice.label}`).join('\n')
}, { immediate: true })

function editChoices(text: string) {
  if (!props.field) return
  choiceText.value = text
  const values: EnumChoice[] = text.split('\n').map((line) => line.trim()).filter(Boolean).map((line) => {
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
  <Dialog :open="field !== null" @update:open="onOpenChange">
    <DialogContent v-if="field" class="field-settings">
      <DialogHeader>
        <DialogTitle>{{ field.label || 'Field' }} settings</DialogTitle>
        <DialogDescription>The server checks these on every save, so they hold for the API and the command line too.</DialogDescription>
      </DialogHeader>
      <div class="settings-grid">
        <template v-if="type === 'Money'">
          <div class="settings-field"><Label for="fs-currency">Currency code</Label><Input id="fs-currency" :model-value="field.currency ?? ''" maxlength="3" placeholder="USD" @update:model-value="setText('currency', $event)" /></div>
          <div class="settings-field"><Label for="fs-scale">Decimal places</Label><Input id="fs-scale" type="number" min="0" max="9" :model-value="field.scale ?? ''" @update:model-value="setNumber('scale', $event)" /></div>
          <p class="settings-note">Stored as whole minor units, so 12.30 USD is 1230. Changing the currency or places clears stored values.</p>
        </template>
        <template v-if="type === 'Quantity'">
          <div class="settings-field"><Label for="fs-uom">Unit of measure</Label><Input id="fs-uom" :model-value="field.uom ?? ''" placeholder="kg" @update:model-value="setText('uom', $event)" /></div>
          <div class="settings-field"><Label for="fs-qscale">Decimal places (at most 6)</Label><Input id="fs-qscale" type="number" min="0" max="6" :model-value="field.scale ?? ''" placeholder="6" @update:model-value="setNumber('scale', $event)" /></div>
        </template>
        <div v-if="type === 'Enum'" class="settings-field is-wide">
          <Label for="fs-choices">Choices, one per line as value: Label</Label>
          <textarea id="fs-choices" class="settings-textarea" rows="5" :value="choiceText" placeholder="open: Open&#10;paid: Paid" @input="editChoices(($event.target as HTMLTextAreaElement).value)" />
          <p class="settings-note">A value is a permanent lowercase identifier. Removing a value clears it from stored records.</p>
        </div>
        <div v-if="type !== 'Link'" class="settings-field"><Label for="fs-default">Default value</Label><Input id="fs-default" :model-value="field.default ?? ''" :placeholder="type === 'Checkbox' ? 'true or false' : ''" @update:model-value="setText('default', $event)" /></div>
        <template v-if="bounded">
          <div class="settings-field"><Label for="fs-min">Minimum</Label><Input id="fs-min" :model-value="field.min ?? ''" :placeholder="type === 'Date' ? 'YYYY-MM-DD' : ''" @update:model-value="setText('min', $event)" /></div>
          <div class="settings-field"><Label for="fs-max">Maximum</Label><Input id="fs-max" :model-value="field.max ?? ''" :placeholder="type === 'Date' ? 'YYYY-MM-DD' : ''" @update:model-value="setText('max', $event)" /></div>
        </template>
        <div v-if="lengthLimited" class="settings-field"><Label for="fs-length">Maximum length</Label><Input id="fs-length" type="number" min="1" :model-value="field.max_length ?? ''" @update:model-value="setNumber('max_length', $event)" /></div>
        <div class="settings-field"><Label for="fs-level">Permission level (0 to 9)</Label><Input id="fs-level" type="number" min="0" max="9" :model-value="field.permission_level ?? ''" placeholder="0" @update:model-value="setNumber('permission_level', $event)" /></div>
        <p class="settings-note">Only people whose Grants reach this level see or change the field. Its old values are masked in the history for everyone else.</p>
        <div v-if="patterned" class="settings-field"><Label for="fs-pattern">Pattern (regular expression)</Label><Input id="fs-pattern" :model-value="field.pattern ?? ''" placeholder="[A-Z]{3}-[0-9]+" @update:model-value="setText('pattern', $event)" /></div>
      </div>
      <div class="settings-grid">
        <p class="settings-note is-wide">Expressions read fields as <code>doc.field_key</code> and a child Record Type's lines as <code>doc.line.field_key</code> in <code>sum</code>, <code>min</code>, <code>max</code> and <code>count</code>. Write decimals as <code>dec("1.5")</code>.</p>
        <div v-if="type !== 'Link'" class="settings-field is-wide"><Label for="fs-formula">Formula</Label><Input id="fs-formula" :model-value="field.formula ?? ''" placeholder="doc.qty * doc.rate" :aria-invalid="expressionProblem('formula') ? 'true' : undefined" @update:model-value="setExpression('formula', $event)" /><p v-if="expressionProblem('formula')" class="field-error">{{ expressionProblem('formula') }}</p></div>
        <div class="settings-field is-wide"><Label for="fs-required-if">Required when</Label><Input id="fs-required-if" :model-value="field.required_if ?? ''" placeholder="doc.is_credit" :aria-invalid="expressionProblem('required_if') ? 'true' : undefined" @update:model-value="setExpression('required_if', $event)" /><p v-if="expressionProblem('required_if')" class="field-error">{{ expressionProblem('required_if') }}</p></div>
        <div class="settings-field is-wide"><Label for="fs-read-only-if">Read only when</Label><Input id="fs-read-only-if" :model-value="field.read_only_if ?? ''" placeholder="doc.paid" :aria-invalid="expressionProblem('read_only_if') ? 'true' : undefined" @update:model-value="setExpression('read_only_if', $event)" /><p v-if="expressionProblem('read_only_if')" class="field-error">{{ expressionProblem('read_only_if') }}</p></div>
        <div class="settings-field is-wide"><Label for="fs-validate">Must be true</Label><Input id="fs-validate" :model-value="field.validate ?? ''" placeholder="doc.rate >= dec(0)" :aria-invalid="expressionProblem('validate') ? 'true' : undefined" @update:model-value="setExpression('validate', $event)" /><p v-if="expressionProblem('validate')" class="field-error">{{ expressionProblem('validate') }}</p></div>
        <div class="settings-field is-wide"><Label for="fs-depends-on">Show when</Label><Input id="fs-depends-on" :model-value="field.depends_on ?? ''" placeholder="doc.is_stock_item" :aria-invalid="expressionProblem('depends_on') ? 'true' : undefined" @update:model-value="setExpression('depends_on', $event)" /><p v-if="expressionProblem('depends_on')" class="field-error">{{ expressionProblem('depends_on') }}</p></div>
        <p class="settings-note is-wide">A formula is calculated by the server on every save, so people cannot edit it. It cannot use <code>today()</code>. Show when only hides the field in the desk.</p>
      </div>
      <div class="settings-flags">
        <label v-if="indexable"><Checkbox :model-value="field.unique ?? false" @update:model-value="setFlag('unique', $event)" />Unique <small>No two records share a value; a deleted master keeps its value.</small></label>
        <label v-if="indexable"><Checkbox :model-value="field.index ?? false" @update:model-value="setFlag('index', $event)" />Indexed <small>Faster lookups.</small></label>
        <label><Checkbox :model-value="field.read_only ?? false" @update:model-value="setFlag('read_only', $event)" />Read only <small>Only the default sets it.</small></label>
        <label><Checkbox :model-value="field.set_once ?? false" @update:model-value="setFlag('set_once', $event)" />Set once <small>Cannot change after it has a value.</small></label>
        <label><Checkbox :model-value="field.no_copy ?? false" @update:model-value="setFlag('no_copy', $event)" />Do not copy <small>Left empty when a document is amended.</small></label>
        <label v-if="lifecycle"><Checkbox :model-value="field.allow_on_submit ?? false" @update:model-value="setFlag('allow_on_submit', $event)" />Allow on submit <small>Still editable after the document is submitted. Every change is audited.</small></label>
      </div>
      <DialogFooter><Button @click="emit('close')">Done</Button></DialogFooter>
    </DialogContent>
  </Dialog>
</template>
