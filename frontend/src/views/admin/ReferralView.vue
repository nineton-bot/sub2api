<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header -->
      <div class="flex items-start justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('admin.referral.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.referral.description') }}
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

      <!-- Status banner -->
      <div v-if="overview" class="card p-4">
        <div class="flex flex-wrap items-center gap-x-6 gap-y-2">
          <div class="flex items-center gap-2">
            <span
              class="inline-block h-2.5 w-2.5 rounded-full"
              :class="overview.enabled ? 'bg-green-500' : 'bg-gray-400'"
            ></span>
            <span class="text-sm font-medium">
              {{
                overview.enabled
                  ? t('admin.referral.statusEnabled')
                  : t('admin.referral.statusDisabled')
              }}
            </span>
          </div>
          <div class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.referral.currentRate') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{ (overview.commission_rate * 100).toFixed(1) }}%
            </span>
          </div>
          <div class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.referral.currentBonus') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              ¥{{ overview.referee_bonus_amount.toFixed(2) }}
            </span>
          </div>
          <div v-if="!overview.enabled" class="basis-full text-xs text-amber-600 dark:text-amber-400">
            {{ t('admin.referral.enableHint') }}
          </div>
        </div>
      </div>

      <!-- Stat cards -->
      <div v-if="overview" class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
              <Icon name="users" size="md" class="text-blue-600 dark:text-blue-400" :stroke-width="2" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.statInvited') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ overview.total_invited_users }}
              </p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-emerald-100 p-2 dark:bg-emerald-900/30">
              <Icon name="chart" size="md" class="text-emerald-600 dark:text-emerald-400" :stroke-width="2" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.statTotalReleased') }}
              </p>
              <p class="text-xl font-bold text-emerald-600 dark:text-emerald-400">
                ¥{{ overview.total_released.toFixed(2) }}
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
                {{ t('admin.referral.statTotalPending') }}
              </p>
              <p class="text-xl font-bold text-amber-600 dark:text-amber-400">
                ¥{{ overview.total_pending.toFixed(2) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.statTotalGross') }}: ¥{{
                  overview.total_gross_commission.toFixed(2)
                }}
              </p>
            </div>
          </div>
        </div>
        <div class="card p-4">
          <div class="flex items-center gap-3">
            <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
              <Icon name="gift" size="md" class="text-purple-600 dark:text-purple-400" :stroke-width="2" />
            </div>
            <div>
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.statRefereeBonusGranted') }}
              </p>
              <p class="text-xl font-bold text-gray-900 dark:text-white">
                {{ overview.referee_bonus_granted }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.referral.statRefereeBonusPending') }}: {{ overview.referee_bonus_pending }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Top Referrers -->
      <div class="card">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.referral.topReferrers') }}
          </h2>
          <div class="flex gap-2">
            <button
              @click="setSort('commission')"
              class="btn"
              :class="sortBy === 'commission' ? 'btn-primary' : 'btn-secondary'"
            >
              {{ t('admin.referral.sortByCommission') }}
            </button>
            <button
              @click="setSort('count')"
              class="btn"
              :class="sortBy === 'count' ? 'btn-primary' : 'btn-secondary'"
            >
              {{ t('admin.referral.sortByCount') }}
            </button>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead class="border-b border-gray-100 bg-gray-50 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-6 py-3">{{ t('admin.referral.colUser') }}</th>
                <th class="px-6 py-3">{{ t('admin.referral.colInvited') }}</th>
                <th class="px-6 py-3">{{ t('admin.referral.colGross') }}</th>
                <th class="px-6 py-3">{{ t('admin.referral.colReleased') }}</th>
                <th class="px-6 py-3 text-right">{{ t('admin.referral.colActions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="loading && rankings.length === 0">
                <td colspan="5" class="px-6 py-10 text-center text-gray-500 dark:text-gray-400">
                  <LoadingSpinner />
                </td>
              </tr>
              <tr v-else-if="!loading && rankings.length === 0">
                <td colspan="5" class="px-6 py-10 text-center text-gray-500 dark:text-gray-400">
                  {{ t('admin.referral.noReferrers') }}
                </td>
              </tr>
              <tr
                v-else
                v-for="item in rankings"
                :key="item.user_id"
                class="hover:bg-gray-50 dark:hover:bg-dark-800/50"
              >
                <td class="px-6 py-4">
                  <div class="font-medium text-gray-900 dark:text-white">
                    {{ item.username || '(no name)' }}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ item.email }}</div>
                </td>
                <td class="px-6 py-4 text-gray-900 dark:text-white">{{ item.invited_count }}</td>
                <td class="px-6 py-4 text-gray-900 dark:text-white">
                  ¥{{ item.gross_commission.toFixed(2) }}
                </td>
                <td class="px-6 py-4 text-emerald-600 dark:text-emerald-400">
                  ¥{{ item.released_commission.toFixed(2) }}
                </td>
                <td class="px-6 py-4 text-right">
                  <button
                    @click="openDrilldown(item)"
                    class="text-primary-600 hover:text-primary-500"
                  >
                    {{ t('admin.referral.viewDetail') }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Drilldown Dialog -->
    <BaseDialog
      :show="drilldownOpen"
      :title="drilldownTitle"
      width="wide"
      @close="handleDialogClose"
    >
      <div class="min-h-[200px]">
        <div v-if="drilldownLoading" class="flex items-center justify-center py-10">
          <LoadingSpinner />
        </div>
        <div
          v-else-if="drilldownRows.length === 0"
          class="py-10 text-center text-sm text-gray-500 dark:text-gray-400"
        >
          {{ t('admin.referral.noReferees') }}
        </div>
        <div v-else>
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead class="border-b border-gray-100 bg-gray-50 text-left text-xs uppercase text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
                <tr>
                  <th class="px-4 py-2">{{ t('admin.referral.colReferee') }}</th>
                  <th class="px-4 py-2">{{ t('admin.referral.colJoinedAt') }}</th>
                  <th class="px-4 py-2">{{ t('admin.referral.colOrderCount') }}</th>
                  <th class="px-4 py-2">{{ t('admin.referral.colGross') }}</th>
                  <th class="px-4 py-2">{{ t('admin.referral.colReleased') }}</th>
                  <th class="px-4 py-2">{{ t('admin.referral.colBonusGranted') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr
                  v-for="row in drilldownRows"
                  :key="row.referee_id"
                  class="hover:bg-gray-50 dark:hover:bg-dark-800/50"
                >
                  <td class="px-4 py-2">
                    <div class="font-medium text-gray-900 dark:text-white">
                      {{ row.username || '(no name)' }}
                    </div>
                    <div class="text-xs text-gray-500 dark:text-gray-400">{{ row.email }}</div>
                  </td>
                  <td class="px-4 py-2 text-gray-600 dark:text-gray-300">
                    {{ formatDateTime(row.joined_at) }}
                  </td>
                  <td class="px-4 py-2 text-gray-900 dark:text-white">{{ row.order_count }}</td>
                  <td class="px-4 py-2 text-gray-900 dark:text-white">
                    ¥{{ row.gross_commission.toFixed(2) }}
                  </td>
                  <td class="px-4 py-2 text-emerald-600 dark:text-emerald-400">
                    ¥{{ row.released_commission.toFixed(2) }}
                  </td>
                  <td class="px-4 py-2">
                    <span
                      class="badge"
                      :class="row.bonus_granted ? 'badge-success' : 'badge-warning'"
                    >
                      {{
                        row.bonus_granted
                          ? t('admin.referral.bonusGranted')
                          : t('admin.referral.bonusPending')
                      }}
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="drilldownPagination.total > 0" class="mt-4">
            <Pagination
              :page="drilldownPagination.page"
              :total="drilldownPagination.total"
              :page-size="drilldownPagination.pageSize"
              @update:page="handleDrilldownPageChange"
              @update:pageSize="handleDrilldownPageSizeChange"
            />
          </div>
        </div>
      </div>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type {
  GlobalReferralStats,
  ReferrerRank,
  RefereeDrilldown
} from '@/api/admin/referral'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const overview = ref<GlobalReferralStats | null>(null)
const rankings = ref<ReferrerRank[]>([])
const sortBy = ref<'commission' | 'count'>('commission')

const drilldownOpen = ref(false)
const drilldownLoading = ref(false)
const drilldownRows = ref<RefereeDrilldown[]>([])
const drilldownReferrer = ref<ReferrerRank | null>(null)
const drilldownPagination = reactive({
  page: 1,
  pageSize: getPersistedPageSize(),
  total: 0
})

const drilldownTitle = computed(() =>
  t('admin.referral.drilldownTitle', {
    name:
      drilldownReferrer.value?.username ||
      drilldownReferrer.value?.email ||
      '...'
  })
)

function formatDateTime(s: string): string {
  if (!s) return ''
  try {
    return new Date(s).toLocaleString()
  } catch {
    return s
  }
}

async function loadOverview() {
  try {
    overview.value = await adminAPI.referral.getOverview()
  } catch (err) {
    appStore.showError(t('admin.referral.loadFailed'))
    console.error('loadOverview:', err)
  }
}

async function loadRankings() {
  try {
    rankings.value = await adminAPI.referral.listTopReferrers(sortBy.value, 20)
  } catch (err) {
    appStore.showError(t('admin.referral.loadFailed'))
    console.error('loadRankings:', err)
  }
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadOverview(), loadRankings()])
  } finally {
    loading.value = false
  }
}

async function setSort(next: 'commission' | 'count') {
  if (sortBy.value === next) return
  sortBy.value = next
  loading.value = true
  try {
    await loadRankings()
  } finally {
    loading.value = false
  }
}

async function loadDrilldownPage() {
  if (!drilldownReferrer.value) return
  drilldownLoading.value = true
  try {
    const resp = await adminAPI.referral.getReferrerDrilldown(
      drilldownReferrer.value.user_id,
      drilldownPagination.page,
      drilldownPagination.pageSize
    )
    drilldownRows.value = resp.items
    drilldownPagination.total = resp.total
  } catch (err) {
    appStore.showError(t('admin.referral.loadFailed'))
    console.error('openDrilldown:', err)
  } finally {
    drilldownLoading.value = false
  }
}

async function openDrilldown(item: ReferrerRank) {
  drilldownReferrer.value = item
  drilldownOpen.value = true
  drilldownPagination.page = 1
  drilldownPagination.total = 0
  drilldownRows.value = []
  await loadDrilldownPage()
}

function handleDrilldownPageChange(next: number) {
  drilldownPagination.page = next
  loadDrilldownPage()
}

function handleDrilldownPageSizeChange(next: number) {
  drilldownPagination.pageSize = next
  drilldownPagination.page = 1
  loadDrilldownPage()
}

function closeDrilldown() {
  drilldownReferrer.value = null
  drilldownRows.value = []
  drilldownPagination.total = 0
  drilldownPagination.page = 1
}

function handleDialogClose() {
  drilldownOpen.value = false
  closeDrilldown()
}

onMounted(loadAll)
</script>
