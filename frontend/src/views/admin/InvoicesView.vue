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
          <button class="btn btn-secondary" @click="fetchInvoices">
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
              <th class="px-4 py-3 text-left text-xs font-medium uppercase text-gray-500">ID</th>
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
              <td class="px-4 py-3 font-mono text-xs">{{ inv.id }}</td>
              <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">{{ inv.user_email }}</td>
              <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">
                {{ inv.title }}
                <span class="ml-1 text-xs text-gray-500">({{ t('invoices.titleType.' + inv.title_type) }})</span>
              </td>
              <td class="px-4 py-3 text-right text-sm font-semibold text-emerald-600">¥{{ inv.amount.toFixed(2) }}</td>
              <td class="px-4 py-3"><InvoiceStatusBadge :status="inv.status" /></td>
              <td class="px-4 py-3 text-xs text-gray-600 dark:text-gray-400">{{ formatDate(inv.submitted_at) }}</td>
              <td class="px-4 py-3 text-right">
                <div class="flex flex-wrap items-center justify-end gap-1">
                  <button class="action-btn text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20" @click="detailId = inv.id">
                    {{ t('admin.invoices.actions.view') }}
                  </button>
                  <template v-if="inv.status === 'pending'">
                    <button class="action-btn text-emerald-600 hover:bg-emerald-50 dark:text-emerald-400 dark:hover:bg-emerald-900/20" @click="approve(inv)">
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
                    <button class="action-btn text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" @click="openVoid(inv.id, false)">
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
                    <button class="action-btn text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20" @click="openUpload(inv.id, true)">
                      {{ t('admin.invoices.actions.replacePdf') }}
                    </button>
                    <button class="action-btn text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" @click="openVoid(inv.id, true)">
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
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import InvoiceStatusBadge from '@/components/invoice/InvoiceStatusBadge.vue'
import InvoiceDetailDialog from '@/components/invoice/InvoiceDetailDialog.vue'
import AdminInvoiceUploadDialog from '@/components/admin/invoice/AdminInvoiceUploadDialog.vue'
import AdminInvoiceRejectDialog from '@/components/admin/invoice/AdminInvoiceRejectDialog.vue'
import AdminInvoiceVoidDialog from '@/components/admin/invoice/AdminInvoiceVoidDialog.vue'
import AdminInvoiceMarkIssuedDialog from '@/components/admin/invoice/AdminInvoiceMarkIssuedDialog.vue'
import adminInvoiceAPI from '@/api/admin/invoices'
import { extractApiErrorMessage } from "@/utils/apiError"
import type { Invoice, InvoiceDetail } from '@/types/invoice'

const { t } = useI18n()
const loading = ref(false)
const invoices = ref<Invoice[]>([])
const currentFilter = ref('')
const emailFilter = ref('')
const detailId = ref<number | null>(null)
const uploadTargetId = ref<number | null>(null)
const uploadReplace = ref(false)
const rejectTargetId = ref<number | null>(null)
const voidTargetId = ref<number | null>(null)
const voidIssued = ref(false)
const markIssuedTargetId = ref<number | null>(null)
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

async function fetchInvoices() {
  loading.value = true
  try {
    const res = await adminInvoiceAPI.list({
      page: pagination.page,
      page_size: pagination.page_size,
      status: currentFilter.value || undefined,
      email: emailFilter.value.trim() || undefined,
    })
    invoices.value = res.data.items || []
    pagination.total = res.data.total
  } catch (err) {
    alert(extractApiErrorMessage(err))
    invoices.value = []
    pagination.total = 0
  } finally {
    loading.value = false
  }
}

async function adminFetcher(id: number): Promise<{ data: InvoiceDetail }> {
  return adminInvoiceAPI.detail(id)
}

async function approve(inv: Invoice) {
  try {
    await adminInvoiceAPI.approve(inv.id)
    fetchInvoices()
  } catch (err) {
    alert(extractApiErrorMessage(err))
  }
}

function openUpload(id: number, replace: boolean) {
  uploadReplace.value = replace
  uploadTargetId.value = id
}

function openVoid(id: number, isIssued: boolean) {
  voidIssued.value = isIssued
  voidTargetId.value = id
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
</script>

<style scoped>
.action-btn {
  @apply inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium;
}
</style>
