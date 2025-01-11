import { HTTP_STATUS_CODE } from '@/constants/http_status'
import { STORAGE_ADAPTER_KEYS } from '@/constants/storageAdapterKey'
import { getStorageAdapter } from '@/lib/storageAdapter'
import { createSubsStore } from '@/lib/store_utils'
import { login, registerUser } from '@/server/user.server'
import type { UserStore } from './types'

const storageAdapter = getStorageAdapter({ key: STORAGE_ADAPTER_KEYS.userInfo.key })

export const useUserStore = createSubsStore<UserStore>((set, get) => {
  return {
    user: null,
    userFormData: {
      email: '',
      password: '',
    },
    isUserOpen: false,
    isRegistering: false,
    loading: false,

    init: async () => {
      const userInfoData = await storageAdapter.load()
      if (userInfoData) {
        localStorage.setItem('auth_token', userInfoData?.access_token)
        localStorage.setItem('token_type', userInfoData?.token_type)
        set({
          user: userInfoData?.user,
        })
      }
    },

    setUserFormData: (data) =>
      set((state) => ({
        userFormData: {
          ...state.userFormData,
          ...data,
        },
      })),
    resetUserFormData: () => {
      set({
        userFormData: {
          email: '',
          password: '',
          username: '',
        },
      })
    },
    setUserOpen: (flag) => set({ isUserOpen: flag }),
    toggleRegistering: () => {
      set((state) => {
        return {
          isRegistering: !state.isRegistering,
          userFormData: { email: '', password: '', username: '' },
        }
      })
    },

    handleSignIn: async () => {
      set({
        loading: true,
      })
      const { userFormData, resetUserFormData } = get()
      const res = await login({
        email: userFormData?.email,
        password: userFormData?.password,
      })
      if (res?.data && res.status_code === HTTP_STATUS_CODE.SUCCESS) {
        localStorage.setItem('auth_token', res.data?.access_token)
        localStorage.setItem('token_type', res.data?.token_type)

        storageAdapter.save({
          access_token: res.data?.access_token,
          token_type: res.data?.token_type,
          user: res.data?.user,
        })
        resetUserFormData()
        set({ isUserOpen: false, user: res.data?.user })
      }
      set({
        loading: false,
      })
    },

    handleRegister: async () => {
      set({
        loading: true,
      })
      const { handleSignIn, userFormData } = get()
      const res = await registerUser({
        username: userFormData?.username,
        email: userFormData?.email,
        password: userFormData?.password,
      })
      if (res?.data && res.status_code === HTTP_STATUS_CODE.SUCCESS) {
        handleSignIn()
      } else {
        set({
          loading: false,
        })
      }
    },
  }
})
