<template>
  <BaseDialog
    :show="show"
    :title="t('invoices.voidRequest.title')"
    width="narrow"
    @close="onClose"
  >
    <div class="space-y-4">
      <div class="rounded-md border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-700/40 dark:bg-amber-900/10 dark:text-amber-300">
        {{ t('invoices.voidRequest.warning') }}
      </div>

      <div v-if="invoice" class="rounded-md bg-gray-50 p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">
        <div class="flex justify-between">
          <span>{{ t('invoices.fields.applicationNo') }}</span>
          <span class="font-mono">{{ invoice.application_no }}</span>
        </div>
        <div class="mt-1 flex justify-between">
          <span>{{ t('invoices.fields.invoiceNo') }}</span>
          <span class="font-mono">{{ invoice.invoice_no || '—' }}</span>
        </div>
        <div class="mt-1 flex justify-between">
          <span>{{ t('invoices.fields.title') }}</span>
          <span>{{ invoice.title }}</span>
        </div>
        <div class="mt-1 flex justify-between">
          <span>{{ t('invoices.fields.amount') }}</span>
          <span class="font-semibold text-emerald-600 dark:text-emerald-400">¥{{ invoice.amount.toFixed(2) }}</span>
        </div>
      </div>

      <div>
        <label class="mb-1 block text-sm font-medium text-gray-900 dark:text-white">
          {{ t('invoices.voidRequest.reasonLabel') }} <span class="text-red-500">*</span>
        </label>
        <textarea
          v-model="reason"
          rows="4"
          class="input w-full"
          :placeholder="t('invoices.voidRequest.reasonPlaceholder')"
          maxlength="500"
        />
        <div class="mt-1 text-right text-xs text-gray-400">{{ reason.length }} / 500</div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" :disabled="submitting" @click="onClose">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn btn-danger"
          :disabled="submitting || !reason.trim()"
          @click="submit"
        >
          {{ submitting ? t('common.processing') : t('invoices.voidRequest.submit') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import invoiceAPI from '@/api/invoices'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { Invoice } from '@/types/invoice'

const props = defineProps<{
  show: boolean
  invoice: Invoice | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  submitted: []
}>()

const { t } = useI18n()
const reason = ref('')
const submitting = ref(false)

watch(
  () => props.show,
  (v) => {
    if (v) {
      reason.value = ''
    }
  },
)

function onClose() {
  if (submitting.value) return
  emit('update:show', false)
}

async function submit() {
  if (!props.invoice || !reason.value.trim()) return
  submitting.value = true
  try {
    await invoiceAPI.requestVoid(props.invoice.id, reason.value.trim())
    emit('submitted')
    emit('update:show', false)
  } catch (err) {
    alert(extractApiErrorMessage(err))
  } finally {
    submitting.value = false
  }
}
</script>
