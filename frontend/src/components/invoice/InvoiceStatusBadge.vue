<template>
  <span
    class="inline-flex items-center whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium"
    :class="statusClass"
  >
    {{ statusLabel }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { InvoiceStatus } from '@/types/invoice'

const props = defineProps<{
  status: InvoiceStatus
}>()

const { t } = useI18n()

const statusMap: Record<InvoiceStatus, { key: string; class: string }> = {
  pending: {
    key: 'invoices.status.pending',
    class: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400',
  },
  approved: {
    key: 'invoices.status.approved',
    class: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
  },
  issued: {
    key: 'invoices.status.issued',
    class: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-400',
  },
  rejected: {
    key: 'invoices.status.rejected',
    class: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
  },
  voided: {
    key: 'invoices.status.voided',
    class: 'bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400',
  },
}

const statusLabel = computed(() => {
  const entry = statusMap[props.status]
  return entry ? t(entry.key) : props.status
})

const statusClass = computed(() => {
  const entry = statusMap[props.status]
  return entry?.class ?? 'bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400'
})
</script>
