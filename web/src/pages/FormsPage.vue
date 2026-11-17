<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FormDefinition, Field } from '../types'
const props = defineProps<{ forms: FormDefinition[]; currentForm: FormDefinition; databaseState: string }>()
const emit = defineEmits<{ select: [name: string]; addForm: []; addField: []; save: []; preview: [] }>()
const showNewForm = ref(false)
const search = ref('')
const filteredForms = computed(() => props.forms.filter((item) => `${item.label} ${item.name} ${item.module}`.toLowerCase().includes(search.value.toLowerCase())))
</script>

<template>
  <section class="page-content builder-page">
    <div class="page-title-row"><div><div class="eyebrow">BUILD · DATA MODEL</div><h1>Forms</h1><p class="lead">Configure the fields used by each kind of business record.</p></div><button class="button primary" @click="showNewForm = !showNewForm">＋ New Form</button></div>
    <div v-if="showNewForm" class="notice-row">Create a new Form definition?<button class="button small" @click="emit('addForm'); showNewForm = false">Create Form</button><button class="text-button" @click="showNewForm = false">Cancel</button></div>
    <div class="builder-layout">
      <aside class="builder-list"><div class="builder-list-heading"><strong>FORM DEFINITIONS</strong><span>{{ forms.length }}</span></div><label class="search-box"><span>⌕</span><input v-model="search" placeholder="Find a Form" /></label><button v-for="form in filteredForms" :key="form.name" class="form-choice" :class="{ chosen: currentForm.name === form.name }" @click="emit('select', form.name)"><span class="list-icon">▤</span><span><strong>{{ form.label }}</strong><small>{{ form.module }}</small></span></button><div v-if="filteredForms.length === 0" class="empty-state">No matching Forms.</div></aside>
      <div class="editor-column"><div class="editor-toolbar"><div><span class="breadcrumb-label">Forms /</span><strong>{{ currentForm.label }}</strong><span class="status-chip">Draft</span></div><div class="toolbar-actions"><button class="button" @click="emit('preview')">Preview</button><button class="button primary" @click="emit('save')">Save</button></div></div>
        <div class="editor-card"><div class="form-heading"><div class="form-symbol">▤</div><div><input v-model="currentForm.label" class="title-input" aria-label="Form name"/><p>{{ currentForm.description }}</p></div></div>
          <div class="tab-strip"><button class="active">Fields</button><button>Rules <span class="planned-tag">PLANNED</span></button><button>Module</button><button>Permissions <span class="planned-tag">PLANNED</span></button></div>
          <div class="field-table"><div class="field-head"><span>LABEL</span><span>FIELD NAME</span><span>TYPE</span><span>REQUIRED</span><span></span></div><div v-for="(field, index) in currentForm.fields" :key="field.name + index" class="field-row"><span><input v-model="field.label" :aria-label="`Field ${index + 1} label`" /></span><span><code>{{ field.name }}</code></span><span><select v-model="field.type" aria-label="Field type"><option>Text</option><option>Long text</option><option>Email</option><option>Phone</option><option>Number</option><option>Currency</option><option>Date</option><option>Select</option><option>Link</option><option>Checkbox</option></select></span><span><input v-model="field.required" type="checkbox" :aria-label="`${field.label} required`" /></span><span><button class="icon-action" :aria-label="`Remove ${field.label}`" @click="currentForm.fields.splice(index, 1)">×</button></span></div></div>
          <button class="add-field" @click="emit('addField')">＋ Add field</button><div class="editor-footnote">{{ databaseState === 'Connected' ? 'Changes can be saved to MariaDB.' : 'Changes are in browser memory until MariaDB is connected.' }}</div>
        </div>
      </div>
      <aside class="inspector"><div class="inspector-title">FORM SETTINGS</div><label>Form name<input :value="currentForm.name" readonly /></label><label>Module<select v-model="currentForm.module"><option>Core</option><option>Invoicing</option><option>Support</option></select></label><label>Description<textarea v-model="currentForm.description" rows="3" /></label><div class="inspector-note"><strong>Record definition</strong><p>A Form describes one record type. Use Modules to bundle related Forms into a feature.</p></div></aside>
    </div>
  </section>
</template>
