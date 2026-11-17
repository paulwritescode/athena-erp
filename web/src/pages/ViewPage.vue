<script setup lang="ts">
import type { FormDefinition } from '../types'
defineProps<{ forms: FormDefinition[]; currentForm: FormDefinition; notice: string }>()
const emit = defineEmits<{ select: [name: string]; back: []; save: [] }>()
</script>
<template>
  <section class="page-content view-page"><div class="view-head"><div><div class="eyebrow">VIEW · FORM EXPERIENCE</div><h1>{{ currentForm.label }}</h1><p class="lead">This is the record entry experience your team will use.</p></div><div class="preview-label">FORM PREVIEW <span>· record saving is not implemented</span></div></div>
    <div class="view-layout"><aside class="view-menu"><strong>WORKSPACE</strong><button v-for="form in forms" :key="form.name" :class="{ selected: currentForm.name === form.name }" @click="emit('select', form.name)"><span class="list-icon">▤</span>{{ form.label }}</button></aside><div class="record-preview"><div class="record-top"><div><div class="breadcrumb-label">{{ currentForm.module }} / {{ currentForm.label }}</div><h2>New {{ currentForm.label }}</h2></div><div><button class="button" @click="emit('back')">Back to Build</button> <button class="button primary" @click="emit('save')">Save definition</button></div></div><p class="record-description">{{ currentForm.description }}</p>
      <div class="record-fields"><label v-for="(field, index) in currentForm.fields" :key="field.name + index">{{ field.label }}<span v-if="field.required" class="required-mark">*</span><input v-if="['Text', 'Email', 'Phone', 'Number', 'Currency', 'Date', 'Link'].includes(field.type)" :type="field.type === 'Email' ? 'email' : field.type === 'Date' ? 'date' : field.type === 'Number' || field.type === 'Currency' ? 'number' : 'text'" :placeholder="`Enter ${field.label.toLowerCase()}`"/><select v-else-if="field.type === 'Select'"><option value="">Select {{ field.label.toLowerCase() }}</option><option>Option 1</option><option>Option 2</option></select><textarea v-else-if="field.type === 'Long text'" rows="4" :placeholder="`Enter ${field.label.toLowerCase()}`"/><input v-else type="checkbox"/></label></div><div v-if="notice" class="notice-row">{{ notice }}</div><div class="record-bottom"><span>Required fields are marked with <b>*</b></span><span>Form definition · {{ currentForm.fields.length }} fields</span></div>
    </div></div>
  </section>
</template>
