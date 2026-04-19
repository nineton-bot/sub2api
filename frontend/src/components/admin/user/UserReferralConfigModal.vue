<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.referralConfig.title')"
    width="narrow"
    @close="$emit('close')"
  >
    <form v-if="user" id="referral-config-form" @submit.prevent="handleSubmit" class="space-y-5">
      <!-- 用户信息 -->
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100">
          <span class="text-lg font-medium text-primary-700">{{ user.email.charAt(0).toUpperCase() }}</span>
        </div>
        <div class="flex-1">
          <p class="font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">#{{ user.id }}</p>
        </div>
      </div>

      <!-- 启用状态：单 toggle（默认跟随全局 referral_default_for_all_users，toggle 不动则不覆盖 DB 原值） -->
      <div class="flex items-start gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-700">
        <input
          id="referral-enabled-toggle"
          v-model="enabledToggle"
          type="checkbox"
          class="mt-0.5 h-4 w-4"
        />
        <label for="referral-enabled-toggle" class="flex-1 cursor-pointer">
          <span class="font-medium text-gray-900 dark:text-white">
            {{ t('admin.users.referralConfig.enabled') }}
          </span>
          <p class="mt-0.5 text-xs text-gray-600 dark:text-gray-400">
            {{ t('admin.users.referralConfig.enabledHint') }}
          </p>
        </label>
      </div>

      <!-- 佣金比例 override -->
      <div>
        <div class="flex items-center justify-between">
          <label class="input-label">{{ t('admin.users.referralConfig.rateOverride') }}</label>
          <label class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <input type="checkbox" v-model="rateFollow" class="h-3.5 w-3.5" />
            {{ t('admin.users.referralConfig.followGlobal') }}
          </label>
        </div>
        <div class="relative">
          <input
            v-model.number="ratePercentInput"
            type="number"
            step="0.1"
            min="0"
            max="100"
            :disabled="rateFollow"
            class="input pr-8"
            :placeholder="t('admin.users.referralConfig.ratePlaceholder')"
          />
          <span class="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-gray-500">%</span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.users.referralConfig.rateHint') }}
        </p>
      </div>

      <!-- 新人赠金 override -->
      <div>
        <div class="flex items-center justify-between">
          <label class="input-label">{{ t('admin.users.referralConfig.bonusOverride') }}</label>
          <label class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <input type="checkbox" v-model="bonusFollow" class="h-3.5 w-3.5" />
            {{ t('admin.users.referralConfig.followGlobal') }}
          </label>
        </div>
        <div class="relative">
          <span class="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-gray-500">¥</span>
          <input
            v-model.number="bonusInput"
            type="number"
            step="0.01"
            min="0"
            :disabled="bonusFollow"
            class="input pl-7"
            :placeholder="t('admin.users.referralConfig.bonusPlaceholder')"
          />
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.users.referralConfig.bonusHint') }}
        </p>
      </div>

      <!-- 可提现开关 -->
      <div class="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/40">
        <input
          id="withdrawal-allowed"
          v-model="form.withdrawal_allowed"
          type="checkbox"
          class="mt-0.5 h-4 w-4"
        />
        <label for="withdrawal-allowed" class="flex-1 cursor-pointer">
          <span class="font-medium text-gray-900 dark:text-white">
            {{ t('admin.users.referralConfig.withdrawalAllowed') }}
          </span>
          <p class="mt-0.5 text-xs text-gray-600 dark:text-gray-400">
            {{ t('admin.users.referralConfig.withdrawalAllowedHint') }}
          </p>
        </label>
      </div>

      <!-- 备注 -->
      <div>
        <label class="input-label">{{ t('admin.users.referralConfig.notes') }}</label>
        <textarea v-model="form.notes" rows="2" class="input"></textarea>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" @click="$emit('close')" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="referral-config-form"
          :disabled="submitting"
          class="btn btn-primary"
        >
          {{ submitting ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { usersAPI, type UserReferralConfig } from '@/api/admin/users'
import type { AdminUser } from '@/types'

const props = defineProps<{
  show: boolean
  user: AdminUser | null
}>()
const emit = defineEmits(['close', 'success'])

const { t } = useI18n()
const appStore = useAppStore()

const form = reactive<{
  withdrawal_allowed: boolean
  notes: string
}>({
  withdrawal_allowed: false,
  notes: ''
})

// enabled toggle：UI 只暴露单 boolean；保存时若未变动则回写原值（含 null），保留 DB 原状态
const enabledToggle = ref<boolean>(false)
const enabledInitialToggle = ref<boolean>(false)
const originalEnabledRaw = ref<boolean | null>(null)

const rateFollow = ref(true)
const ratePercentInput = ref<number | null>(null)
const bonusFollow = ref(true)
const bonusInput = ref<number | null>(null)
const submitting = ref(false)

function resetForm() {
  form.withdrawal_allowed = false
  form.notes = ''
  enabledToggle.value = false
  enabledInitialToggle.value = false
  originalEnabledRaw.value = null
  rateFollow.value = true
  ratePercentInput.value = null
  bonusFollow.value = true
  bonusInput.value = null
}

async function loadConfig() {
  if (!props.user) return
  resetForm()
  try {
    const cfg: UserReferralConfig = await usersAPI.getReferralConfig(props.user.id)
    originalEnabledRaw.value = (cfg.enabled ?? null) as boolean | null
    const globalDefault = appStore.cachedPublicSettings?.referral_default_for_all_users === true
    // 初始 toggle = override（若有）否则跟随全局默认
    enabledToggle.value = originalEnabledRaw.value ?? globalDefault
    enabledInitialToggle.value = enabledToggle.value
    form.withdrawal_allowed = cfg.withdrawal_allowed
    form.notes = cfg.notes
    if (cfg.commission_rate_override !== null && cfg.commission_rate_override !== undefined) {
      rateFollow.value = false
      ratePercentInput.value = Number((cfg.commission_rate_override * 100).toFixed(2))
    }
    if (cfg.referee_bonus_override !== null && cfg.referee_bonus_override !== undefined) {
      bonusFollow.value = false
      bonusInput.value = cfg.referee_bonus_override
    }
  } catch (e: any) {
    console.error('load referral config:', e)
    appStore.showError(e?.response?.data?.detail || t('common.error'))
  }
}

watch(() => props.show, (v) => {
  if (v && props.user) {
    loadConfig()
  }
})

async function handleSubmit() {
  if (!props.user) return
  // 校验
  if (!rateFollow.value) {
    const p = ratePercentInput.value
    if (p === null || p === undefined || isNaN(p) || p < 0 || p > 100) {
      appStore.showError(t('admin.users.referralConfig.rateInvalid'))
      return
    }
  }
  if (!bonusFollow.value) {
    const b = bonusInput.value
    if (b === null || b === undefined || isNaN(b) || b < 0) {
      appStore.showError(t('admin.users.referralConfig.bonusInvalid'))
      return
    }
  }

  // "不动就不写"：toggle 未变更时把原值（含 null / follow-global）原样回写，保留 DB 原状态
  const enabledToSend: boolean | null =
    enabledToggle.value === enabledInitialToggle.value
      ? originalEnabledRaw.value
      : enabledToggle.value

  submitting.value = true
  try {
    await usersAPI.upsertReferralConfig(props.user.id, {
      enabled: enabledToSend,
      commission_rate_override: rateFollow.value
        ? null
        : Number(((ratePercentInput.value ?? 0) / 100).toFixed(6)),
      referee_bonus_override: bonusFollow.value ? null : Number(bonusInput.value ?? 0),
      withdrawal_allowed: form.withdrawal_allowed,
      notes: form.notes.trim()
    })
    appStore.showSuccess(t('common.success'))
    emit('success')
    emit('close')
  } catch (e: any) {
    console.error('upsert referral config:', e)
    appStore.showError(e?.response?.data?.detail || t('common.error'))
  } finally {
    submitting.value = false
  }
}
</script>
