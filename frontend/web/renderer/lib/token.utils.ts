export const setAuthToken = ({
  access_token,
  token_type,
}: { access_token: string; token_type: string }) => {
  localStorage.setItem('auth_token', access_token)
  localStorage.setItem('token_type', token_type)
}

export const getAuthToken = () => {
  const access_token = localStorage.getItem('auth_token')
  const token_type = localStorage.getItem('token_type')
  return {
    access_token,
    token_type,
  }
}

export const clearAuthToken = () => {
  localStorage.removeItem('auth_token')
  localStorage.removeItem('token_type')
}
