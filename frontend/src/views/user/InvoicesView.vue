<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Filters + actions -->
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-3">
          <Select
            v-model="currentFilter"
            :options="statusFilters"
            class="w-36"
            @change="onFilterChange"
          />
          <div class="flex flex-1 items-center justify-end gap-2">
            <button
              @click="fetchInvoices"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="applyOpen = true">
              {{ t('invoices.apply') }}
            </button>
          </div>
        </div>
      </div>

      <!-- Table -->
      <div class="card overflow-hidden">
        <div v-if="loading" class="py-12 text-center text-sm text-gray-500">
          {{ t('common.loading') }}
        </div>
        <div v-else-if="invoices.length === 0" class="py-12 text-center text-sm text-gray-500">
          {{ t('invoices.empty') }}
        </div>
        <table v-else class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('invoices.fields.applicationNo') }}
              </th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('invoices.fields.invoiceNo') }}
              </th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('invoices.fields.title') }}
              </th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('invoices.fields.amount') }}
              </th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('invoices.fields.status') }}
              </th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('invoices.fields.submittedAt') }}
              </th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('invoices.fields.actions') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
            <tr v-for="inv in invoices" :key="inv.id" class="hover:bg-gray-50 dark:hover:bg-dark-800">
              <td class="px-4 py-3 font-mono text-xs text-gray-900 dark:text-white">
                {{ inv.application_no }}
              </td>
              <td class="px-4 py-3 font-mono text-xs text-gray-900 dark:text-white">
                {{ inv.invoice_no || '—' }}
              </td>
              <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">
                {{ inv.title }}
                <span class="ml-1 text-xs text-gray-500">({{ t('invoices.titleType.' + inv.title_type) }})</span>
              </td>
              <td class="px-4 py-3 text-right text-sm font-semibold text-emerald-600 dark:text-emerald-400">
                ¥{{ inv.amount.toFixed(2) }}
              </td>
              <td class="whitespace-nowrap px-4 py-3">
                <InvoiceStatusBadge :status="inv.status" />
              </td>
              <td class="px-4 py-3 text-sm text-gray-600 dark:text-gray-400">
                {{ formatDate(inv.submitted_at) }}
              </td>
              <td class="px-4 py-3 text-right">
                <div class="flex items-center justify-end gap-2">
                  <button
                    class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
                    @click="openDetail(inv.id)"
                  >
                    <Icon name="eye" size="sm" />
                    <span>{{ t('common.view') }}</span>
                  </button>
                  <button
                    v-if="inv.status === 'pending'"
                    class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                    @click="cancelTargetId = inv.id"
                  >
                    <Icon name="x" size="sm" />
                    <span>{{ t('common.cancel') }}</span>
                  </button>
                  <button
                    v-if="inv.status === 'issued'"
                    type="button"
                    :disabled="downloadingId === inv.id"
                    class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-emerald-600 hover:bg-emerald-50 disabled:opacity-50 dark:text-emerald-400 dark:hover:bg-emerald-900/20"
                    @click="downloadInvoicePdf(inv.id)"
                  >
                    <Icon name="download" size="sm" />
                    <span>{{ t('invoices.detail.downloadPdf') }}</span>
                  </button>
                  <span
                    v-if="inv.status === 'issued' && inv.pending_void_request"
                    class="inline-flex items-center gap-1 rounded-md bg-amber-50 px-2 py-1 text-xs font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
                    :title="t('invoices.voidRequest.pendingTooltip', { time: formatDate(inv.pending_void_request.requested_at), reason: inv.pending_void_request.reason })"
                  >
                    <Icon name="clock" size="sm" />
                    <span>{{ t('invoices.voidRequest.pendingBadge') }}</span>
                  </span>
                  <button
                    v-else-if="inv.status === 'issued' && canRequestVoid(inv)"
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                    @click="openVoidRequest(inv)"
                  >
                    <Icon name="x" size="sm" />
                    <span>{{ t('invoices.voidRequest.button') }}</span>
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Pagination -->
      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
    </div>

    <!-- Apply dialog -->
    <InvoiceApplyDialog
      :show="applyOpen"
      @update:show="(v) => (applyOpen = v)"
      @submitted="onSubmitted"
    />

    <!-- Detail dialog -->
    <InvoiceDetailDialog
      :show="!!detailId"
      :invoice-id="detailId"
      @update:show="(v) => { if (!v) detailId = null }"
    />

    <!-- Void request dialog -->
    <InvoiceVoidRequestDialog
      :show="!!voidRequestTarget"
      :invoice="voidRequestTarget"
      @update:show="(v) => { if (!v) voidRequestTarget = null }"
      @submitted="onVoidSubmitted"
    />

    <!-- Cancel confirm -->
    <BaseDialog
      :show="!!cancelTargetId"
      :title="t('invoices.cancel')"
      width="narrow"
      @close="cancelTargetId = null"
    >
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('invoices.confirmCancel') }}</p>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" @click="cancelTargetId = null">{{ t('common.cancel') }}</button>
          <button
            class="btn btn-danger"
            :disabled="actionLoading"
            @click="confirmCancel"
          >
            {{ actionLoading ? t('common.processing') : t('invoices.cancel') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import InvoiceStatusBadge from '@/components/invoice/InvoiceStatusBadge.vue'
import InvoiceApplyDialog from '@/components/invoice/InvoiceApplyDialog.vue'
import InvoiceDetailDialog from '@/components/invoice/InvoiceDetailDialog.vue'
import InvoiceVoidRequestDialog from '@/components/invoice/InvoiceVoidRequestDialog.vue'
import invoiceAPI from '@/api/invoices'
import { extractApiErrorMessage } from "@/utils/apiError"
import type { Invoice } from '@/types/invoice'

const { t } = useI18n()

const loading = ref(false)
const actionLoading = ref(false)
const invoices = ref<Invoice[]>([])
const currentFilter = ref('')
const applyOpen = ref(false)
const detailId = ref<number | null>(null)
const cancelTargetId = ref<number | null>(null)
const voidRequestTarget = ref<Invoice | null>(null)
const downloadingId = ref<number | null>(null)
const pagination = reactive({ page: 1, page_size: 20, total: 0 })

const statusFilters = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'pending', label: t('invoices.status.pending') },
  { value: 'approved', label: t('invoices.status.approved') },
  { value: 'issued', label: t('invoices.status.issued') },
  { value: 'rejected', label: t('invoices.status.rejected') },
  { value: 'voided', label: t('invoices.status.voided') },
])

async function fetchInvoices() {
  loading.value = true
  try {
    const res = await invoiceAPI.list({
      page: pagination.page,
      page_size: pagination.page_size,
      status: currentFilter.value || undefined,
    })
    invoices.value = res.data.items || []
    pagination.total = res.data.total
  } catch (err) {
    console.error('load invoices', extractApiErrorMessage(err))
    invoices.value = []
    pagination.total = 0
  } finally {
    loading.value = false
  }
}

function openDetail(id: number) {
  detailId.value = id
}

async function downloadInvoicePdf(id: number) {
  if (downloadingId.value) return
  downloadingId.value = id
  try {
    await invoiceAPI.downloadPdf(id)
  } catch (err) {
    alert(extractApiErrorMessage(err))
  } finally {
    downloadingId.value = null
  }
}

async function confirmCancel() {
  if (!cancelTargetId.value) return
  actionLoading.value = true
  try {
    await invoiceAPI.cancel(cancelTargetId.value)
    cancelTargetId.value = null
    await fetchInvoices()
  } catch (err) {
    alert(extractApiErrorMessage(err))
  } finally {
    actionLoading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  fetchInvoices()
}

function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  fetchInvoices()
}

function onSubmitted() {
  pagination.page = 1
  fetchInvoices()
}

function canRequestVoid(inv: Invoice): boolean {
  // 仅 issued + 自动渠道 + provider_state 不在红冲流水线中时允许申请
  // （manual 渠道无法自动红冲；正在红冲的票后端也会拒绝，前端先把按钮藏掉避免误触）
  if (inv.status !== 'issued') return false
  if (!inv.provider || inv.provider === 'manual') return false
  const reverseStates: Array<Invoice['provider_state']> = [
    'reverse_pending',
    'reversing',
    'reverse_success',
    'reverse_failed',
  ]
  return !reverseStates.includes(inv.provider_state)
}

function openVoidRequest(inv: Invoice) {
  voidRequestTarget.value = inv
}

function onVoidSubmitted() {
  fetchInvoices()
}

function onFilterChange() {
  pagination.page = 1
  fetchInvoices()
}

function formatDate(s: string): string {
  if (!s) return ''
  try {
    return new Date(s).toLocaleString()
  } catch {
    return s
  }
}

onMounted(() => {
  fetchInvoices()
})
</script>
