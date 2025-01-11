import { http } from '@/lib/http'

export const login = (data: any) => {
  return http.post('/v1/login', data)
}

export const getUserInfo = () => {
  return http.get('/v1/me')
}

export const registerUser = (data: any) => {
  return http.post('/v1/register', data)
}
