import request from "@/utils/request"
import { BASE_URL } from "@/config"
import { GetUserByIdParams, GetUserListParams } from "@/interfaces/request"

// Super admin get user list
export const adminGetUserList = (params: GetUserListParams) => request.get(`${BASE_URL}/users`, params)

// Super admin get user by id
export const adminGetUserById = (params: GetUserByIdParams) => request.get(`${BASE_URL}/users/${params.user_id}`)

// Super admin delete user
export const adminDeleteUser = (params: GetUserByIdParams) => request.del(`${BASE_URL}/users`, params)

// Super admin set admin
export const setAdmin = (query: GetUserByIdParams) => request.post(`${BASE_URL}/users/set_admin`, {}, query)

// Super admin unset admin
export const unSetAdmin = (query: GetUserByIdParams) => request.post(`${BASE_URL}/users/unset_admin`, {}, query)
