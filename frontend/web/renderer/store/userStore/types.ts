export interface User {
  username?: string
  email?: string
  id?: string
}

export interface UserStore {
  user: User | null
  setUser: (user: User | null) => void

  userFormData: UserFormData
  setUserFormData: (data: UserFormData) => void
  resetUserFormData: () => void

  isUserOpen: boolean
  isRegistering: boolean
  setUserOpen: (flag: boolean) => void
  toggleRegistering: () => void
  handleSignIn: () => void
  handleRegister: () => void
}

interface UserFormData {
  email?: string
  username?: string
  password?: string
  confirmPassword?: string
}
