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
      save: INVOKE_CHANNLE.saveChats,
      load: INVOKE_CHANNLE.loadChats,
    },
  },
}
