<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex items-start justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('referral.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('referral.description') }}
          </p>
        </div>
        <button
          @click="loadAll"
          :disabled="loading"
          class="btn btn-secondary"
          :title="t('common.refresh')"
        >
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>

      <!-- Disabled banner -->
      <div
        v-if="!referralEnabled"
        class="card border border-amber-200 bg-amber-50 p-4 dark:border-amber-700/50 dark:bg-amber-900/20"
      >
        <div class="flex items-start gap-3">
          <Icon
            name="infoCircle"
            size="md"
            class="mt-0.5 text-amber-600 dark:text-amber-400"
          />
          <p class="text-sm text-amber-700 dark:text-amber-300">
            {{ t('referral.disabled') }}
          </p>
        </div>
      </div>

      <!-- Invite link hero -->
      <div class="card p-6">
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-[1fr,auto]">
          <div class="space-y-4">
            <div>
              <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('referral.inviteLink') }}
              </p>
              <div class="mt-2 flex gap-2">
                <div
                  class="flex flex-1 items-center gap-2 overflow-hidden rounded-md border border-gray-200 bg-gray-50 px-3 py-2 font-mono text-sm text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-white"
                >
                  <Icon name="link" size="sm" class="shrink-0 text-gray-400" />
                  <span class="truncate">{{ inviteLink || '—' }}</span>
                </div>
                <button
                  @click="copy(inviteLink, 'link')"
                  :disabled="!inviteLink"
                  class="btn btn-primary whitespace-nowrap"
                >
                  <Icon name="copy" size="sm" class="mr-1" />
                  {{ copiedField === 'link' ? t('referral.copied') : t('referral.copyLink') }}
                </button>
              </div>
            </div>

            <div>
              <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
                {{ t('referral.inviteCode') }}
              </p>
              <div class="mt-2 flex gap-2">
                <div
                  class="flex flex-1 items-center gap-2 overflow-hidden rounded-md border border-gray-200 bg-gray-50 px-3 py-2 font-mono text-sm uppercase tracking-wider text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-white"
                >
                  <span class="truncate">{{ stats?.invite_code || '—' }}</span>
                </div>
                <button
                  @click="copy(stats?.invite_code || '', 'code')"
                  :disabled="!stats?.invite_code"
                  class="btn btn-secondary whitespace-nowrap"
                >
                  <Icon name="copy" size="sm" class="mr-1" />
                  {{ copiedField === 'code' ? t('referral.copied') : t('referral.copyCode') }}
                </button>
              </div>
            </div>

            <div class="flex flex-wrap gap-6 pt-2 text-sm">
              <div>
                <span class="text-gray-500 dark:text-gray-400">
                  {{ t('referral.commissionRate') }}:
                </span>
                <span class="ml-1 font-medium text-gray-900 dark:text-white">
                  {{ ((stats?.commission_rate ?? 0) * 100).toFixed(1) }}%
                </span>
              </div>
              <div>
                <span class="text-gray-500 dark:text-gray-400">
                  {{ t('referral.bonusAmount') }}:
                </span>
                <span class="ml-1 font-medium text-gray-900 dark:text-white">
                  ¥{{ (stats?.referee_bonus_amount ?? 0).toFixed(2) }}
                </span>
              </div>
            </div>
          </div>

          <!-- QR Code -->
          <div v-if="inviteLink" class="flex flex-col items-center gap-2">
            <div class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700">
              <canvas ref="qrCanvas" class="block"></canvas>
            </div>
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('referral.qrTitle') }}</p>
            <button @click="downloadQr" class="text-xs text-primary-600 hover:text-primary-500">
              {{ t('referral.downloadQr') }}
            </button>
          </div>
        </div>
      </div>

      <!-- Stat cards -->
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
              <Icon name="chart" size="md" class="text-blue-600 dark:text-blue-400" :stroke-width="2" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('referral.statGross') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                ¥{{ (stats?.gross_commission ?? 0).toFixed(2) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('referral.statGrossHint') }}
              </p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-emerald-100 p-2 dark:bg-emerald-900/30">
              <Icon name="dollar" size="md" class="text-emerald-600 dark:text-emerald-400" :stroke-width="2" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('referral.statReleased') }}
              </p>
              <p class="text-xl font-bold text-emerald-600 dark:text-emerald-400">
                ¥{{ (stats?.released_commission ?? 0).toFixed(2) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('referral.statReleasedHint') }}
              </p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30">
              <Icon name="clock" size="md" class="text-amber-600 dark:text-amber-400" :stroke-width="2" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('referral.statPending') }}
              </p>
              <p class="text-xl font-bold text-amber-600 dark:text-amber-400">
                ¥{{ (stats?.pending_commission ?? 0).toFixed(2) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('referral.statPendingHint') }}
              </p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
              <Icon name="users" size="md" class="text-purple-600 dark:text-purple-400" :stroke-width="2" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('referral.statInvited') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ stats?.invited_count ?? 0 }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('referral.statInvitedHint') }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- How it works -->
      <div class="card p-5">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ t('referral.howItWorksTitle') }}
        </h2>
        <ul class="mt-3 space-y-1.5 text-sm text-gray-600 dark:text-gray-300">
          <li class="flex gap-2">
            <span class="text-primary-600">•</span>
            <span>{{ t('referral.howItWorksLine1') }}</span>
          </li>
          <li class="flex gap-2">
            <span class="text-primary-600">•</span>
            <span>
              {{ t('referral.howItWorksLine2', { rate: ((stats?.commission_rate ?? 0) * 100).toFixed(1) }) }}
            </span>
          </li>
          <li class="flex gap-2">
            <span class="text-primary-600">•</span>
            <span>{{ t('referral.howItWorksLine3') }}</span>
          </li>
          <li class="flex gap-2">
            <span class="text-primary-600">•</span>
            <span>{{ t('referral.howItWorksLine4') }}</span>
          </li>
        </ul>
      </div>

      <!-- Commission log -->
      <div class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('referral.historyTitle') }}
          </h2>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead class="border-b border-gray-100 bg-gray-50 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-6 py-3">{{ t('referral.colReferee') }}</th>
                <th class="px-6 py-3">{{ t('referral.colType') }}</th>
                <th class="px-6 py-3">{{ t('referral.colAmount') }}</th>
                <th class="px-6 py-3">{{ t('referral.colGross') }}</th>
                <th class="px-6 py-3">{{ t('referral.colReleased') }}</th>
                <th class="px-6 py-3">{{ t('referral.colStatus') }}</th>
                <th class="px-6 py-3">{{ t('referral.colCreatedAt') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="listLoading && logs.length === 0">
                <td colspan="7" class="px-6 py-10 text-center text-gray-500 dark:text-gray-400">
                  <LoadingSpinner />
                </td>
              </tr>
              <tr v-else-if="!listLoading && logs.length === 0">
                <td colspan="7" class="px-6 py-10 text-center text-gray-500 dark:text-gray-400">
                  {{ t('referral.empty') }}
                </td>
              </tr>
              <tr
                v-else
                v-for="row in logs"
                :key="row.id"
                class="hover:bg-gray-50 dark:hover:bg-dark-800/50"
              >
                <td class="px-6 py-3 text-gray-900 dark:text-white">
                  {{ maskEmail(row.referee_email) }}
                </td>
                <td class="px-6 py-3">
                  <span
                    class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
                    :class="
                      row.source_type === 'subscription'
                        ? 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-200'
                        : 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-200'
                    "
                  >
                    {{
                      row.source_type === 'subscription'
                        ? t('referral.typeSubscription')
                        : t('referral.typeRecharge')
                    }}
                  </span>
                </td>
                <td class="px-6 py-3 text-gray-900 dark:text-white">
                  ¥{{ row.source_amount.toFixed(2) }}
                </td>
                <td class="px-6 py-3 text-gray-900 dark:text-white">
                  ¥{{ row.gross_commission.toFixed(2) }}
                </td>
                <td class="px-6 py-3 text-emerald-600 dark:text-emerald-400">
                  ¥{{ row.released_commission.toFixed(2) }}
                </td>
                <td class="px-6 py-3">
                  <span class="badge" :class="statusBadgeClass(row.status)">
                    {{ statusLabel(row.status) }}
                  </span>
                </td>
                <td class="px-6 py-3 text-gray-500 dark:text-gray-400">
                  {{ formatDateTime(row.created_at) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="pagination.total > 0" class="px-6 py-4">
          <Pagination
            :page="pagination.page"
            :total="pagination.total"
            :page-size="pagination.pageSize"
            @update:page="handlePageChange"
            @update:pageSize="handlePageSizeChange"
          />
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { referralAPI } from '@/api/referral'
import type { ReferralStats, CommissionLog } from '@/api/referral'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const listLoading = ref(false)
const stats = ref<ReferralStats | null>(null)
const logs = ref<CommissionLog[]>([])
const copiedField = ref<'' | 'link' | 'code'>('')
const qrCanvas = ref<HTMLCanvasElement | null>(null)

const pagination = reactive({
  page: 1,
  pageSize: getPersistedPageSize(),
  total: 0
})

const referralEnabled = computed(
  () => appStore.cachedPublicSettings?.referral_enabled === true
)

const inviteLink = computed(() => {
  if (!stats.value?.invite_code) return ''
  const origin =
    typeof window !== 'undefined' && window.location ? window.location.origin : ''
  return `${origin}/register?ref=${stats.value.invite_code}`
})

function formatDateTime(s: string): string {
  if (!s) return ''
  try {
    return new Date(s).toLocaleString()
  } catch {
    return s
  }
}

function maskEmail(email: string): string {
  if (!email) return '—'
  const [user, domain] = email.split('@')
  if (!domain) return email
  if (user.length <= 2) return `${user[0] || '*'}***@${domain}`
  return `${user.slice(0, 2)}***@${domain}`
}

function statusLabel(status: string): string {
  switch (status) {
    case 'fully_released':
      return t('referral.statusFullyReleased')
    case 'reversed':
      return t('referral.statusReversed')
    case 'partial_reversed':
      return t('referral.statusPartialReversed')
    case 'accruing':
    default:
      return t('referral.statusAccruing')
  }
}

function statusBadgeClass(status: string): string {
  switch (status) {
    case 'fully_released':
      return 'badge-success'
    case 'reversed':
    case 'partial_reversed':
      return 'badge-danger'
    case 'accruing':
    default:
      return 'badge-warning'
  }
}

async function copy(value: string, field: 'link' | 'code') {
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
    copiedField.value = field
    setTimeout(() => {
      if (copiedField.value === field) copiedField.value = ''
    }, 2000)
  } catch (err) {
    console.error('copy failed:', err)
    appStore.showError(t('common.copyFailed') || 'Copy failed')
  }
}

async function renderQr() {
  if (!qrCanvas.value || !inviteLink.value) return
  try {
    await QRCode.toCanvas(qrCanvas.value, inviteLink.value, {
      width: 160,
      margin: 1,
      color: { dark: '#111827', light: '#ffffff' }
    })
  } catch (err) {
    console.error('QR render failed:', err)
  }
}

function downloadQr() {
  if (!qrCanvas.value) return
  try {
    const url = qrCanvas.value.toDataURL('image/png')
    const link = document.createElement('a')
    link.href = url
    link.download = `referral-${stats.value?.invite_code || 'qr'}.png`
    link.click()
  } catch (err) {
    console.error('download QR failed:', err)
  }
}

async function loadStats() {
  if (!referralEnabled.value) return
  try {
    // Ensure invite code exists (idempotent) before querying overview
    await referralAPI.ensureInviteCode().catch(() => {})
    stats.value = await referralAPI.getOverview()
  } catch (err) {
    appStore.showError(t('referral.loadFailed'))
    console.error('loadStats:', err)
  }
}

async function loadLogs() {
  if (!referralEnabled.value) return
  listLoading.value = true
  try {
    const resp = await referralAPI.listCommissions(pagination.page, pagination.pageSize)
    logs.value = resp.items
    pagination.total = resp.total
  } catch (err) {
    appStore.showError(t('referral.loadFailed'))
    console.error('loadLogs:', err)
  } finally {
    listLoading.value = false
  }
}

async function loadAll() {
  // 功能未启用时跳过所有接口调用，只展示 disabled banner。
  // 避免用户在管理员关闭开关后仍持续打 /user/referral/overview 等接口。
  if (!referralEnabled.value) {
    return
  }
  loading.value = true
  try {
    await Promise.all([loadStats(), loadLogs()])
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  loadLogs()
}

function handlePageSizeChange(size: number) {
  pagination.pageSize = size
  pagination.page = 1
  loadLogs()
}

watch(inviteLink, async () => {
  await nextTick()
  renderQr()
})

onMounted(async () => {
  await loadAll()
  await nextTick()
  renderQr()
})
</script>
