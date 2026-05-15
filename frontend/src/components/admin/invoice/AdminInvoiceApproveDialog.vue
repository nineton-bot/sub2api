<template>
  <BaseDialog
    :show="show"
    title="审批发票申请"
    width="normal"
    @close="onClose"
  >
    <div class="space-y-5">
      <!-- 申请单基本信息 -->
      <div v-if="invoice" class="rounded-lg bg-gray-50 px-4 py-3 text-sm dark:bg-dark-800/40">
        <div class="text-gray-900 dark:text-white">{{ invoice.title }}</div>
        <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          <span v-if="invoice.title_type === 'business' && invoice.tax_no">{{ invoice.tax_no }}</span>
          <span v-else>个人抬头</span>
          ·
          <span class="font-medium text-gray-700 dark:text-gray-300">¥ {{ invoice.amount.toFixed(2) }}</span>
        </div>
      </div>

      <!-- 用户备注：常包含「请开专票」等开票诉求，需在选「票种」前提醒管理员看 -->
      <div
        v-if="userNotes"
        class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-xs dark:border-amber-700/40 dark:bg-amber-900/10"
      >
        <div class="mb-1 font-medium text-amber-800 dark:text-amber-300">用户备注</div>
        <div class="whitespace-pre-wrap break-words text-amber-900 dark:text-amber-200">{{ userNotes }}</div>
      </div>
      <div v-else-if="loadingDetail" class="text-xs text-gray-400">加载用户备注…</div>

      <!-- 票种 -->
      <div>
        <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
          票种 <span class="text-red-500">*</span>
        </label>
        <div class="grid grid-cols-2 gap-3">
          <button
            type="button"
            class="rounded-md border px-3 py-2 text-sm transition-colors"
            :class="invoiceKind === 'normal'
              ? 'border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-400 dark:bg-blue-900/20 dark:text-blue-300'
              : 'border-gray-300 text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800'"
            @click="invoiceKind = 'normal'"
          >
            普票（增值税普通发票）
          </button>
          <button
            type="button"
            class="rounded-md border px-3 py-2 text-sm transition-colors"
            :class="invoiceKind === 'special'
              ? 'border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-400 dark:bg-blue-900/20 dark:text-blue-300'
              : 'border-gray-300 text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800'"
            :disabled="!canChooseSpecial"
            :title="canChooseSpecial ? '' : `专票必填项缺失：${specialMissing.join('、')}`"
            @click="canChooseSpecial && (invoiceKind = 'special')"
          >
            专票（增值税专用发票）
          </button>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          默认普票。专票需购方税号 / 地址 / 电话 / 开户行 / 银行账号全部齐全。
        </p>
        <p v-if="!canChooseSpecial" class="mt-1 text-xs text-amber-700 dark:text-amber-400">
          ⚠ 专票缺失：{{ specialMissing.join('、') }}（请让用户补齐后重新申请）
        </p>
      </div>

      <!-- 开票渠道 -->
      <div>
        <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
          开票渠道 <span class="text-red-500">*</span>
        </label>
        <select v-model="provider" class="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm dark:border-dark-600 dark:bg-dark-800 dark:text-white">
          <option value="caiyuntong">财云通（自动开票）</option>
          <option value="manual">人工开票（管理员上传 PDF）</option>
          <option value="nuonuo" disabled>诺诺网（规划中）</option>
          <option value="baiwang" disabled>百望云（规划中）</option>
        </select>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          自动开票通过后由系统调用第三方接口异步开具，开票完成自动邮件通知用户。
        </p>
      </div>

      <!-- 备注 -->
      <div>
        <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">审批备注（可选）</label>
        <textarea
          v-model="notes"
          rows="3"
          class="input w-full"
          placeholder="审批意见，将留存日志"
        />
      </div>

      <div v-if="errorMsg" class="rounded-lg bg-red-50 p-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMsg }}
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" @click="onClose">{{ t('common.cancel') }}</button>
        <button
          class="btn btn-primary"
          :disabled="submitting"
          @click="submit"
        >
          {{ submitting ? t('common.processing') : '批准并自动开票' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import adminInvoiceAPI from '@/api/admin/invoices'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { Invoice } from '@/types/invoice'

const props = defineProps<{
  show: boolean
  invoice: Invoice | null
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  (e: 'submitted'): void
}>()

const { t } = useI18n()
const invoiceKind = ref<'normal' | 'special'>('normal')
const provider = ref('caiyuntong')
const notes = ref('')
const submitting = ref(false)
const errorMsg = ref('')
const userNotes = ref('')
const loadingDetail = ref(false)

// 专票需要：企业抬头 + 税号 + 地址 + 电话 + 开户行 + 银行账号
// 缺一项前端就置灰按钮，避免提交到后端被 422 弹出错误。
const specialMissing = computed<string[]>(() => {
  const inv = props.invoice
  if (!inv || inv.title_type !== 'business') return ['企业抬头']
  const missing: string[] = []
  if (!inv.tax_no) missing.push('税号')
  if (!inv.buyer_address) missing.push('地址')
  if (!inv.buyer_phone) missing.push('电话')
  if (!inv.buyer_bank_name) missing.push('开户行')
  if (!inv.buyer_bank_account) missing.push('银行账号')
  return missing
})

const canChooseSpecial = computed(() => specialMissing.value.length === 0)

watch(
  () => props.show,
  async (v) => {
    if (!v) return
    invoiceKind.value = 'normal'
    provider.value = 'caiyuntong'
    notes.value = ''
    errorMsg.value = ''
    userNotes.value = ''
    if (!props.invoice) return
    loadingDetail.value = true
    try {
      const resp = await adminInvoiceAPI.detail(props.invoice.id)
      userNotes.value = (resp.data.notes || '').trim()
    } catch {
      // 失败不阻塞审批流程，仅静默
    } finally {
      loadingDetail.value = false
    }
  },
)

function onClose() {
  emit('update:show', false)
}

async function submit() {
  if (!props.invoice) return
  submitting.value = true
  errorMsg.value = ''
  try {
    await adminInvoiceAPI.approve(props.invoice.id, {
      notes: notes.value.trim(),
      invoice_kind: invoiceKind.value,
      provider: provider.value,
    })
    emit('submitted')
    emit('update:show', false)
  } catch (err) {
    errorMsg.value = extractApiErrorMessage(err)
  } finally {
    submitting.value = false
  }
}
</script>
