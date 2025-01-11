import { useAppSettingsStore } from '@/store/appSettingsStore'
import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { API_CONFIG } from './http'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export const getFetchApiKey = () => {
  const apiKey = useAppSettingsStore.getState().providers?.zgi?.apiKey
  return apiKey
}

export const getAPIProxyAddress = () => {
  const apiEndpoint = useAppSettingsStore.getState().providers?.zgi?.apiEndpoint
  return apiEndpoint || API_CONFIG.COMMON
}
