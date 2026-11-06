<script setup lang="ts">
import { computed, ref, watch } from 'vue'

interface AutomationDraft {
  id: string
  name: string
  trigger: string
  condition: string
  action: string
  enabled: boolean
}

const storageKey = 'r3p.automation-drafts.v1'
const triggerOptions = [
  { value: 'cpu-high', label: 'CPU exceeds threshold' },
  { value: 'memory-high', label: 'Memory exceeds threshold' },
  { value: 'client-joins', label: 'A DHCP lease becomes active' },
  { value: 'usb-mounted', label: 'USB storage is mounted' },
]
const conditionOptions = [
  { value: 'once', label: 'Once per event' },
  { value: 'five-minutes', label: 'Condition persists for 5 minutes' },
  { value: 'quiet-hours', label: 'Only during quiet hours (22:00–07:00)' },
]
const actionOptions = [
  { value: 'record-event', label: 'Record an event in this draft' },
  { value: 'notify-ui', label: 'Show a dashboard notification (planned)' },
]

function loadDrafts(): AutomationDraft[] {
  try {
    const raw = localStorage.getItem(storageKey)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((item): item is AutomationDraft =>
      item !== null &&
      typeof item === 'object' &&
      typeof item.id === 'string' &&
      typeof item.name === 'string' &&
      typeof item.trigger === 'string' &&
      typeof item.condition === 'string' &&
      typeof item.action === 'string' &&
      typeof item.enabled === 'boolean',
    ).slice(0, 20)
  } catch (cause) {
    console.warn('Could not load saved automation drafts.', cause)
    return []
  }
}

const drafts = ref<AutomationDraft[]>(loadDrafts())
const name = ref('')
const trigger = ref(triggerOptions[0]!.value)
const condition = ref(conditionOptions[0]!.value)
const action = ref(actionOptions[0]!.value)
const error = ref<string | null>(null)
const canSave = computed(() => name.value.trim().length > 0 && drafts.value.length < 20)

watch(drafts, (value) => {
  try {
    localStorage.setItem(storageKey, JSON.stringify(value))
    error.value = null
  } catch (cause) {
    error.value = 'Could not save this draft in browser storage.'
    console.error('Could not save automation drafts.', cause)
  }
}, { deep: true })

function addDraft() {
  if (!canSave.value) return
  drafts.value.unshift({
    id: crypto.randomUUID(),
    name: name.value.trim(),
    trigger: trigger.value,
    condition: condition.value,
    action: action.value,
    enabled: false,
  })
  name.value = ''
}

function labelFor(options: Array<{ value: string; label: string }>, value: string) {
  return options.find((option) => option.value === value)?.label ?? 'Unknown rule'
}

function removeDraft(id: string) {
  drafts.value = drafts.value.filter((draft) => draft.id !== id)
}
</script>

<template>
  <section class="h-full overflow-y-auto p-5 md:p-6 space-y-6">
    <header>
      <h2 class="text-xl font-semibold text-white">Visual automation builder</h2>
      <p class="mt-1 text-sm text-gray-400">Compose trigger → condition → action flows without shell scripts or cron syntax.</p>
    </header>

    <div class="glass-card p-5 space-y-4">
      <div class="grid gap-4 lg:grid-cols-4">
        <label class="form-label">
          Rule name
          <input v-model="name" class="form-control" maxlength="64" placeholder="Evening network watch" />
        </label>
        <label class="form-label">
          <span>01 · When</span>
          <select v-model="trigger" class="form-control">
            <option v-for="option in triggerOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
        </label>
        <label class="form-label">
          <span>02 · If</span>
          <select v-model="condition" class="form-control">
            <option v-for="option in conditionOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
        </label>
        <label class="form-label">
          <span>03 · Then</span>
          <select v-model="action" class="form-control">
            <option v-for="option in actionOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
        </label>
      </div>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-xs text-amber-300">Drafts are saved in this browser only. The router automation engine is not implemented; rules cannot run.</p>
        <button class="primary-button" :disabled="!canSave" @click="addDraft">Save draft</button>
      </div>
      <p v-if="error" role="alert" class="text-sm text-red-300">{{ error }}</p>
    </div>

    <section class="space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="font-semibold text-white">Saved drafts</h3>
        <span class="text-xs text-gray-500">{{ drafts.length }} / 20</span>
      </div>
      <p v-if="drafts.length === 0" class="glass-card p-6 text-sm text-gray-400">No automation drafts saved yet.</p>
      <article v-for="draft in drafts" :key="draft.id" class="glass-card p-4">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h4 class="truncate font-medium text-white">{{ draft.name }}</h4>
            <p class="mt-2 text-sm text-gray-300">
              <span class="text-sky-300">When</span> {{ labelFor(triggerOptions, draft.trigger) }}
              <span class="mx-1 text-gray-600">→</span>
              <span class="text-purple-300">If</span> {{ labelFor(conditionOptions, draft.condition) }}
              <span class="mx-1 text-gray-600">→</span>
              <span class="text-emerald-300">Then</span> {{ labelFor(actionOptions, draft.action) }}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <span class="status-pill status-pill-muted">Draft · not active</span>
            <button class="secondary-button" :aria-label="`Delete draft ${draft.name}`" @click="removeDraft(draft.id)">Remove</button>
          </div>
        </div>
      </article>
    </section>
  </section>
</template>
