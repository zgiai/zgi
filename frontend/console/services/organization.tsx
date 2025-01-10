import { BASE_URL } from "@/config";
import request from "@/utils/request";
import { UpdateOrganizationParams, GetOrganizationParams, CreateOrganizationParams, GetOrgPermissionParams, GetOrgMembersListParams, SetOrgAdminParams } from "@/interfaces/request";

// Get organization list
export const getOrganization = () => request.get(`${BASE_URL}/organizations/list`)

// Get organization details
export const getOrganizationDetail = (params: GetOrganizationParams) => request.get(`${BASE_URL}/organizations/info`, params)

// Update organization information
export const updateOrganization = (query: GetOrganizationParams, params: UpdateOrganizationParams) => request.put(`${BASE_URL}/organizations/update`, params, query)

// Delete organization
export const deleteOrganization = (params: GetOrganizationParams) => request.del(`${BASE_URL}/organizations/delete`, params)

// Create organization
export const createOrganization = (params: CreateOrganizationParams) => request.post(`${BASE_URL}/organizations/create`, params)

// Get current organization permissions
export const getOrgPermission = (params: GetOrgPermissionParams) => request.get(`${BASE_URL}/organizations/members/me`, params)

// Get organization members list
export const getOrgMembersList = (params: GetOrgMembersListParams) => request.get(`${BASE_URL}/organizations/members/list`, params)

// Set organization administrator
export const setOrgAdmin = (params: SetOrgAdminParams) => request.post(`${BASE_URL}/organizations/set_admins`, params)

// Remove organization administrator
export const unsetOrgAdmin = (params: SetOrgAdminParams) => request.post(`${BASE_URL}/organizations/unset_admins`, params)

// Query user by email
export const searchUserByEmail = (params: { organization_id: string, email: string }) => request.get(`${BASE_URL}/organizations/search_user`, params)

// Add organization member
export const addOrgMember = (params: { organization_id: string; user_ids: string[] }) => request.post(`${BASE_URL}/organizations/set_members`, params)

// Remove organization member
export const removeOrgMember = (params: { organization_id: string; user_id: string }) => request.post(`${BASE_URL}/organizations/unset_members`, params)

// Invite organization member
export const inviteOrgMember = (params: { organization_id: string; }) => request.post(`${BASE_URL}/organizations/invite`, params)

// Accept invite
export const acceptInvite = (params: { invite_token: string; }) => request.post(`${BASE_URL}/organizations/accept_invite`, params)
