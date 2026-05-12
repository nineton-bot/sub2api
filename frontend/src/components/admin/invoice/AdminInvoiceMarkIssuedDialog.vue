<template>
  <BaseDialog
    :show="show"
    :title="t('admin.invoices.actions.markIssued')"
    width="narrow"
    @close="onClose"
  >
    <div class="space-y-3">
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.invoices.dialogs.upload.invoiceNoLabel') }}
      </label>
      <input
        v-model="invoiceNo"
        type="text"
        class="input w-full"
        :placeholder="t('admin.invoices.dialogs.upload.invoiceNoPlaceholder')"
      />
      <p class="text-xs text-gray-500">
        {{ t('admin.invoices.dialogs.markIssued.note') }}
      </p>
      <div v-if="errorMsg" class="rounded-lg bg-red-50 p-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMsg }}
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" @click="onClose">{{ t('common.cancel') }}</button>
        <button
          class="btn btn-primary"
          :disabled="submitting || !invoiceNo.trim()"
          @click="submit"
        >
          {{ submitting ? t('common.processing') : t('admin.invoices.actions.markIssued') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import adminInvoiceAPI from '@/api/admin/invoices'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{
  show: boolean
  invoiceId: number | null
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  (e: 'submitted'): void
}>()

const { t } = useI18n()
const invoiceNo = ref('')
const submitting = ref(false)
const errorMsg = ref('')

watch(
  () => props.show,
  (v) => {
    if (v) {
      invoiceNo.value = ''
      errorMsg.value = ''
    }
  },
)

async function submit() {
  if (!props.invoiceId || !invoiceNo.value.trim()) return
  submitting.value = true
  errorMsg.value = ''
  try {
    await adminInvoiceAPI.markIssued(props.invoiceId, invoiceNo.value.trim())
    emit('submitted')
    onClose()
  } catch (err) {
    errorMsg.value = extractApiErrorMessage(err)
  } finally {
    submitting.value = false
  }
}

function onClose() {
  emit('update:show', false)
}
</script>
