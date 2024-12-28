import { BASE_URL } from "@/config";
import request from "@/utils/request";
import { UpdateOrganizationParams, GetOrganizationParams, CreateOrganizationParams } from "@/interfaces/request";

// 获取组织列表
export const getOrganization = () => request.get(`${BASE_URL}/organizations/list`)

// 获取组织详情
export const getOrganizationDetail = (params: GetOrganizationParams) => request.get(`${BASE_URL}/organizations/info`, params)

// 更新组织信息
export const updateOrganization = (query: GetOrganizationParams, params: UpdateOrganizationParams) => request.put(`${BASE_URL}/organizations/update`, params, query)

// 删除组织
export const deleteOrganization = (params: GetOrganizationParams) => request.del(`${BASE_URL}/organizations/delete`, params)

// 创建组织
export const createOrganization = (params: CreateOrganizationParams) => request.post(`${BASE_URL}/organizations/create`, params)
