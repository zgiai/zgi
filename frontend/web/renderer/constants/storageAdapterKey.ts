import { INVOKE_CHANNLE } from '@shared/constants/channleName'

export const STORAGE_ADAPTER_KEYS = {
  chat: {
    key: 'chat',
    desktop: {
      save: INVOKE_CHANNLE.saveChats,
      load: INVOKE_CHANNLE.loadChats,
    },
  },
  app_settings: {
    key: 'app_settings',
    desktop: {
      save: INVOKE_CHANNLE.saveAppSettings,
      load: INVOKE_CHANNLE.loadAppSettings,
    },
  },
  userInfo: {
    key: 'userInfo',
    desktop: {
      save: INVOKE_CHANNLE.saveUserInfo,
      load: INVOKE_CHANNLE.loadUserInfo,
    },
  },
}
