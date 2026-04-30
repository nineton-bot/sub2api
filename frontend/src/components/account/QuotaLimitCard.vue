<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import QuotaDimensionRow from './QuotaDimensionRow.vue'
import type { QuotaThresholdType, QuotaResetMode } from '@/constants/account'

const { t } = useI18n()

const props = withDefaults(defineProps<{
<<<<<<< HEAD
  meter: 'cost' | 'requests' | null
  meterEditable?: boolean
=======
>>>>>>> upstream/main
  totalLimit: number | null
  dailyLimit: number | null
  weeklyLimit: number | null
  dailyResetMode: QuotaResetMode | null
  dailyResetHour: number | null
  weeklyResetMode: QuotaResetMode | null
  weeklyResetDay: number | null
  weeklyResetHour: number | null
  resetTimezone: string | null
<<<<<<< HEAD
}>(), {
  meterEditable: true,
=======
  quotaNotifyGlobalEnabled?: boolean
  quotaNotifyDailyEnabled?: boolean | null
  quotaNotifyDailyThreshold?: number | null
  quotaNotifyDailyThresholdType?: QuotaThresholdType | null
  quotaNotifyWeeklyEnabled?: boolean | null
  quotaNotifyWeeklyThreshold?: number | null
  quotaNotifyWeeklyThresholdType?: QuotaThresholdType | null
  quotaNotifyTotalEnabled?: boolean | null
  quotaNotifyTotalThreshold?: number | null
  quotaNotifyTotalThresholdType?: QuotaThresholdType | null
}>(), {
  quotaNotifyGlobalEnabled: false,
  quotaNotifyDailyEnabled: null,
  quotaNotifyDailyThreshold: null,
  quotaNotifyDailyThresholdType: null,
  quotaNotifyWeeklyEnabled: null,
  quotaNotifyWeeklyThreshold: null,
  quotaNotifyWeeklyThresholdType: null,
  quotaNotifyTotalEnabled: null,
  quotaNotifyTotalThreshold: null,
  quotaNotifyTotalThresholdType: null,
>>>>>>> upstream/main
})

const emit = defineEmits<{
  'update:meter': [value: 'cost' | 'requests' | null]
  'update:totalLimit': [value: number | null]
  'update:dailyLimit': [value: number | null]
  'update:weeklyLimit': [value: number | null]
  'update:dailyResetMode': [value: QuotaResetMode | null]
  'update:dailyResetHour': [value: number | null]
  'update:weeklyResetMode': [value: QuotaResetMode | null]
  'update:weeklyResetDay': [value: number | null]
  'update:weeklyResetHour': [value: number | null]
  'update:resetTimezone': [value: string | null]
  'update:quotaNotifyDailyEnabled': [value: boolean | null]
  'update:quotaNotifyDailyThreshold': [value: number | null]
  'update:quotaNotifyDailyThresholdType': [value: QuotaThresholdType | null]
  'update:quotaNotifyWeeklyEnabled': [value: boolean | null]
  'update:quotaNotifyWeeklyThreshold': [value: number | null]
  'update:quotaNotifyWeeklyThresholdType': [value: QuotaThresholdType | null]
  'update:quotaNotifyTotalEnabled': [value: boolean | null]
  'update:quotaNotifyTotalThreshold': [value: number | null]
  'update:quotaNotifyTotalThresholdType': [value: QuotaThresholdType | null]
}>()

const enabled = computed(() =>
  (props.totalLimit != null && props.totalLimit > 0) ||
  (props.dailyLimit != null && props.dailyLimit > 0) ||
  (props.weeklyLimit != null && props.weeklyLimit > 0)
)

const localEnabled = ref(enabled.value)
const collapsed = ref(false)

const meterValue = computed(() => props.meter || 'cost')

// Sync when props change externally
watch(enabled, (val) => {
  localEnabled.value = val
})

// When toggle is turned off, clear all values and expand
watch(localEnabled, (val) => {
  if (!val) {
    collapsed.value = false
    emit('update:totalLimit', null)
    emit('update:dailyLimit', null)
    emit('update:weeklyLimit', null)
    emit('update:dailyResetMode', null)
    emit('update:dailyResetHour', null)
    emit('update:weeklyResetMode', null)
    emit('update:weeklyResetDay', null)
    emit('update:weeklyResetHour', null)
    emit('update:resetTimezone', null)
    if (props.meterEditable) {
      emit('update:meter', 'cost')
    }
  }
})

// Common timezone options
const timezoneOptions = [
  'UTC', 'Asia/Shanghai', 'Asia/Tokyo', 'Asia/Seoul', 'Asia/Singapore', 'Asia/Kolkata',
  'Asia/Dubai', 'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Europe/Moscow',
  'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'America/Sao_Paulo', 'Australia/Sydney', 'Pacific/Auckland',
]

// Hours for dropdown (0-23)
const hourOptions = Array.from({ length: 24 }, (_, i) => i)

// Day of week options
const dayOptions = [
  { value: 1, key: 'monday' },
  { value: 2, key: 'tuesday' },
  { value: 3, key: 'wednesday' },
  { value: 4, key: 'thursday' },
  { value: 5, key: 'friday' },
  { value: 6, key: 'saturday' },
  { value: 0, key: 'sunday' },
]

// Precomputed hint strings for the weekly fixed mode
const weeklyFixedHint = computed(() => {
  const dayKey = dayOptions.find(d => d.value === (props.weeklyResetDay ?? 1))?.key || 'monday'
  return t('admin.accounts.quotaWeeklyLimitHintFixed', {
    day: t('admin.accounts.dayOfWeek.' + dayKey),
    hour: String(props.weeklyResetHour ?? 0).padStart(2, '0'),
    timezone: props.resetTimezone || 'UTC',
  })
})

<<<<<<< HEAD
const onDailyModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as 'rolling' | 'fixed'
  emit('update:dailyResetMode', val)
  if (val === 'fixed') {
    if (props.dailyResetHour == null) emit('update:dailyResetHour', 0)
    if (!props.resetTimezone) emit('update:resetTimezone', 'UTC')
  }
}

const onWeeklyModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as 'rolling' | 'fixed'
  emit('update:weeklyResetMode', val)
  if (val === 'fixed') {
    if (props.weeklyResetDay == null) emit('update:weeklyResetDay', 1)
    if (props.weeklyResetHour == null) emit('update:weeklyResetHour', 0)
    if (!props.resetTimezone) emit('update:resetTimezone', 'UTC')
  }
}

const isRequestMeter = computed(() => meterValue.value === 'requests')
=======
const dailyFixedHint = computed(() =>
  t('admin.accounts.quotaDailyLimitHintFixed', {
    hour: String(props.dailyResetHour ?? 0).padStart(2, '0'),
    timezone: props.resetTimezone || 'UTC',
  })
)
>>>>>>> upstream/main
</script>

<template>
  <div class="rounded-lg border border-gray-200 dark:border-dark-600">
      <!-- Header: toggle + collapse -->
      <div class="flex items-center justify-between p-4" :class="{ 'pb-0': localEnabled && !collapsed }">
        <div class="flex items-center gap-2 flex-1 cursor-pointer" @click="localEnabled && (collapsed = !collapsed)">
          <svg v-if="localEnabled" class="h-4 w-4 text-gray-400 transition-transform" :class="{ '-rotate-90': collapsed }" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
          </svg>
          <div>
            <label class="input-label mb-0 cursor-pointer">{{ t('admin.accounts.quotaLimitToggle') }}</label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.quotaLimitToggleHint') }}
            </p>
          </div>
        </div>
        <button
          type="button"
          @click="localEnabled = !localEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            localEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              localEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>

<<<<<<< HEAD
      <div v-if="localEnabled" class="space-y-3">
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaMeter') }}</label>
          <select
            :value="meterValue"
            @change="emit('update:meter', (($event.target as HTMLSelectElement).value as 'cost' | 'requests'))"
            :disabled="!props.meterEditable"
            class="input text-sm"
          >
            <option value="cost">{{ t('admin.accounts.quotaMeterCost') }}</option>
            <option value="requests">{{ t('admin.accounts.quotaMeterRequests') }}</option>
          </select>
        </div>

        <!-- 日配额 -->
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaDailyLimit') }}</label>
          <div class="relative">
            <span v-if="!isRequestMeter" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              :value="dailyLimit"
              @input="onDailyInput"
              type="number"
              min="0"
              :step="isRequestMeter ? '1' : '0.01'"
              :class="['input', isRequestMeter ? '' : 'pl-7']"
              :placeholder="t('admin.accounts.quotaLimitPlaceholder')"
            />
          </div>
          <!-- 日配额重置模式 -->
          <div class="mt-2 flex items-center gap-2">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') }}</label>
            <select
              :value="dailyResetMode || 'rolling'"
              @change="onDailyModeChange"
              class="input py-1 text-xs"
            >
              <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') }}</option>
              <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') }}</option>
            </select>
          </div>
          <!-- 固定模式：小时选择 -->
          <div v-if="dailyResetMode === 'fixed'" class="mt-2 flex items-center gap-2">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') }}</label>
            <select
              :value="dailyResetHour ?? 0"
              @change="emit('update:dailyResetHour', Number(($event.target as HTMLSelectElement).value))"
              class="input py-1 text-xs w-24"
            >
              <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') }}:00</option>
            </select>
          </div>
          <p class="input-hint">
            <template v-if="dailyResetMode === 'fixed'">
              {{ t('admin.accounts.quotaDailyLimitHintFixed', { hour: String(dailyResetHour ?? 0).padStart(2, '0'), timezone: resetTimezone || 'UTC' }) }}
            </template>
            <template v-else>
              {{ isRequestMeter ? t('admin.accounts.quotaDailyLimitHintRequests') : t('admin.accounts.quotaDailyLimitHint') }}
            </template>
          </p>
        </div>

        <!-- 周配额 -->
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaWeeklyLimit') }}</label>
          <div class="relative">
            <span v-if="!isRequestMeter" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              :value="weeklyLimit"
              @input="onWeeklyInput"
              type="number"
              min="0"
              :step="isRequestMeter ? '1' : '0.01'"
              :class="['input', isRequestMeter ? '' : 'pl-7']"
              :placeholder="t('admin.accounts.quotaLimitPlaceholder')"
            />
          </div>
          <!-- 周配额重置模式 -->
          <div class="mt-2 flex items-center gap-2">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') }}</label>
            <select
              :value="weeklyResetMode || 'rolling'"
              @change="onWeeklyModeChange"
              class="input py-1 text-xs"
            >
              <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') }}</option>
              <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') }}</option>
            </select>
          </div>
          <!-- 固定模式：星期几 + 小时 -->
          <div v-if="weeklyResetMode === 'fixed'" class="mt-2 flex items-center gap-2 flex-wrap">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaWeeklyResetDay') }}</label>
            <select
              :value="weeklyResetDay ?? 1"
              @change="emit('update:weeklyResetDay', Number(($event.target as HTMLSelectElement).value))"
              class="input py-1 text-xs w-28"
            >
              <option v-for="d in dayOptions" :key="d.value" :value="d.value">{{ t('admin.accounts.dayOfWeek.' + d.key) }}</option>
            </select>
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') }}</label>
            <select
              :value="weeklyResetHour ?? 0"
              @change="emit('update:weeklyResetHour', Number(($event.target as HTMLSelectElement).value))"
              class="input py-1 text-xs w-24"
            >
              <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') }}:00</option>
            </select>
          </div>
          <p class="input-hint">
            <template v-if="weeklyResetMode === 'fixed'">
              {{ t('admin.accounts.quotaWeeklyLimitHintFixed', { day: t('admin.accounts.dayOfWeek.' + (dayOptions.find(d => d.value === (weeklyResetDay ?? 1))?.key || 'monday')), hour: String(weeklyResetHour ?? 0).padStart(2, '0'), timezone: resetTimezone || 'UTC' }) }}
            </template>
            <template v-else>
              {{ isRequestMeter ? t('admin.accounts.quotaWeeklyLimitHintRequests') : t('admin.accounts.quotaWeeklyLimitHint') }}
            </template>
          </p>
        </div>

        <!-- 时区选择（当任一维度使用固定模式时显示） -->
        <div v-if="hasFixedMode">
          <label class="input-label">{{ t('admin.accounts.quotaResetTimezone') }}</label>
          <select
            :value="resetTimezone || 'UTC'"
            @change="emit('update:resetTimezone', ($event.target as HTMLSelectElement).value)"
            class="input text-sm"
          >
            <option v-for="tz in timezoneOptions" :key="tz" :value="tz">{{ tz }}</option>
          </select>
        </div>

        <!-- 总配额 -->
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaTotalLimit') }}</label>
          <div class="relative">
            <span v-if="!isRequestMeter" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              :value="totalLimit"
              @input="onTotalInput"
              type="number"
              min="0"
              :step="isRequestMeter ? '1' : '0.01'"
              :class="['input', isRequestMeter ? '' : 'pl-7']"
              :placeholder="t('admin.accounts.quotaLimitPlaceholder')"
            />
          </div>
          <p class="input-hint">
            {{ isRequestMeter ? t('admin.accounts.quotaTotalLimitHintRequests') : t('admin.accounts.quotaTotalLimitHint') }}
          </p>
        </div>
=======
      <!-- Collapsible content -->
      <div v-if="localEnabled && !collapsed" class="space-y-2 p-4 pt-3">
        <!-- Daily quota -->
        <QuotaDimensionRow
          dim="daily"
          :label="t('admin.accounts.quotaDailyLimit')"
          :limit="dailyLimit"
          :quota-notify-global-enabled="quotaNotifyGlobalEnabled"
          :notify-enabled="props.quotaNotifyDailyEnabled"
          :notify-threshold="props.quotaNotifyDailyThreshold"
          :notify-threshold-type="props.quotaNotifyDailyThresholdType"
          :reset-mode="dailyResetMode"
          :reset-hour="dailyResetHour"
          :reset-day="null"
          :reset-timezone="resetTimezone"
          :hint-rolling="t('admin.accounts.quotaDailyLimitHint')"
          :hint-fixed="dailyFixedHint"
          :hour-options="hourOptions"
          :day-options="dayOptions"
          :timezone-options="timezoneOptions"
          @update:limit="emit('update:dailyLimit', $event)"
          @update:notify-enabled="emit('update:quotaNotifyDailyEnabled', $event)"
          @update:notify-threshold="emit('update:quotaNotifyDailyThreshold', $event)"
          @update:notify-threshold-type="emit('update:quotaNotifyDailyThresholdType', $event)"
          @update:reset-mode="emit('update:dailyResetMode', $event)"
          @update:reset-hour="emit('update:dailyResetHour', $event)"
          @update:reset-timezone="emit('update:resetTimezone', $event)"
        />

        <!-- Weekly quota -->
        <QuotaDimensionRow
          dim="weekly"
          :label="t('admin.accounts.quotaWeeklyLimit')"
          :limit="weeklyLimit"
          :quota-notify-global-enabled="quotaNotifyGlobalEnabled"
          :notify-enabled="props.quotaNotifyWeeklyEnabled"
          :notify-threshold="props.quotaNotifyWeeklyThreshold"
          :notify-threshold-type="props.quotaNotifyWeeklyThresholdType"
          :reset-mode="weeklyResetMode"
          :reset-hour="weeklyResetHour"
          :reset-day="weeklyResetDay"
          :reset-timezone="resetTimezone"
          :hint-rolling="t('admin.accounts.quotaWeeklyLimitHint')"
          :hint-fixed="weeklyFixedHint"
          :hour-options="hourOptions"
          :day-options="dayOptions"
          :timezone-options="timezoneOptions"
          @update:limit="emit('update:weeklyLimit', $event)"
          @update:notify-enabled="emit('update:quotaNotifyWeeklyEnabled', $event)"
          @update:notify-threshold="emit('update:quotaNotifyWeeklyThreshold', $event)"
          @update:notify-threshold-type="emit('update:quotaNotifyWeeklyThresholdType', $event)"
          @update:reset-mode="emit('update:weeklyResetMode', $event)"
          @update:reset-hour="emit('update:weeklyResetHour', $event)"
          @update:reset-day="emit('update:weeklyResetDay', $event)"
          @update:reset-timezone="emit('update:resetTimezone', $event)"
        />

        <!-- Total quota -->
        <QuotaDimensionRow
          dim="total"
          :label="t('admin.accounts.quotaTotalLimit')"
          :limit="totalLimit"
          :quota-notify-global-enabled="quotaNotifyGlobalEnabled"
          :notify-enabled="props.quotaNotifyTotalEnabled"
          :notify-threshold="props.quotaNotifyTotalThreshold"
          :notify-threshold-type="props.quotaNotifyTotalThresholdType"
          :reset-mode="null"
          :reset-hour="null"
          :reset-day="null"
          :reset-timezone="null"
          :hint-rolling="t('admin.accounts.quotaTotalLimitHint')"
          hint-fixed=""
          :hour-options="hourOptions"
          :day-options="dayOptions"
          @update:limit="emit('update:totalLimit', $event)"
          @update:notify-enabled="emit('update:quotaNotifyTotalEnabled', $event)"
          @update:notify-threshold="emit('update:quotaNotifyTotalThreshold', $event)"
          @update:notify-threshold-type="emit('update:quotaNotifyTotalThresholdType', $event)"
        />
>>>>>>> upstream/main
      </div>
  </div>
</template>
