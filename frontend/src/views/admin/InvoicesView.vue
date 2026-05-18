<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Filters -->
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-3">
          <Select
            v-model="currentFilter"
            :options="statusFilters"
            class="w-36"
            @change="onFilterChange"
          />
          <input
            v-model="emailFilter"
            type="text"
            class="input w-56"
            :placeholder="t('admin.invoices.filters.email')"
            @keyup.enter="onFilterChange"
          />
          <label class="inline-flex items-center gap-1 text-sm text-gray-700 dark:text-gray-300">
            <input
              type="checkbox"
              class="h-4 w-4"
              v-model="voidPendingFilter"
              @change="onFilterChange"
            />
            只看待审批作废
          </label>
          <button class="btn btn-secondary" @click="fetchInvoices()">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>

      <!-- Table -->
      <div class="card overflow-hidden">
        <div v-if="loading" class="py-12 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else-if="invoices.length === 0" class="py-12 text-center text-sm text-gray-500">{{ t('common.noData') }}</div>
        <table v-else class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('invoices.fields.applicationNo') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('invoices.fields.invoiceNo') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.invoices.filters.email') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('invoices.fields.title') }}</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">{{ t('invoices.fields.amount') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('invoices.fields.status') }}</th>
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">{{ t('invoices.fields.submittedAt') }}</th>
              <th class="px-4 py-3 text-right text-xs font-medium uppercase text-gray-500">{{ t('invoices.fields.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 dark:divide-dark-700">
            <tr v-for="inv in invoices" :key="inv.id" class="hover:bg-gray-50 dark:hover:bg-dark-800">
              <td class="px-4 py-3 font-mono text-xs">{{ inv.application_no }}</td>
              <td class="px-4 py-3 font-mono text-xs">{{ inv.invoice_no || '—' }}</td>
              <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">{{ inv.user_email }}</td>
              <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">
                {{ inv.title }}
                <span class="ml-1 text-xs text-gray-500">({{ t('invoices.titleType.' + inv.title_type) }})</span>
              </td>
              <td class="px-4 py-3 text-right text-sm font-semibold text-emerald-600">¥{{ inv.amount.toFixed(2) }}</td>
              <td class="whitespace-nowrap px-4 py-3">
                <InvoiceStatusBadge :status="inv.status" />
                <div
                  v-if="inv.source === 'bank_transfer'"
                  class="mt-1 inline-flex items-center whitespace-nowrap rounded-md px-1.5 py-0.5 text-[11px] font-medium"
                  :class="inv.transfer_confirmed
                    ? 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/30 dark:text-indigo-300'
                    : 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'"
                >
                  对公转账 · {{ inv.transfer_confirmed ? '已确认收款' : '待确认收款' }}
                </div>
                <div
                  v-if="inv.pending_void_request"
                  class="mt-1 inline-flex items-center gap-1 rounded-md bg-red-100 px-1.5 py-0.5 text-[11px] font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300"
                >
                  <span>⚠ 待审批作废</span>
                  <HelpTooltip
                    :content="`提交于 ${formatDate(inv.pending_void_request.requested_at)}\n原因：${inv.pending_void_request.reason}`"
                    trigger="click"
                    width-class="max-w-md whitespace-pre-line break-all"
                  >
                    <template #trigger>
                      <svg class="h-3.5 w-3.5 cursor-pointer" fill="currentColor" viewBox="0 0 20 20" aria-label="查看作废申请详情">
                        <path fill-rule="evenodd" d="M18 10A8 8 0 11 2 10a8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
                      </svg>
                    </template>
                  </HelpTooltip>
                </div>
                <div
                  v-if="providerStateLabel(inv)"
                  class="mt-1 inline-flex items-center gap-1 text-[11px]"
                  :class="providerStateColor(inv)"
                >
                  <span>{{ providerStateLabel(inv) }}</span>
                  <!-- 失败时把详细 error 收到 tooltip 里，避免列宽被超长报错撑爆 -->
                  <HelpTooltip
                    v-if="inv.provider_last_error"
                    :content="inv.provider_last_error"
                    trigger="click"
                    width-class="max-w-md break-all"
                  >
                    <template #trigger>
                      <svg
                        class="h-3.5 w-3.5 cursor-pointer text-amber-600 transition-colors hover:text-amber-800 dark:text-amber-400 dark:hover:text-amber-300"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                        :aria-label="t('common.viewError') || '查看错误详情'"
                      >
                        <path
                          fill-rule="evenodd"
                          d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v4a.75.75 0 01-1.5 0v-4A.75.75 0 0110 5zm0 8a1 1 0 100 2 1 1 0 000-2z"
                          clip-rule="evenodd"
                        />
                      </svg>
                    </template>
                  </HelpTooltip>
                </div>
              </td>
              <td class="px-4 py-3 text-xs text-gray-600 dark:text-gray-400">{{ formatDate(inv.submitted_at) }}</td>
              <td class="px-4 py-3 text-right">
                <div class="flex flex-wrap items-center justify-end gap-1">
                  <button class="action-btn text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20" @click="detailId = inv.id">
                    {{ t('admin.invoices.actions.view') }}
                  </button>
                  <!-- v3：自动开票失败后的恢复入口
                       仅在主状态仍是 approved 时才显示——若已被 reject/void，订单已释放，
                       重试也走不通（service 层会拦下报 INVOICE_NOT_APPROVED）。 -->
                  <template v-if="inv.status === 'approved' && inv.provider_state === 'failed'">
                    <button class="action-btn text-amber-600 hover:bg-amber-50 dark:text-amber-400 dark:hover:bg-amber-900/20" @click="retryIssue(inv.id)">
                      重新开票
                    </button>
                  </template>
                  <!-- 红冲失败仅在 issued 状态下才能继续 -->
                  <template v-if="inv.status === 'issued' && inv.provider_state === 'reverse_failed'">
                    <button class="action-btn text-amber-600 hover:bg-amber-50 dark:text-amber-400 dark:hover:bg-amber-900/20" @click="retryReverse(inv.id)">
                      重新红冲
                    </button>
                    <button class="action-btn text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700" @click="markReversed(inv)">
                      标记已手工红冲
                    </button>
                  </template>
                  <template v-if="inv.status === 'pending'">
                    <button
                      v-if="inv.source === 'bank_transfer' && !inv.transfer_confirmed"
                      class="action-btn text-orange-600 hover:bg-orange-50 dark:text-orange-400 dark:hover:bg-orange-900/20"
                      @click="confirmTransfer(inv.id)"
                    >
                      确认收款
                    </button>
                    <button
                      v-if="inv.source !== 'bank_transfer' || inv.transfer_confirmed"
                      class="action-btn text-emerald-600 hover:bg-emerald-50 dark:text-emerald-400 dark:hover:bg-emerald-900/20"
                      @click="approve(inv)"
                    >
                      {{ t('admin.invoices.actions.approve') }}
                    </button>
                    <button class="action-btn text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" @click="rejectTargetId = inv.id">
                      {{ t('admin.invoices.actions.reject') }}
                    </button>
                  </template>
                  <template v-if="inv.status === 'approved'">
                    <button class="action-btn text-emerald-600 hover:bg-emerald-50 dark:text-emerald-400 dark:hover:bg-emerald-900/20" @click="openUpload(inv.id, false)">
                      {{ t('admin.invoices.actions.uploadPdf') }}
                    </button>
                    <button class="action-btn text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700" @click="markIssuedTargetId = inv.id">
                      {{ t('admin.invoices.actions.markIssued') }}
                    </button>
                    <button class="action-btn text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" @click="openVoid(inv, false)">
                      {{ t('admin.invoices.actions.void') }}
                    </button>
                  </template>
                  <template v-if="inv.status === 'issued'">
                    <button
                      type="button"
                      :disabled="downloadingId === inv.id"
                      class="action-btn text-emerald-600 hover:bg-emerald-50 disabled:opacity-50 dark:text-emerald-400 dark:hover:bg-emerald-900/20"
                      @click="downloadAdminPdf(inv.id)"
                    >
                      {{ t('admin.invoices.actions.downloadPdf') }}
                    </button>
                    <button
                      v-if="!inv.pending_void_request"
                      class="action-btn text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
                      @click="openUpload(inv.id, true)"
                    >
                      {{ t('admin.invoices.actions.replacePdf') }}
                    </button>
                    <template v-if="inv.pending_void_request">
                      <button class="action-btn text-red-700 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-900/20" @click="voidApproveTarget = inv">
                        通过作废
                      </button>
                      <button class="action-btn text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700" @click="voidRejectTarget = inv">
                        驳回作废
                      </button>
                    </template>
                    <button v-else class="action-btn text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" @click="openVoid(inv, true)">
                      {{ t('admin.invoices.actions.void') }}
                    </button>
                  </template>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="(p) => { pagination.page = p; fetchInvoices() }"
        @update:pageSize="(s) => { pagination.page_size = s; pagination.page = 1; fetchInvoices() }"
      />
    </div>

    <!-- Detail dialog (admin fetcher) -->
    <InvoiceDetailDialog
      :show="!!detailId"
      :invoice-id="detailId"
      :admin="true"
      :fetcher="adminFetcher"
      @update:show="(v) => { if (!v) detailId = null }"
    />

    <!-- Upload PDF dialog -->
    <AdminInvoiceUploadDialog
      :show="!!uploadTargetId"
      :invoice-id="uploadTargetId"
      :replace="uploadReplace"
      @update:show="(v) => { if (!v) uploadTargetId = null }"
      @submitted="onActionDone"
    />

    <!-- Reject dialog -->
    <AdminInvoiceRejectDialog
      :show="!!rejectTargetId"
      :invoice-id="rejectTargetId"
      @update:show="(v) => { if (!v) rejectTargetId = null }"
      @submitted="onActionDone"
    />

    <!-- Void dialog -->
    <AdminInvoiceVoidDialog
      :show="!!voidTargetId"
      :invoice-id="voidTargetId"
      :warn-issued="voidIssued"
      :will-auto-reverse="voidWillAutoReverse"
      @update:show="(v) => { if (!v) voidTargetId = null }"
      @submitted="onActionDone"
    />

    <!-- Mark issued dialog -->
    <AdminInvoiceMarkIssuedDialog
      :show="!!markIssuedTargetId"
      :invoice-id="markIssuedTargetId"
      @update:show="(v) => { if (!v) markIssuedTargetId = null }"
      @submitted="onActionDone"
    />

    <!-- Approve dialog (v3：含票种 + 渠道选择) -->
    <AdminInvoiceApproveDialog
      :show="!!approveTarget"
      :invoice="approveTarget"
      @update:show="(v) => { if (!v) approveTarget = null }"
      @submitted="onActionDone"
    />

    <!-- Void request approve / reject -->
    <AdminVoidRequestApproveDialog
      :show="!!voidApproveTarget"
      :invoice="voidApproveTarget"
      @update:show="(v) => { if (!v) voidApproveTarget = null }"
      @submitted="onActionDone"
    />
    <AdminVoidRequestRejectDialog
      :show="!!voidRejectTarget"
      :invoice="voidRejectTarget"
      @update:show="(v) => { if (!v) voidRejectTarget = null }"
      @submitted="onActionDone"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import InvoiceStatusBadge from '@/components/invoice/InvoiceStatusBadge.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import InvoiceDetailDialog from '@/components/invoice/InvoiceDetailDialog.vue'
import AdminInvoiceUploadDialog from '@/components/admin/invoice/AdminInvoiceUploadDialog.vue'
import AdminInvoiceRejectDialog from '@/components/admin/invoice/AdminInvoiceRejectDialog.vue'
import AdminInvoiceVoidDialog from '@/components/admin/invoice/AdminInvoiceVoidDialog.vue'
import AdminInvoiceMarkIssuedDialog from '@/components/admin/invoice/AdminInvoiceMarkIssuedDialog.vue'
import AdminInvoiceApproveDialog from '@/components/admin/invoice/AdminInvoiceApproveDialog.vue'
import AdminVoidRequestApproveDialog from '@/components/admin/invoice/AdminVoidRequestApproveDialog.vue'
import AdminVoidRequestRejectDialog from '@/components/admin/invoice/AdminVoidRequestRejectDialog.vue'
import adminInvoiceAPI from '@/api/admin/invoices'
import { extractApiErrorMessage } from "@/utils/apiError"
import type { Invoice, InvoiceDetail, ProviderState } from '@/types/invoice'

const { t } = useI18n()
const loading = ref(false)
const invoices = ref<Invoice[]>([])
const currentFilter = ref('')
const emailFilter = ref('')
const voidPendingFilter = ref(false)
const voidApproveTarget = ref<Invoice | null>(null)
const voidRejectTarget = ref<Invoice | null>(null)
const detailId = ref<number | null>(null)
const uploadTargetId = ref<number | null>(null)
const uploadReplace = ref(false)
const rejectTargetId = ref<number | null>(null)
const voidTargetId = ref<number | null>(null)
const voidIssued = ref(false)
const voidWillAutoReverse = ref(false)
const markIssuedTargetId = ref<number | null>(null)
const approveTarget = ref<Invoice | null>(null)
const downloadingId = ref<number | null>(null)
const pagination = reactive({ page: 1, page_size: 20, total: 0 })

const statusFilters = computed(() => [
  { value: '', label: t('admin.invoices.filters.all') },
  { value: 'pending', label: t('invoices.status.pending') },
  { value: 'approved', label: t('invoices.status.approved') },
  { value: 'issued', label: t('invoices.status.issued') },
  { value: 'rejected', label: t('invoices.status.rejected') },
  { value: 'voided', label: t('invoices.status.voided') },
])

// 自动开票/红冲进行中的瞬态子状态——列表里只要还有这类发票，就轮询刷新，
// 让管理员不用手动刷新就能看到「开票排队中 → 开票中 → 已开票」的推进。
const transientProviderStates: ProviderState[] = [
  'queued',
  'issuing',
  'reverse_pending',
  'reversing',
]
const POLL_INTERVAL_MS = 10000
let pollTimer: ReturnType<typeof setTimeout> | null = null

function hasTransitioningInvoice(): boolean {
  return invoices.value.some((inv) => {
    if (inv.status === 'rejected' || inv.status === 'voided') return false
    const ps = inv.provider_state
    return ps != null && transientProviderStates.includes(ps)
  })
}

function scheduleNextPoll() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  if (hasTransitioningInvoice()) {
    pollTimer = setTimeout(() => fetchInvoices({ silent: true }), POLL_INTERVAL_MS)
  }
}

async function fetchInvoices(opts?: { silent?: boolean }) {
  const silent = opts?.silent === true
  if (!silent) loading.value = true
  try {
    const res = await adminInvoiceAPI.list({
      page: pagination.page,
      page_size: pagination.page_size,
      status: currentFilter.value || undefined,
      email: emailFilter.value.trim() || undefined,
      void_pending: voidPendingFilter.value || undefined,
    })
    invoices.value = res.data.items || []
    pagination.total = res.data.total
  } catch (err) {
    // 静默轮询失败不弹窗打扰、也不清空已有数据，留到下一轮重试。
    if (!silent) {
      alert(extractApiErrorMessage(err))
      invoices.value = []
      pagination.total = 0
    }
  } finally {
    if (!silent) loading.value = false
    scheduleNextPoll()
  }
}

async function adminFetcher(id: number): Promise<{ data: InvoiceDetail }> {
  return adminInvoiceAPI.detail(id)
}

// v3：审批改成弹窗形式，让管理员选择票种 + 开票渠道。
function approve(inv: Invoice) {
  approveTarget.value = inv
}

// v3：自动开票/红冲子状态的可读标签。
// 失败的详细错误不展开拼到标签里，改放 HelpTooltip（点感叹号查看），
// 避免单元格被超长 HTTP 报错撑爆。
//
// 主状态进入终态（rejected/voided）后，订单已经释放，provider_state 即使为 failed
// 也是历史值，不应再让管理员看到"开票失败"以为还有重试入口。
function providerStateLabel(inv: Invoice): string {
  if (inv.status === 'rejected' || inv.status === 'voided') {
    return ''
  }
  switch (inv.provider_state) {
    case 'queued':
      return '开票排队中'
    case 'issuing':
      return '开票中...'
    case 'failed':
      return '开票失败'
    case 'reverse_pending':
      return '红冲排队中'
    case 'reversing':
      return '红冲中...'
    case 'reverse_failed':
      return '红冲失败'
    default:
      return ''
  }
}

function providerStateColor(inv: Invoice): string {
  switch (inv.provider_state) {
    case 'failed':
    case 'reverse_failed':
      return 'text-red-600 dark:text-red-400'
    case 'queued':
    case 'issuing':
    case 'reverse_pending':
    case 'reversing':
      return 'text-amber-600 dark:text-amber-400'
    default:
      return 'text-gray-500'
  }
}

async function retryIssue(id: number) {
  try {
    await adminInvoiceAPI.retryIssue(id)
    fetchInvoices()
  } catch (err) {
    alert(extractApiErrorMessage(err))
  }
}

async function confirmTransfer(id: number) {
  if (!window.confirm('确认该对公转账发票已收到款项？确认后即可审批开票。')) return
  try {
    await adminInvoiceAPI.confirmTransfer(id)
    fetchInvoices()
  } catch (err) {
    alert(extractApiErrorMessage(err))
  }
}

async function retryReverse(id: number) {
  try {
    await adminInvoiceAPI.retryReverse(id)
    fetchInvoices()
  } catch (err) {
    alert(extractApiErrorMessage(err))
  }
}

async function markReversed(inv: Invoice) {
  const redNo = window.prompt(`请输入在财云通后台手工开具的红票号码（关联发票 ${inv.invoice_no}）：`)
  if (redNo === null) return // 取消
  try {
    await adminInvoiceAPI.markReversed(inv.id, redNo.trim())
    fetchInvoices()
  } catch (err) {
    alert(extractApiErrorMessage(err))
  }
}

function openUpload(id: number, replace: boolean) {
  uploadReplace.value = replace
  uploadTargetId.value = id
}

function openVoid(inv: Invoice, isIssued: boolean) {
  voidIssued.value = isIssued
  // 当 issued + 非 manual 渠道时，作废会触发真红冲（service 端 AdminVoid 自动分支）
  voidWillAutoReverse.value = isIssued && !!inv.provider && inv.provider !== 'manual'
  voidTargetId.value = inv.id
}

function onActionDone() {
  fetchInvoices()
}

async function downloadAdminPdf(id: number) {
  if (downloadingId.value) return
  downloadingId.value = id
  try {
    await adminInvoiceAPI.downloadPdf(id)
  } catch (err) {
    alert(extractApiErrorMessage(err))
  } finally {
    downloadingId.value = null
  }
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

onMounted(() => fetchInvoices())
onUnmounted(() => {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
})
</script>

<style scoped>
.action-btn {
  @apply inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium;
}
</style>
