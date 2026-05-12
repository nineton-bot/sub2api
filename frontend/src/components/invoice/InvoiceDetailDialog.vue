<template>
  <BaseDialog :show="show" :title="t('invoices.detail.title')" width="wide" @close="onClose">
    <div v-if="loading" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
    <div v-else-if="!detail" class="py-8 text-center text-sm text-gray-500">{{ t('invoices.detail.notFound') }}</div>
    <div v-else class="space-y-5">
      <!-- 基本信息 -->
      <div class="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoices.fields.invoiceNo') }}</div>
          <div class="font-mono text-gray-900 dark:text-white">{{ detail.invoice_no || '—' }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoices.fields.status') }}</div>
          <InvoiceStatusBadge :status="detail.status" />
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoices.fields.title') }}</div>
          <div class="text-gray-900 dark:text-white">
            {{ detail.title }}
            <span class="ml-1 text-xs text-gray-500">({{ t('invoices.titleType.' + detail.title_type) }})</span>
          </div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoices.fields.amount') }}</div>
          <div class="text-base font-semibold text-emerald-600 dark:text-emerald-400">¥{{ detail.amount.toFixed(2) }}</div>
        </div>
        <div v-if="detail.tax_no">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoices.fields.taxNo') }}</div>
          <div class="font-mono text-gray-900 dark:text-white">{{ detail.tax_no }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoices.fields.submittedAt') }}</div>
          <div class="text-gray-900 dark:text-white">{{ formatDate(detail.submitted_at) }}</div>
        </div>
        <div v-if="detail.contact_email">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoices.fields.contactEmail') }}</div>
          <div class="text-gray-900 dark:text-white">{{ detail.contact_email }}</div>
        </div>
        <div v-if="detail.issued_at">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('invoices.detail.issuedAt') }}</div>
          <div class="text-gray-900 dark:text-white">{{ formatDate(detail.issued_at) }}</div>
        </div>
      </div>

      <!-- 备注 -->
      <div v-if="detail.notes" class="rounded-lg bg-gray-50 p-3 text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-400">
        <div class="mb-1 font-semibold">{{ t('invoices.fields.notes') }}</div>
        {{ detail.notes }}
      </div>
      <div v-if="detail.review_notes" class="rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
        <div class="mb-1 font-semibold">{{ t('invoices.detail.reviewNotes') }}</div>
        {{ detail.review_notes }}
      </div>

      <!-- 订单明细 -->
      <div>
        <h4 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('invoices.detail.items') }}</h4>
        <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr>
                <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">{{ t('invoices.fields.orderNo') }}</th>
                <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">{{ t('invoices.fields.product') }}</th>
                <th class="px-3 py-2 text-right text-xs font-medium uppercase text-gray-500 dark:text-gray-400">{{ t('invoices.fields.amount') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
              <tr v-for="it in detail.items" :key="it.order_id">
                <td class="px-3 py-2 font-mono text-xs text-gray-900 dark:text-white">{{ it.order_no }}</td>
                <td class="px-3 py-2 text-gray-900 dark:text-white">{{ it.product_name }}</td>
                <td class="px-3 py-2 text-right text-gray-900 dark:text-white">¥{{ it.pay_amount.toFixed(2) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- PDF 下载 -->
      <div v-if="detail.pdf_available" class="flex justify-end">
        <button
          type="button"
          class="btn btn-primary inline-flex items-center gap-2"
          :disabled="downloading"
          @click="downloadPdf"
        >
          {{ downloading ? t('common.loading') : t('invoices.detail.downloadPdf') }}
        </button>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button class="btn btn-secondary" @click="onClose">{{ t('common.close') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import InvoiceStatusBadge from './InvoiceStatusBadge.vue'
import invoiceAPI from '@/api/invoices'
import adminInvoiceAPI from '@/api/admin/invoices'
import { extractApiErrorMessage } from "@/utils/apiError"
import type { InvoiceDetail } from '@/types/invoice'

const props = defineProps<{
  show: boolean
  invoiceId: number | null
  /** admin=true 时使用管理员 PDF endpoint */
  admin?: boolean
  /** 当 admin=true 时传入 admin api 取详情；用户端默认走 invoiceAPI */
  fetcher?: (id: number) => Promise<{ data: InvoiceDetail }>
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const { t } = useI18n()
const detail = ref<InvoiceDetail | null>(null)
const loading = ref(false)
const downloading = ref(false)

async function downloadPdf() {
  if (!detail.value || downloading.value) return
  downloading.value = true
  try {
    if (props.admin) {
      await adminInvoiceAPI.downloadPdf(detail.value.id)
    } else {
      await invoiceAPI.downloadPdf(detail.value.id)
    }
  } catch (err) {
    alert(extractApiErrorMessage(err))
  } finally {
    downloading.value = false
  }
}

watch(
  () => [props.show, props.invoiceId] as const,
  async ([show, id]) => {
    if (!show || !id) {
      detail.value = null
      return
    }
    loading.value = true
    try {
      const res = props.fetcher
        ? await props.fetcher(id)
        : await invoiceAPI.detail(id)
      detail.value = res.data
    } catch (err) {
      console.error('load invoice detail', extractApiErrorMessage(err))
      detail.value = null
    } finally {
      loading.value = false
    }
  },
  { immediate: false },
)

function formatDate(s: string | null | undefined): string {
  if (!s) return ''
  try {
    return new Date(s).toLocaleString()
  } catch {
    return s
  }
}

function onClose() {
  emit('update:show', false)
}
</script>
