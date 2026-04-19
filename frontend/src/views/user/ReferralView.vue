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
            {{
              disabledReason === 'not_enabled_for_user' || disabledReason === 'disabled_for_user'
                ? t('referral.disabledForUser')
                : t('referral.disabled')
            }}
          </p>
        </div>
      </div>

      <!-- Invite link hero -->
      <div class="card p-6">
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-[1fr,auto]">
          <div class="space-y-4">
            <div>
              <div class="flex items-center justify-between">
                <p class="text-sm font-medium text-gray-500 dark:text-gray-400">
                  {{ t('referral.inviteLink') }}
                </p>
                <label
                  class="flex cursor-pointer items-center gap-2 text-xs text-gray-600 dark:text-gray-300"
                  :title="t('referral.stealthModeHint')"
                >
                  <input
                    type="checkbox"
                    v-model="stealthMode"
                    class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                  />
                  <span>{{ t('referral.stealthModeLabel') }}</span>
                </label>
              </div>
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
              <p v-if="stealthMode" class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('referral.stealthModeActive') }}
              </p>
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
                {{ t('referral.statUsable') }}
              </p>
              <p class="text-xl font-bold text-emerald-600 dark:text-emerald-400">
                ¥{{ (stats?.usable_commission ?? 0).toFixed(2) }}
              </p>
              <p
                class="text-xs text-gray-500 dark:text-gray-400"
                :title="t('referral.statUsableTooltip')"
              >
                {{ t('referral.statUsableHint') }}
              </p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-sky-100 p-2 dark:bg-sky-900/30">
              <Icon name="checkCircle" size="md" class="text-sky-600 dark:text-sky-400" :stroke-width="2" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('referral.statReleased') }}
              </p>
              <p class="text-xl font-bold text-sky-600 dark:text-sky-400">
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

      <!-- Usable commission action bar (V2) -->
      <div
        v-if="(stats?.usable_commission ?? 0) > 0"
        class="card flex flex-wrap items-center justify-between gap-3 p-4"
      >
        <div class="text-sm">
          <p class="font-medium text-gray-900 dark:text-white">
            {{ t('referral.useUsableTitle') }}
          </p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('referral.useUsableHint') }}
          </p>
        </div>
        <div class="flex gap-2">
          <button
            class="btn btn-primary"
            @click="showTransferModal = true"
          >
            {{ t('referral.transfer.action') }}
          </button>
          <button
            v-if="stats?.withdrawal_allowed"
            class="btn btn-secondary"
            @click="appStore.showInfo(t('referral.withdrawalComingSoon'))"
          >
            {{ t('referral.withdraw.action') }}
          </button>
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
                <th class="w-10 px-2 py-3"></th>
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
                <td colspan="8" class="px-6 py-10 text-center text-gray-500 dark:text-gray-400">
                  <LoadingSpinner />
                </td>
              </tr>
              <tr v-else-if="!listLoading && logs.length === 0">
                <td colspan="8" class="px-6 py-10 text-center text-gray-500 dark:text-gray-400">
                  {{ t('referral.empty') }}
                </td>
              </tr>
              <template v-else v-for="row in logs" :key="row.id">
                <tr class="hover:bg-gray-50 dark:hover:bg-dark-800/50">
                  <td class="px-2 py-3 text-center">
                    <button
                      v-if="row.released_commission > 0"
                      type="button"
                      class="inline-flex h-6 w-6 items-center justify-center rounded text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                      :aria-expanded="expandedRowIds.has(row.id)"
                      :title="t('referral.releaseLog.toggle')"
                      @click="toggleRelease(row.id)"
                    >
                      <Icon
                        :name="expandedRowIds.has(row.id) ? 'chevronDown' : 'chevronRight'"
                        size="sm"
                        :stroke-width="2"
                      />
                    </button>
                  </td>
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
                <tr v-if="expandedRowIds.has(row.id)" class="bg-gray-50/70 dark:bg-dark-800/40">
                  <td></td>
                  <td colspan="7" class="px-6 py-3">
                    <div class="mb-2 text-xs font-semibold text-gray-500 dark:text-gray-400">
                      {{ t('referral.releaseLog.title') }}
                    </div>
                    <div
                      v-if="releaseLogsState[row.id] === 'loading'"
                      class="py-2 text-xs text-gray-500"
                    >
                      <LoadingSpinner />
                    </div>
                    <div
                      v-else-if="releaseLogsState[row.id] === 'error'"
                      class="py-2 text-xs text-red-500"
                    >
                      {{ t('referral.releaseLog.loadFailed') }}
                    </div>
                    <div
                      v-else-if="(releaseLogsByCommission[row.id]?.length ?? 0) === 0"
                      class="py-2 text-xs text-gray-500"
                    >
                      {{ t('referral.releaseLog.empty') }}
                    </div>
                    <ul v-else class="space-y-1.5">
                      <li
                        v-for="(agg, idx) in releaseLogsByCommission[row.id]"
                        :key="idx"
                        class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs"
                      >
                        <span class="font-mono text-gray-600 dark:text-gray-300">
                          {{ agg.day }}
                        </span>
                        <span class="inline-flex items-center rounded bg-gray-200 px-2 py-0.5 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
                          {{ t(`referral.releaseLog.trigger.${agg.trigger_type}`) }}
                        </span>
                        <span
                          class="font-medium"
                          :class="
                            agg.total_amount >= 0
                              ? 'text-emerald-600 dark:text-emerald-400'
                              : 'text-red-600 dark:text-red-400'
                          "
                        >
                          {{ agg.total_amount >= 0 ? '+' : '' }}¥{{ agg.total_amount.toFixed(2) }}
                        </span>
                        <span class="text-gray-500 dark:text-gray-400">
                          ×{{ agg.event_count }}
                        </span>
                      </li>
                    </ul>
                  </td>
                </tr>
              </template>
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

    <TransferToBalanceModal
      :show="showTransferModal"
      :usable="stats?.usable_commission ?? 0"
      @close="showTransferModal = false"
      @success="onTransferSuccess"
    />
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
import TransferToBalanceModal from '@/components/user/referral/TransferToBalanceModal.vue'
import { referralAPI } from '@/api/referral'
import type {
  ReferralStats,
  CommissionLog,
  EligibilityResponse,
  ReleaseLogDayAggregate
} from '@/api/referral'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(false)
const listLoading = ref(false)
const stats = ref<ReferralStats | null>(null)
const logs = ref<CommissionLog[]>([])
const copiedField = ref<'' | 'link' | 'code'>('')
const qrCanvas = ref<HTMLCanvasElement | null>(null)
const showTransferModal = ref(false)

function onTransferSuccess(newStats: ReferralStats) {
  stats.value = newStats
  authStore.refreshUser().catch(() => { /* non-fatal */ })
}

// 释放日志（按天聚合）展开状态 —— commission_id 为键
const expandedRowIds = ref<Set<number>>(new Set())
const releaseLogsByCommission = ref<Record<number, ReleaseLogDayAggregate[]>>({})
const releaseLogsState = ref<Record<number, 'loading' | 'error' | 'loaded'>>({})

async function toggleRelease(commissionId: number) {
  if (expandedRowIds.value.has(commissionId)) {
    expandedRowIds.value.delete(commissionId)
    expandedRowIds.value = new Set(expandedRowIds.value)
    return
  }
  expandedRowIds.value.add(commissionId)
  expandedRowIds.value = new Set(expandedRowIds.value)
  // 已加载过则直接复用
  if (releaseLogsState.value[commissionId] === 'loaded') return
  releaseLogsState.value = { ...releaseLogsState.value, [commissionId]: 'loading' }
  try {
    const resp = await referralAPI.listReleaseLogsDaily(commissionId, 1, 60)
    releaseLogsByCommission.value = {
      ...releaseLogsByCommission.value,
      [commissionId]: resp.items ?? []
    }
    releaseLogsState.value = { ...releaseLogsState.value, [commissionId]: 'loaded' }
  } catch (e) {
    console.error('load release logs failed', e)
    releaseLogsState.value = { ...releaseLogsState.value, [commissionId]: 'error' }
  }
}


// V2 访问守卫：挂载时查 per-user 可见性。
// master 开关关闭时后端直接返回 enabled=false, reason=feature_disabled；
// 单用户未开启返 reason=not_enabled_for_user。
// eligibility 为 null 表示尚未获取（loading 中）。
const eligibility = ref<EligibilityResponse | null>(null)

// 隐藏模式：链接格式切换为 /g/:code（无 ref 参数痕迹），本地 localStorage 记住偏好
const STEALTH_STORAGE_KEY = 'referral_link_stealth'
const stealthMode = ref<boolean>(
  typeof localStorage !== 'undefined' && localStorage.getItem(STEALTH_STORAGE_KEY) === '1'
)
watch(stealthMode, (v) => {
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STEALTH_STORAGE_KEY, v ? '1' : '0')
    }
  } catch {
    /* ignore quota / privacy mode errors */
  }
})

const pagination = reactive({
  page: 1,
  pageSize: getPersistedPageSize(),
  total: 0
})

// effective: 以后端 eligibility 为 SSOT（master off 时后端直接返回 feature_disabled）。
// eligibility 尚未获取（null）时，乐观展示骨架，不阻塞首次 UI。
const referralEnabled = computed(() => {
  if (eligibility.value === null) return true
  return eligibility.value.enabled === true
})

// 对本用户关闭原因（用于提示文案）：
//  - feature_disabled:      总开关关
//  - not_enabled_for_user:  全局默认关，且 admin 未显式为本用户开
//  - disabled_for_user:     admin 显式关闭本用户
const disabledReason = computed<string>(() => {
  if (eligibility.value && !eligibility.value.enabled) return eligibility.value.reason
  return ''
})

const inviteLink = computed(() => {
  if (!stats.value?.invite_code) return ''
  const origin =
    typeof window !== 'undefined' && window.location ? window.location.origin : ''
  if (stealthMode.value) {
    return `${origin}/g/${stats.value.invite_code}`
  }
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

async function loadEligibility() {
  // SSOT：后端 IsReferralVisibleForUser 已覆盖 master off / 全局默认 / 单用户 override 所有分支，
  // 前端不再短路，避免刷新时 cachedPublicSettings 异步未到导致误判"feature_disabled"。
  try {
    eligibility.value = await referralAPI.getEligibility()
    appStore.referralEligible = eligibility.value.enabled
  } catch (err) {
    // 失败时退回保守判定：不展示（避免误开）。
    console.error('loadEligibility:', err)
    eligibility.value = { enabled: false, reason: 'feature_disabled' }
    appStore.referralEligible = false
  }
}

async function loadAll() {
  await loadEligibility()
  // 功能未对本用户启用时跳过所有接口调用，只展示 disabled banner。
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
