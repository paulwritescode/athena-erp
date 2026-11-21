// FormPreview renders a layout draft the way View would: the same sections,
// columns and blocks, every field empty and disabled. Visibility is not
// evaluated here — there is no record — so every section shows, and a caption
// says so. The builder keeps it live beside the editing controls.
<script setup lang="ts">
import { computed } from 'vue'
import { itemSpanClass, planSections, type FormLayout } from './layout'
import ArtPlaceholder from './ArtPlaceholder.vue'
import WorkspaceImage from '@/components/WorkspaceImage.vue'
import CheckboxField from '@/components/fields/CheckboxField.vue'
import DateField from '@/components/fields/DateField.vue'
import ImageField from '@/components/fields/ImageField.vue'
import LinkField from '@/components/fields/LinkField.vue'
import LongTextField from '@/components/fields/LongTextField.vue'
import NumberField from '@/components/fields/NumberField.vue'
import SelectField from '@/components/fields/SelectField.vue'
import TextField from '@/components/fields/TextField.vue'
import type { Field } from '../types'

const props = defineProps<{ recordTypeKey: string; fields: Field[]; layout: FormLayout | undefined }>()

const sections = computed(() => planSections(props.layout, props.fields))

function fieldLabel(field: Field): string {
  if (field.type === 'Money' && field.currency) return `${field.label} (${field.currency})`
  if (field.type === 'Quantity' && field.uom) return `${field.label} (${field.uom})`
  return field.label
}

function inputType(field: Field): string {
  return field.type === 'Email' ? 'email' : field.type === 'Phone' ? 'tel' : 'text'
}
</script>

<template>
  <div class="form-preview">
    <section v-for="(section, sectionIndex) in sections" :key="sectionIndex" class="layout-section" :aria-label="section.title || `Section ${sectionIndex + 1}`">
      <div v-if="section.title || section.description" class="layout-section-head">
        <h2 v-if="section.title">{{ section.title }}</h2>
        <p v-if="section.description" class="layout-section-description">{{ section.description }}</p>
      </div>
      <div class="layout-grid" :class="section.columns === 2 ? 'is-two' : 'is-one'">
        <template v-for="(item, itemIndex) in section.items" :key="itemIndex">
          <p v-if="item.kind === 'text'" class="layout-text is-wide">{{ item.text }}</p>
          <WorkspaceImage v-else-if="item.kind === 'image'" :path="item.path" alt="" class="is-wide layout-block-image" />
          <ArtPlaceholder v-else-if="item.kind === 'art'" class="is-wide" />
          <template v-else-if="item.kind === 'field'">
            <LinkField
              v-if="item.field.type === 'Link'"
              :id="`preview-${item.field.name}`"
              :label="fieldLabel(item.field)"
              :optional="!item.field.required"
              :help="item.helpText || undefined"
              model-value=""
              :record-type-key="item.field.target_record_type ?? ''"
              :target-label="item.field.target_record_type ?? 'linked record'"
              :class="itemSpanClass(section, item)"
              disabled
            />
            <CheckboxField
              v-else-if="item.field.type === 'Checkbox'"
              :id="`preview-${item.field.name}`"
              :label="fieldLabel(item.field)"
              :optional="!item.field.required"
              :help="item.helpText || undefined"
              :model-value="false"
              :class="itemSpanClass(section, item)"
              disabled
            />
            <SelectField
              v-else-if="item.field.type === 'Enum'"
              :id="`preview-${item.field.name}`"
              :label="fieldLabel(item.field)"
              :optional="!item.field.required"
              :help="item.helpText || undefined"
              model-value=""
              :options="item.field.values ?? []"
              :class="itemSpanClass(section, item)"
              disabled
            />
            <LongTextField
              v-else-if="item.field.type === 'Long text'"
              :id="`preview-${item.field.name}`"
              :label="fieldLabel(item.field)"
              :optional="!item.field.required"
              :help="item.helpText || undefined"
              model-value=""
              :class="itemSpanClass(section, item)"
              disabled
            />
            <DateField
              v-else-if="item.field.type === 'Date'"
              :id="`preview-${item.field.name}`"
              :label="fieldLabel(item.field)"
              :optional="!item.field.required"
              :help="item.helpText || undefined"
              model-value=""
              :class="itemSpanClass(section, item)"
              disabled
            />
            <NumberField
              v-else-if="['Number', 'Currency', 'Money', 'Quantity'].includes(item.field.type)"
              :id="`preview-${item.field.name}`"
              :label="fieldLabel(item.field)"
              :optional="!item.field.required"
              :help="item.helpText || undefined"
              model-value=""
              :class="itemSpanClass(section, item)"
              disabled
            />
            <ImageField
              v-else-if="item.field.type === 'Image'"
              :id="`preview-${item.field.name}`"
              :label="fieldLabel(item.field)"
              :optional="!item.field.required"
              :help="item.helpText || undefined"
              :model-value="null"
              :record-type-key="recordTypeKey"
              :field-name="item.field.name"
              :class="itemSpanClass(section, item)"
              disabled
            />
            <TextField
              v-else
              :id="`preview-${item.field.name}`"
              :label="fieldLabel(item.field)"
              :optional="!item.field.required"
              :help="item.helpText || undefined"
              model-value=""
              :type="inputType(item.field)"
              :class="itemSpanClass(section, item)"
              disabled
            />
          </template>
        </template>
      </div>
    </section>
    <p class="muted-label">Every section shows here. Sections with a visibility rule hide in View when the rule is false.</p>
  </div>
</template>
