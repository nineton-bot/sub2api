<template>
  <BaseDialog
    :show="show"
    title="驳回用户作废申请"
    width="narrow"
    @close="onClose"
  >
    <div class="space-y-3">
      <div v-if="invoice" class="rounded-md bg-gray-50 p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">
        <div class="flex justify-between"><span>申请单号</span><span class="font-mono">{{ invoice.application_no }}</span></div>
        <div class="mt-1 flex justify-between"><span>发票号码</span><span class="font-mono">{{ invoice.invoice_no || '—' }}</span></div>
        <div v-if="invoice.pending_void_request" class="mt-2">
          <div class="text-gray-500">用户作废原因：</div>
          <div class="mt-1 break-all">{{ invoice.pending_void_request.reason }}</div>
        </div>
      </div>

      <div>
        <label class="mb-1 block text-sm font-medium text-gray-900 dark:text-white">
          驳回理由 <span class="text-red-500">*</span>
        </label>
        <textarea
          v-model="reason"
          rows="4"
          class="input w-full"
          placeholder="请说明驳回理由，用户将看到此内容"
          maxlength="500"
        />
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" :disabled="submitting" @click="onClose">取消</button>
        <button class="btn btn-danger" :disabled="submitting || !reason.trim()" @click="submit">
          {{ submitting ? '处理中...' : '驳回' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import adminInvoiceAPI from '@/api/admin/invoices'
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

const reason = ref('')
const submitting = ref(false)

watch(() => props.show, (v) => { if (v) reason.value = '' })

function onClose() {
  if (submitting.value) return
  emit('update:show', false)
}

async function submit() {
  if (!props.invoice?.pending_void_request || !reason.value.trim()) return
  submitting.value = true
  try {
    await adminInvoiceAPI.rejectVoidRequest(props.invoice.pending_void_request.id, reason.value.trim())
    emit('submitted')
    emit('update:show', false)
  } catch (err) {
    alert(extractApiErrorMessage(err))
  } finally {
    submitting.value = false
  }
}
</script>
