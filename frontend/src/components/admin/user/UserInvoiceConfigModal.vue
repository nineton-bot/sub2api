<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.invoiceConfig.title')"
    width="narrow"
    @close="$emit('close')"
  >
    <div v-if="user" class="space-y-5">
      <!-- 用户信息 -->
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-emerald-100">
          <span class="text-lg font-medium text-emerald-700">{{ user.email.charAt(0).toUpperCase() }}</span>
        </div>
        <div class="flex-1">
          <p class="font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
          <p class="text-sm text-gray-500 dark:text-gray-400">#{{ user.id }}</p>
        </div>
      </div>

      <!-- 全局策略说明 -->
      <div
        v-if="globalDefaultForAll"
        class="rounded-lg bg-blue-50 p-3 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300"
      >
        {{ t('admin.users.invoiceConfig.defaultForAllOnNote') }}
      </div>

      <!-- 单 toggle：是否允许该用户开发票 -->
      <div class="flex items-start gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-700">
        <input
          id="invoice-enabled-toggle"
          v-model="enabled"
          type="checkbox"
          class="mt-0.5 h-4 w-4"
          :disabled="loading || saving"
        />
        <label for="invoice-enabled-toggle" class="flex-1 cursor-pointer">
          <span class="font-medium text-gray-900 dark:text-white">
            {{ t('admin.users.invoiceConfig.enabled') }}
          </span>
          <p class="mt-0.5 text-xs text-gray-600 dark:text-gray-400">
            {{ t('admin.users.invoiceConfig.enabledHint') }}
          </p>
        </label>
      </div>

      <div v-if="errorMsg" class="rounded-lg bg-red-50 p-3 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMsg }}
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" @click="$emit('close')">{{ t('common.cancel') }}</button>
        <button
          class="btn btn-primary"
          :disabled="saving || loading"
          @click="save"
        >
          {{ saving ? t('common.processing') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import adminInvoiceAPI from '@/api/admin/invoices'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { AdminUser } from '@/types'

const props = defineProps<{
  show: boolean
  user: AdminUser | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const enabled = ref(false)
const loading = ref(false)
const saving = ref(false)
const errorMsg = ref('')

const globalDefaultForAll = computed(
  () => !!appStore.cachedPublicSettings?.invoice_default_for_all_users,
)

watch(
  () => [props.show, props.user?.id] as const,
  async ([show, id]) => {
    if (!show || !id) {
      enabled.value = false
      errorMsg.value = ''
      return
    }
    loading.value = true
    errorMsg.value = ''
    try {
      const res = await adminInvoiceAPI.getUserConfig(id)
      enabled.value = !!res.data.enabled
    } catch (err) {
      errorMsg.value = extractApiErrorMessage(err)
    } finally {
      loading.value = false
    }
  },
  { immediate: false },
)

async function save() {
  if (!props.user) return
  saving.value = true
  errorMsg.value = ''
  try {
    await adminInvoiceAPI.setUserConfig(props.user.id, enabled.value)
    emit('close')
  } catch (err) {
    errorMsg.value = extractApiErrorMessage(err)
  } finally {
    saving.value = false
  }
}
</script>
