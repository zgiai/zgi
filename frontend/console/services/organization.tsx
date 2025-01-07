import { BASE_URL } from "@/config";
import request from "@/utils/request";
import { UpdateOrganizationParams, GetOrganizationParams, CreateOrganizationParams, GetOrgPermissionParams, GetOrgMembersListParams, SetOrgAdminParams } from "@/interfaces/request";

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

// 获取当前组织权限
export const getOrgPermission = (params: GetOrgPermissionParams) => request.get(`${BASE_URL}/organizations/members/me`, params)

// 获取组织成员列表
export const getOrgMembersList = (params: GetOrgMembersListParams) => request.get(`${BASE_URL}/organizations/members/list`, params)

// 设置组织管理员
export const setOrgAdmin = (params: SetOrgAdminParams) => request.post(`${BASE_URL}/organizations/set_admin`, params)


