<template>
  <div class="space-y-4">
    <button type="button" :disabled="disabled" class="btn btn-secondary w-full" @click="startLogin">
      <span
        class="mr-2 inline-flex h-5 w-5 items-center justify-center rounded-full bg-primary-100 text-xs font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
      >
        {{ providerInitial }}
      </span>
      {{ t('auth.oidc.signIn', { providerName: normalizedProviderName }) }}
    </button>

    <div v-if="showDivider" class="flex items-center gap-3">
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
      <span class="text-xs text-gray-500 dark:text-dark-400">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getReferralCodeFromCookie } from '@/utils/referralCookie'

const props = withDefaults(defineProps<{
  disabled?: boolean
  providerName?: string
  showDivider?: boolean
}>(), {
  providerName: 'OIDC',
  showDivider: true
})

const route = useRoute()
const { t } = useI18n()

const normalizedProviderName = computed(() => {
  const name = props.providerName?.trim()
  return name || 'OIDC'
})

const providerInitial = computed(() => normalizedProviderName.value.charAt(0).toUpperCase() || 'O')

function startLogin(): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  // 邀请返佣：优先显式 URL ?ref=，其次从 /g/:code 设置的 cookie 读取。
  // 追加到 OAuth start URL，由后端把 ref 转存到 OAuth 专用 cookie，回调时取回绑定。
  const refCode = (route.query.ref as string) || getReferralCodeFromCookie() || ''
  const refSuffix = refCode ? `&ref=${encodeURIComponent(refCode)}` : ''
  const startURL = `${normalized}/auth/oauth/oidc/start?redirect=${encodeURIComponent(redirectTo)}${refSuffix}`
  window.location.href = startURL
}
</script>
