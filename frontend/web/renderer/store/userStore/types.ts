export interface User {
  username?: string
  email?: string
  id?: string
}

export interface UserStore {
  loading: boolean
  user: User | null
  userFormData: UserFormData
  isUserOpen: boolean
  isRegistering: boolean

  setUserFormData: (data: UserFormData) => void
  resetUserFormData: () => void
  setUserOpen: (flag: boolean) => void
  toggleRegistering: () => void
  handleSignIn: () => void
  handleRegister: () => void
  init: () => Promise<void>
}

interface UserFormData {
  email?: string
  username?: string
  password?: string
  confirmPassword?: string
}
