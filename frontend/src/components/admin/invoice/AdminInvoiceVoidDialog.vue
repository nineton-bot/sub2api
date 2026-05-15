<template>
  <BaseDialog
    :show="show"
    :title="t('admin.invoices.dialogs.void.title')"
    width="narrow"
    @close="onClose"
  >
    <div class="space-y-3">
      <!-- 已开票（issued）+ 自动渠道 → 提示会触发真红冲 -->
      <div
        v-if="warnIssued && willAutoReverse"
        class="rounded-lg bg-red-50 p-3 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300"
      >
        <div class="font-medium">该发票已在开票平台出具，作废将自动调用红冲接口（不可恢复）</div>
        <ul class="mt-1 list-disc pl-4 leading-relaxed">
          <li>系统会向财云通发起红字信息单 + 红票申请</li>
          <li>red 票成功后本地状态才会转为「已作废」</li>
          <li>若关联的订单已开发票，订单将随红冲完成同步释放</li>
        </ul>
      </div>
      <div
        v-else-if="warnIssued"
        class="rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
      >
        {{ t('admin.invoices.dialogs.void.warningIssued') }}
      </div>
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.invoices.dialogs.void.reasonLabel') }}
      </label>
      <textarea
        v-model="reason"
        rows="4"
        class="input w-full"
        :placeholder="t('admin.invoices.dialogs.void.reasonRequired')"
      />
      <div v-if="errorMsg" class="rounded-lg bg-red-50 p-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMsg }}
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" @click="onClose">{{ t('common.cancel') }}</button>
        <button
          class="btn btn-danger"
          :disabled="submitting || !reason.trim()"
          @click="submit"
        >
          {{ submitting ? t('common.processing') : t('admin.invoices.dialogs.void.confirm') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import adminInvoiceAPI from '@/api/admin/invoices'
import { extractApiErrorMessage } from "@/utils/apiError"

const props = defineProps<{
  show: boolean
  invoiceId: number | null
  warnIssued?: boolean
  /**
   * 自动渠道（如 caiyuntong）issued 状态时 = true，
   * 提示作废会真正触发开票平台的红冲流程（不可恢复）。
   * manual 渠道或 approved 状态时 = false，作废仅本地标记。
   */
  willAutoReverse?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  (e: 'submitted'): void
}>()

const { t } = useI18n()
const reason = ref('')
const submitting = ref(false)
const errorMsg = ref('')

watch(
  () => props.show,
  (v) => {
    if (v) {
      reason.value = ''
      errorMsg.value = ''
    }
  },
)

async function submit() {
  if (!props.invoiceId || !reason.value.trim()) return
  submitting.value = true
  errorMsg.value = ''
  try {
    await adminInvoiceAPI.void(props.invoiceId, reason.value.trim())
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
