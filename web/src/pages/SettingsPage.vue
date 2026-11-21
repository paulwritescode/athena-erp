// Settings: Workspace preferences in stacked sections. Appearance, Security
// and Workspace identity are live; notification preferences arrive in a
// later batch.
<script setup lang="ts">
import PageHeader from '@/components/PageHeader.vue'
import FormSection from '@/components/FormSection.vue'
import EmptyState from '@/components/EmptyState.vue'
import AppearanceSection from '@/components/settings/AppearanceSection.vue'
import IdentitySection from '@/components/settings/IdentitySection.vue'
import SecuritySection from '@/components/settings/SecuritySection.vue'
import { Bell } from '@/icons'

const props = defineProps<{ permissions: string[] }>()
</script>

<template>
  <div class="space-y-6">
    <PageHeader title="Settings" description="Workspace preferences." />
    <FormSection title="Appearance" description="The Workspace accent, worn by buttons, the focus ring, the selected page and charts.">
      <AppearanceSection :can-edit="props.permissions.includes('settings:appearance')" />
    </FormSection>
    <FormSection title="Security" description="How long sign-ins last and how sign-in is protected.">
      <SecuritySection :permissions="props.permissions" />
    </FormSection>
    <FormSection title="Workspace identity" description="The name and logo people see.">
      <IdentitySection :can-edit="props.permissions.includes('settings:workspace')" />
    </FormSection>
    <FormSection title="Notification preferences" description="Which notifications arrive and where.">
      <EmptyState :icon="Bell" title="Notification preferences are on the way" body="Per-kind notification switches will live here." />
    </FormSection>
  </div>
</template>
