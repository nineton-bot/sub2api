<template>
  <BaseDialog :show="show" :title="t('invoices.applyTitle')" width="wide" @close="onClose">
    <div class="space-y-5">
      <!-- 抬头信息 -->
      <div class="space-y-3">
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('invoices.fields.titleSection') }}</h4>
        <div class="flex gap-4">
          <label class="inline-flex items-center gap-2 text-sm">
            <input v-model="form.title_type" type="radio" value="personal" class="h-4 w-4" />
            {{ t('invoices.titleType.personal') }}
          </label>
          <label class="inline-flex items-center gap-2 text-sm">
            <input v-model="form.title_type" type="radio" value="business" class="h-4 w-4" />
            {{ t('invoices.titleType.business') }}
          </label>
        </div>

        <input
          v-model="form.title"
          type="text"
          class="input w-full"
          :placeholder="t('invoices.fields.titlePlaceholder')"
        />
        <input
          v-if="form.title_type === 'business'"
          v-model="form.tax_no"
          type="text"
          class="input w-full"
          :placeholder="t('invoices.fields.taxNoPlaceholder')"
        />
        <input
          v-model="form.contact_email"
          type="email"
          class="input w-full"
          :placeholder="t('invoices.fields.emailPlaceholder')"
        />
      </div>

      <!-- 订单选择 -->
      <div class="space-y-2">
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('invoices.fields.selectOrders') }}</h4>
        <div v-if="ordersLoading" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else-if="eligibleOrders.length === 0" class="rounded-lg bg-gray-50 p-4 text-center text-sm text-gray-500 dark:bg-dark-800 dark:text-gray-400">
          {{ t('invoices.eligibleOrders.empty') }}
        </div>
        <div v-else class="max-h-64 space-y-2 overflow-y-auto">
          <label
            v-for="o in eligibleOrders"
            :key="o.id"
            class="flex cursor-pointer items-center justify-between rounded-lg border border-gray-200 p-3 transition hover:border-emerald-300 dark:border-dark-700 dark:hover:border-emerald-500"
            :class="{ 'border-emerald-400 bg-emerald-50/40 dark:bg-emerald-900/10': selectedIds.has(o.id) }"
          >
            <div class="flex items-center gap-3">
              <input
                type="checkbox"
                class="h-4 w-4 rounded"
                :checked="selectedIds.has(o.id)"
                @change="toggleOrder(o.id)"
              />
              <div>
                <div class="text-sm font-medium text-gray-900 dark:text-white">{{ o.product_name }}</div>
                <div class="text-xs text-gray-500 dark:text-gray-400">
                  {{ o.order_no }} · {{ formatDate(o.paid_at) }}
                </div>
              </div>
            </div>
            <div class="text-right text-sm font-semibold text-emerald-600 dark:text-emerald-400">
              ¥{{ o.pay_amount.toFixed(2) }}
            </div>
          </label>
        </div>

        <div v-if="eligibleOrders.length > 0" class="flex items-center justify-between border-t border-gray-200 pt-2 text-sm dark:border-dark-700">
          <span class="text-gray-500 dark:text-gray-400">
            {{ t('invoices.eligibleOrders.totalAmount', { count: selectedIds.size }) }}
          </span>
          <span class="text-base font-bold text-emerald-600 dark:text-emerald-400">¥{{ totalAmount.toFixed(2) }}</span>
        </div>
      </div>

      <!-- 备注 -->
      <textarea
        v-model="form.notes"
        rows="2"
        class="input w-full"
        :placeholder="t('invoices.fields.notesPlaceholder')"
      />

      <!-- 错误提示 -->
      <div v-if="errorMsg" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMsg }}
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" @click="onClose">{{ t('common.cancel') }}</button>
        <button
          class="btn btn-primary"
          :disabled="submitting || !canSubmit"
          @click="submit"
        >
          {{ submitting ? t('common.processing') : t('invoices.fields.submit') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import invoiceAPI from '@/api/invoices'
import { extractApiErrorMessage } from "@/utils/apiError"
import type { EligibleOrder, InvoiceTitleType } from '@/types/invoice'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'submitted'): void
}>()

const { t } = useI18n()

const form = reactive({
  title_type: 'personal' as InvoiceTitleType,
  title: '',
  tax_no: '',
  contact_email: '',
  notes: '',
})

const eligibleOrders = ref<EligibleOrder[]>([])
const selectedIds = ref<Set<number>>(new Set())
const ordersLoading = ref(false)
const submitting = ref(false)
const errorMsg = ref('')

const totalAmount = computed(() =>
  eligibleOrders.value
    .filter((o) => selectedIds.value.has(o.id))
    .reduce((sum, o) => sum + o.pay_amount, 0),
)

const canSubmit = computed(() => {
  if (!form.title.trim()) return false
  if (form.title_type === 'business' && !form.tax_no.trim()) return false
  if (selectedIds.value.size === 0) return false
  return true
})

watch(
  () => props.show,
  (val) => {
    if (val) {
      resetForm()
      void fetchEligible()
    }
  },
)

// 切到「个人」时清空税号，避免残留
watch(
  () => form.title_type,
  (val) => {
    if (val === 'personal') {
      form.tax_no = ''
    }
  },
)

async function fetchEligible() {
  ordersLoading.value = true
  errorMsg.value = ''
  try {
    const res = await invoiceAPI.eligibleOrders()
    eligibleOrders.value = res.data.items || []
  } catch (err) {
    errorMsg.value = extractApiErrorMessage(err)
  } finally {
    ordersLoading.value = false
  }
}

function resetForm() {
  form.title_type = 'personal'
  form.title = ''
  form.tax_no = ''
  form.contact_email = ''
  form.notes = ''
  selectedIds.value = new Set()
  errorMsg.value = ''
}

function toggleOrder(id: number) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedIds.value = next
}

function formatDate(s: string): string {
  if (!s) return ''
  try {
    return new Date(s).toLocaleString()
  } catch {
    return s
  }
}

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  errorMsg.value = ''
  try {
    await invoiceAPI.create({
      title_type: form.title_type,
      title: form.title.trim(),
      tax_no: form.title_type === 'business' ? form.tax_no.trim() : '',
      contact_email: form.contact_email.trim(),
      notes: form.notes.trim(),
      order_ids: Array.from(selectedIds.value),
    })
    emit('submitted')
    onClose()
  } catch (err) {
    errorMsg.value = extractApiErrorMessage(err)
  } finally {
    submitting.value = false
  }
}

function onClose() {
  emit('update:show', false)
}
</script>
