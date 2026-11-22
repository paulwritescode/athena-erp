// Step indicator for the Setup wizard (B11 slice 2): numbered steps with
// words on tablet and desktop, a "Step N of 6" caption with a progress bar
// on phones (§3.1). Reached steps are buttons back; future steps are plain
// text, never disabled buttons without a reason.
<script setup lang="ts">
import { computed } from 'vue'
import { CircleCheck } from '@/icons'
import { Progress } from '@/components/ui/progress'
import { setupSteps, type SetupStepId } from './setupModel'

const props = defineProps<{
  current: SetupStepId
  done: ReadonlySet<SetupStepId>
  furthest: number
}>()
const emit = defineEmits<{ goto: [index: number] }>()

const currentIndex = computed(() => setupSteps.findIndex((step) => step.id === props.current))
const percent = computed(() => Math.round((props.done.size / setupSteps.length) * 100))
</script>

<template>
  <p class="text-muted-foreground text-sm sm:hidden" aria-live="polite">
    Step {{ currentIndex + 1 }} of {{ setupSteps.length }} · {{ setupSteps[currentIndex]?.word }}
  </p>
  <Progress :model-value="percent" class="mt-2 sm:hidden" aria-hidden="true" />
  <ol class="mt-1 hidden items-start sm:flex" :aria-label="`Setup step ${currentIndex + 1} of ${setupSteps.length}`">
    <li v-for="(step, index) in setupSteps" :key="step.id" class="flex flex-1 items-start last:flex-none">
      <component
        :is="index <= furthest ? 'button' : 'span'"
        type="button"
        class="group flex items-center gap-2"
        :class="index <= furthest && index !== currentIndex ? 'cursor-pointer' : 'cursor-default'"
        :aria-current="index === currentIndex ? 'step' : undefined"
        @click="index <= furthest && index !== currentIndex ? emit('goto', index) : undefined"
      >
        <span
          class="flex size-6 items-center justify-center rounded-full border text-xs font-medium"
          :class="
            done.has(step.id)
              ? 'border-transparent bg-success-soft text-success'
              : index === currentIndex
                ? 'border-primary bg-primary text-primary-foreground'
                : 'text-muted-foreground'
          "
          aria-hidden="true"
        >
          <CircleCheck v-if="done.has(step.id)" class="size-4" />
          <span v-else>{{ index + 1 }}</span>
        </span>
        <span
          class="text-sm"
          :class="index === currentIndex ? 'text-foreground font-medium' : done.has(step.id) ? 'text-foreground' : 'text-muted-foreground'"
        >
          {{ step.word }}
        </span>
      </component>
      <span v-if="index < setupSteps.length - 1" class="mx-2 mt-3 h-px flex-1 bg-border" aria-hidden="true" />
    </li>
  </ol>
</template>
