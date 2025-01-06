import request from "@/utils/request"
import { BASE_URL } from "@/config"
import { GetUserByIdParams, GetUserListParams } from "@/interfaces/request"

export const adminGetUserList = (params: GetUserListParams) => request.get(`${BASE_URL}/users`, params)

export const adminGetUserById = (params: GetUserByIdParams) => request.get(`${BASE_URL}/users/${params.user_id}`)

export const adminDeleteUser = (params: GetUserByIdParams) => request.del(`${BASE_URL}/users`, params)

export const setAdmin = (query: GetUserByIdParams) => request.post(`${BASE_URL}/users/set_admin`, {}, query)

export const unSetAdmin = (query: GetUserByIdParams) => request.post(`${BASE_URL}/users/unset_admin`, {}, query)
