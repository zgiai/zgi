import { HTTP_STATUS_CODE } from '@/constants/http_status'
import { STORAGE_ADAPTER_KEYS } from '@/constants/storageAdapterKey'
import { getStorageAdapter } from '@/lib/storageAdapter'
import { createSubsStore } from '@/lib/store_utils'
import { login } from '@/server/user.server'
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

    setUser: (user) => set({ user }),
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
        },
      })
    },
    setUserOpen: (flag) => set({ isUserOpen: flag }),
    toggleRegistering: () => {
      set((state) => {
        return { isRegistering: !state.isRegistering }
      })
    },

    handleSignIn: async () => {
      const { userFormData, setUser, resetUserFormData } = get()
      const res = await login({
        email: userFormData?.email,
        password: userFormData?.password,
      })
      console.log(res, 'res')
      if (res?.data && res.status_code === HTTP_STATUS_CODE) {
        setUser({ ...res.data?.user })
        resetUserFormData()
        // set({ isUserOpen: false })
      }
    },

    handleRegister: () => {
      const { userFormData, setUser, resetUserFormData } = get()
      setUser({ username: userFormData.email as string })
      resetUserFormData()
      set({ isUserOpen: false })
    },
  }
})
