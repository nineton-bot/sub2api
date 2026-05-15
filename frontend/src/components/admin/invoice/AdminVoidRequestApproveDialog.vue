<template>
  <BaseDialog
    :show="show"
    title="通过用户作废申请"
    width="narrow"
    @close="onClose"
  >
    <div class="space-y-3">
      <div class="rounded-md border border-red-200 bg-red-50 p-3 text-xs text-red-800 dark:border-red-700/40 dark:bg-red-900/10 dark:text-red-300">
        通过后将立即调用开票平台真红冲，原票作废且不可恢复。
      </div>

      <div v-if="invoice" class="rounded-md bg-gray-50 p-3 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">
        <div class="flex justify-between"><span>申请单号</span><span class="font-mono">{{ invoice.application_no }}</span></div>
        <div class="mt-1 flex justify-between"><span>发票号码</span><span class="font-mono">{{ invoice.invoice_no || '—' }}</span></div>
        <div class="mt-1 flex justify-between"><span>抬头</span><span>{{ invoice.title }}</span></div>
        <div class="mt-1 flex justify-between"><span>金额</span><span class="font-semibold text-emerald-600">¥{{ invoice.amount.toFixed(2) }}</span></div>
        <div v-if="invoice.pending_void_request" class="mt-2 border-t border-gray-200 pt-2 dark:border-dark-700">
          <div class="text-gray-500">用户作废原因：</div>
          <div class="mt-1 break-all">{{ invoice.pending_void_request.reason }}</div>
        </div>
      </div>

      <div>
        <label class="mb-1 block text-sm font-medium text-gray-900 dark:text-white">管理员备注</label>
        <textarea
          v-model="notes"
          rows="3"
          class="input w-full"
          placeholder="可选，记录审批意见"
          maxlength="500"
        />
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" :disabled="submitting" @click="onClose">取消</button>
        <button class="btn btn-danger" :disabled="submitting" @click="submit">
          {{ submitting ? '处理中...' : '通过并红冲' }}
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

const notes = ref('')
const submitting = ref(false)

watch(() => props.show, (v) => { if (v) notes.value = '' })

function onClose() {
  if (submitting.value) return
  emit('update:show', false)
}

async function submit() {
  if (!props.invoice?.pending_void_request) return
  submitting.value = true
  try {
    await adminInvoiceAPI.approveVoidRequest(props.invoice.pending_void_request.id, notes.value.trim())
    emit('submitted')
    emit('update:show', false)
  } catch (err) {
    alert(extractApiErrorMessage(err))
  } finally {
    submitting.value = false
  }
}
</script>
