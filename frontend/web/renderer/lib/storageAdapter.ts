import { STORAGE_ADAPTER_KEYS } from '@/constants/storageAdapterKey'
import { INVOKE_CHANNLE } from '@shared/constants/channleName'
import { isDesktop } from './utils'

/**
 * Storage adapter interface
 */
interface StorageAdapter {
  save: (data: any) => Promise<void>
  load: () => Promise<any>
}

/**
 * Desktop storage adapter
 */
class DesktopStorageAdapter implements StorageAdapter {
  private key: string
  constructor(key: string) {
    this.key = key
  }
  async save(data: any) {
    const desktopChannleData = STORAGE_ADAPTER_KEYS[this.key]?.desktop
    return window.ipc?.invoke(desktopChannleData?.save, data)
  }

  async load() {
    const desktopChannleData = STORAGE_ADAPTER_KEYS[this.key]?.desktop
    return window.ipc?.invoke(desktopChannleData?.load)
  }
}

/**
 * Web storage adapter
 */
class WebStorageAdapter implements StorageAdapter {
  // private readonly STORAGE_KEY = 'chat_store_data'
  private key: string
  constructor(key: string) {
    this.key = key
  }
  async save(data: any) {
    try {
      const cacheKey = STORAGE_ADAPTER_KEYS[this.key]?.key
      localStorage.setItem(cacheKey, JSON.stringify(data))
    } catch (error) {
      console.error('Failed to save to localStorage:', error)
    }
  }

  async load() {
    try {
      const cacheKey = STORAGE_ADAPTER_KEYS[this.key]?.key
      const data = localStorage.getItem(cacheKey)
      return data ? JSON.parse(data) : null
    } catch (error) {
      console.error('Failed to load from localStorage:', error)
      return null
    }
  }
}

/**
 * Get storage adapter for current environment
 */
export const getStorageAdapter = ({ key }: { key: string }): StorageAdapter => {
  return isDesktop() ? new DesktopStorageAdapter(key) : new WebStorageAdapter(key)
}
