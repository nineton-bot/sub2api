<template>
  <BaseDialog
    :show="show"
    :title="t('referral.transfer.title')"
    width="narrow"
    @close="handleClose"
  >
    <form id="referral-transfer-form" @submit.prevent="handleSubmit" class="space-y-5">
      <!-- 可使用佣金信息 -->
      <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex items-center justify-between text-sm">
          <span class="text-gray-600 dark:text-gray-400">
            {{ t('referral.transfer.usableAvailable') }}
          </span>
          <span class="text-lg font-bold text-gray-900 dark:text-white">
            ¥{{ formatMoney(usable) }}
          </span>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
          {{ t('referral.transfer.hint') }}
        </p>
      </div>

      <!-- 金额输入 -->
      <div>
        <label class="input-label">{{ t('referral.transfer.amount') }}</label>
        <div class="relative flex gap-2">
          <div class="relative flex-1">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 font-medium text-gray-500">¥</span>
            <input
              v-model.number="amount"
              type="number"
              step="0.01"
              min="0"
              required
              class="input pl-8"
              :placeholder="t('referral.transfer.amountPlaceholder')"
            />
          </div>
          <button
            type="button"
            @click="fillAll"
            :disabled="usable <= 0"
            class="btn btn-secondary whitespace-nowrap"
          >
            {{ t('referral.transfer.all') }}
          </button>
        </div>
        <p v-if="amount > 0 && amount < MIN_TRANSFER" class="mt-1 text-xs text-amber-600">
          {{ t('referral.transfer.belowMin', { min: MIN_TRANSFER.toFixed(2) }) }}
        </p>
        <p v-else-if="amount > usable" class="mt-1 text-xs text-red-600">
          {{ t('referral.transfer.insufficient') }}
        </p>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" @click="handleClose" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="referral-transfer-form"
          :disabled="submitting || !canSubmit"
          class="btn btn-primary"
        >
          {{ submitting ? t('common.saving') : t('referral.transfer.submit') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import referralAPI from '@/api/referral'
import type { ReferralStats } from '@/api/referral'

const props = defineProps<{
  show: boolean
  usable: number
}>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'success', stats: ReferralStats): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const MIN_TRANSFER = 0.01
const amount = ref<number>(0)
const submitting = ref(false)

watch(() => props.show, (v) => { if (v) { amount.value = 0 } })

const canSubmit = computed(() => {
  const a = Number(amount.value)
  return !isNaN(a) && a >= MIN_TRANSFER && a <= props.usable
})

function fillAll() {
  amount.value = Number(props.usable.toFixed(2))
}

function formatMoney(v: number) {
  if (!v || v <= 0) return '0.00'
  return v.toFixed(2)
}

function handleClose() {
  if (!submitting.value) emit('close')
}

async function handleSubmit() {
  const a = Number(amount.value)
  if (!canSubmit.value) {
    appStore.showError(t('referral.transfer.invalidAmount'))
    return
  }
  submitting.value = true
  try {
    const stats = await referralAPI.transferToBalance(a)
    appStore.showSuccess(t('referral.transfer.success', { amount: a.toFixed(2) }))
    emit('success', stats)
    emit('close')
  } catch (e: any) {
    console.error('transferToBalance:', e)
    const code = e?.response?.data?.code
    if (code === 'INSUFFICIENT_REFERRAL_USABLE') {
      appStore.showError(t('referral.transfer.insufficient'))
    } else if (code === 'INVALID_AMOUNT') {
      appStore.showError(t('referral.transfer.belowMin', { min: MIN_TRANSFER.toFixed(2) }))
    } else {
      appStore.showError(e?.response?.data?.detail || t('common.error'))
    }
  } finally {
    submitting.value = false
  }
}
</script>
