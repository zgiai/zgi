import { BASE_URL } from "@/config";
import request from "@/utils/request";
import { CreateApiKeyParams, GetApiKeyListParams, GetApiKeyParams, GetProjectParams } from "@/interfaces/request";

// Create api key
export const createApiKey = (params: CreateApiKeyParams, query: GetProjectParams) => request.post(`${BASE_URL}/api-keys/projects/create`, params, query)

// Delete api key
export const deleteApiKey = (params: GetApiKeyParams) => request.del(`${BASE_URL}/api-keys/projects/delete`, params)

// Get api key list
export const getApiKeyList = (params: GetApiKeyListParams) => request.get(`${BASE_URL}/api-keys/projects/list`, params)

// Update api key
export const updateApiKey = (params: CreateApiKeyParams, query: GetApiKeyParams) => request.put(`${BASE_URL}/api-keys/projects/update`, params, query)

// Get api key detail
export const getApiKeyDetail = (params: GetApiKeyParams) => request.get(`${BASE_URL}/api-keys/projects/info`, params)

// Disable api key
export const disableApiKey = (query: GetApiKeyParams) => request.post(`${BASE_URL}/api-keys/projects/disable`, {}, query)

// Enable api key
export const enableApiKey = (query: GetApiKeyParams) => request.post(`${BASE_URL}/api-keys/projects/enable`, {}, query)
