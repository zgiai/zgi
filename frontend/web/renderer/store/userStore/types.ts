export interface User {
  username?: string
  email?: string
  id?: string
}

export interface UserStore {
  loading: boolean
  isUserInfoPopoverOpen: boolean
  user: User | null
  userFormData: UserFormData
  isUserOpen: boolean
  isRegistering: boolean

  init: () => Promise<void>
  onClickSigninBaseBtn: () => void
  setUserFormData: (data: UserFormData) => void
  resetUserFormData: () => void
  setUserOpen: (flag: boolean) => void
  setUserInfoPopoverOpen: (flag: boolean) => void
  toggleRegistering: () => void
  handleSignIn: () => void
  handleRegister: () => void
  handleLogout: () => Promise<void>
  updateSaveConfig: (data: {
    access_token?: string
    token_type?: string
    user?: any
  }) => Promise<void>
}

interface UserFormData {
  email?: string
  username?: string
  password?: string
  confirmPassword?: string
}
