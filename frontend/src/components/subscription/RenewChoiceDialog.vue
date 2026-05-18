<template>
  <BaseDialog
    :show="show"
    :title="t('userSubscriptions.renewChoiceTitle')"
    width="normal"
    :close-on-click-outside="true"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('userSubscriptions.renewChoiceIntro', { name: groupName }) }}
      </p>

      <!-- 多张活跃订阅时需要让用户选目标 sub；单张时隐藏选择器，确认即用唯一那张 -->
      <div v-if="existingSubs.length > 1" class="space-y-2">
        <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ t('userSubscriptions.renewChoicePickTarget') }}
        </label>
        <div class="space-y-1.5">
          <label
            v-for="sub in existingSubs"
            :key="sub.id"
            class="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm transition hover:border-primary-400 hover:bg-primary-50 dark:border-dark-600 dark:hover:border-primary-500 dark:hover:bg-primary-900/10"
            :class="selectedSubId === sub.id ? 'border-primary-500 bg-primary-50 dark:border-primary-400 dark:bg-primary-900/10' : ''"
          >
            <input
              v-model="selectedSubId"
              type="radio"
              :value="sub.id"
              class="h-4 w-4 text-primary-500"
            />
            <span class="flex-1 text-gray-900 dark:text-white">
              #{{ sub.id }}
            </span>
            <span class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('userSubscriptions.expires') }}: {{ formatDateOnly(new Date(sub.expires_at)) }}
            </span>
          </label>
        </div>
      </div>

      <div class="grid gap-3 sm:grid-cols-2">
        <button
          type="button"
          class="group flex flex-col gap-2 rounded-xl border border-gray-200 bg-white p-4 text-left transition hover:border-primary-500 hover:bg-primary-50 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-400 dark:hover:bg-primary-900/10"
          :disabled="renewDisabled"
          @click="confirm('renew')"
        >
          <div class="flex items-center gap-2">
            <Icon name="creditCard" size="md" class="text-primary-500" />
            <span class="font-semibold text-gray-900 dark:text-white">
              {{ t('userSubscriptions.renewChoiceRenewLabel') }}
            </span>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ renewDesc }}
          </p>
        </button>
        <button
          type="button"
          class="group flex flex-col gap-2 rounded-xl border border-gray-200 bg-white p-4 text-left transition hover:border-indigo-500 hover:bg-indigo-50 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:border-gray-200 disabled:hover:bg-white dark:border-dark-600 dark:bg-dark-800 dark:hover:border-indigo-400 dark:hover:bg-indigo-900/10 dark:disabled:hover:border-dark-600 dark:disabled:hover:bg-dark-800"
          :disabled="buyDisabled"
          :title="buyDisabled ? t('userSubscriptions.renewChoiceBuyDisabledHint') : ''"
          @click="confirm('new')"
        >
          <div class="flex items-center gap-2">
            <Icon name="creditCard" size="md" class="text-indigo-500" />
            <span class="font-semibold text-gray-900 dark:text-white">
              {{ t('userSubscriptions.renewChoiceBuyLabel') }}
            </span>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ buyDesc }}
          </p>
          <p
            v-if="buyDisabled"
            class="text-[11px] font-medium text-amber-600 dark:text-amber-400"
          >
            {{ t('userSubscriptions.renewChoiceBuyDisabledHint') }}
          </p>
        </button>
      </div>

      <p v-if="hint" class="text-[11px] text-gray-400 dark:text-gray-500">{{ hint }}</p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateOnly } from '@/utils/format'

interface ExistingSub {
  id: number
  expires_at: string
}

interface Props {
  show: boolean
  groupName: string
  existingSubs: ExistingSub[]
  // 调用方未传或传 <=0 时使用 Generic 文案（不显示具体天数）
  validityDays?: number
  // stackCap=0 视为"调用方不知道上限"（如订阅页直接打开弹窗时），用 Generic 文案
  stackCount?: number
  stackCap?: number
  hint?: string
}

const props = withDefaults(defineProps<Props>(), {
  validityDays: 0,
  stackCount: 0,
  stackCap: 0,
  hint: '',
})

const emit = defineEmits<{
  (e: 'confirm', payload: { intent: 'renew' | 'new'; subscriptionId?: number }): void
  (e: 'close'): void
}>()

const { t } = useI18n()

const selectedSubId = ref<number | null>(null)

// 仅监听 show 由 false→true 的瞬间重置选中项；避免每次父组件重渲染都触发回调。
watch(
  () => props.show,
  (show) => {
    if (show && props.existingSubs.length > 0) {
      selectedSubId.value = props.existingSubs[0].id
    }
  },
  { immediate: true },
)

// existingSubs 引用变化（保持 show=true 时切换目标）的兜底：确保选中项仍在列表里
watch(
  () => props.existingSubs,
  (subs) => {
    if (!props.show || subs.length === 0) return
    if (selectedSubId.value === null || !subs.some((s) => s.id === selectedSubId.value)) {
      selectedSubId.value = subs[0].id
    }
  },
)

const renewDisabled = computed(() => props.existingSubs.length === 0)
const buyDisabled = computed(
  () => props.stackCap > 0 && props.stackCount >= props.stackCap,
)

// 文案降级：未传 validityDays / stackCap 时显示通用版本（不含具体数字）。
// 防止调用方未提供完整上下文时出现 "延长 0 天" / "已叠加 0/- 张" 这种奇怪渲染。
const renewDesc = computed(() => {
  if (props.validityDays > 0) {
    return t('userSubscriptions.renewChoiceRenewDesc', { days: props.validityDays })
  }
  return t('userSubscriptions.renewChoiceRenewDescGeneric')
})
const buyDesc = computed(() => {
  if (props.stackCap > 0) {
    return t('userSubscriptions.renewChoiceBuyDesc', {
      count: props.stackCount,
      cap: props.stackCap,
    })
  }
  return t('userSubscriptions.renewChoiceBuyDescGeneric')
})

function confirm(intent: 'renew' | 'new') {
  if (intent === 'renew' && renewDisabled.value) return
  if (intent === 'new' && buyDisabled.value) return

  const payload: { intent: 'renew' | 'new'; subscriptionId?: number } = { intent }
  if (intent === 'renew') {
    payload.subscriptionId = selectedSubId.value ?? props.existingSubs[0]?.id
  }
  emit('confirm', payload)
}
</script>
