<template>
  <BaseDialog
    :show="show"
    :title="t('admin.invoices.dialogs.upload.title')"
    width="normal"
    @close="onClose"
  >
    <div class="space-y-4">
      <div>
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.invoices.dialogs.upload.fileLabel') }}
        </label>
        <input
          ref="fileInput"
          type="file"
          accept="application/pdf"
          class="mt-1 block w-full text-sm text-gray-700 dark:text-gray-300"
          @change="onFileChange"
        />
        <p class="mt-1 text-xs text-gray-500">{{ t('admin.invoices.dialogs.upload.sizeTip') }}</p>
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.invoices.dialogs.upload.invoiceNoLabel') }}
        </label>
        <input
          v-model="invoiceNo"
          type="text"
          class="input mt-1 w-full"
          :placeholder="t('admin.invoices.dialogs.upload.invoiceNoPlaceholder')"
        />
      </div>
      <div v-if="progress > 0 && progress < 100" class="text-xs text-gray-500">
        {{ progress }}%
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
          :disabled="uploading || !file"
          @click="submit"
        >
          {{ uploading ? t('admin.invoices.dialogs.upload.uploading') : t('admin.invoices.dialogs.upload.confirm') }}
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
  /** true 表示 issued 状态下重新上传 */
  replace?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
  (e: 'submitted'): void
}>()

const { t } = useI18n()
const file = ref<File | null>(null)
const invoiceNo = ref('')
const uploading = ref(false)
const progress = ref(0)
const errorMsg = ref('')
const fileInput = ref<HTMLInputElement | null>(null)

watch(
  () => props.show,
  (v) => {
    if (v) {
      file.value = null
      invoiceNo.value = ''
      progress.value = 0
      errorMsg.value = ''
      if (fileInput.value) fileInput.value.value = ''
    }
  },
)

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files && input.files.length > 0) {
    file.value = input.files[0]
  } else {
    file.value = null
  }
}

async function submit() {
  if (!props.invoiceId || !file.value) return
  uploading.value = true
  errorMsg.value = ''
  progress.value = 0
  try {
    if (props.replace) {
      await adminInvoiceAPI.replacePdf(props.invoiceId, file.value, (p) => (progress.value = p))
    } else {
      await adminInvoiceAPI.uploadPdf(props.invoiceId, file.value, invoiceNo.value.trim(), (p) => (progress.value = p))
    }
    emit('submitted')
    onClose()
  } catch (err) {
    errorMsg.value = extractApiErrorMessage(err)
  } finally {
    uploading.value = false
  }
}

function onClose() {
  emit('update:show', false)
}
</script>
