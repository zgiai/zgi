import { BASE_URL } from "@/config";
import request from "@/utils/request";
import { CreateApiKeyParams, GetApiKeyListParams, GetApiKeyParams, GetProjectParams } from "@/interfaces/request";

// 创建api key
export const createApiKey = (params: CreateApiKeyParams, query: GetProjectParams) => request.post(`${BASE_URL}/api-keys/projects/create`, params, query)

// 删除api key
export const deleteApiKey = (params: GetApiKeyParams) => request.del(`${BASE_URL}/api-keys/projects/delete`, params)

// 获取api key列表
export const getApiKeyList = (params: GetApiKeyListParams) => request.get(`${BASE_URL}/api-keys/projects/list`, params)

// 更新api key
export const updateApiKey = (params: CreateApiKeyParams, query: GetApiKeyParams) => request.put(`${BASE_URL}/api-keys/projects/update`, params, query)

// 获取api key详情
export const getApiKeyDetail = (params: GetApiKeyParams) => request.get(`${BASE_URL}/api-keys/projects/info`, params)

// 禁用api key
export const disableApiKey = (params: GetApiKeyParams) => request.post(`${BASE_URL}/api-keys/projects/disable`, params)

// 启用api key
export const enableApiKey = (params: GetApiKeyParams) => request.put(`${BASE_URL}/api-keys/projects/enable`, params)

