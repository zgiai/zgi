import { BASE_URL } from "@/config";
import request from "@/utils/request";
import { UpdateProjectParams, GetProjectParams, CreateProjectParams, ListProjectParams } from "@/interfaces/request";

// 获取项目列表
export const getProject = (params: ListProjectParams) => request.get(`${BASE_URL}/projects/list`, params)

// 获取项目详情
export const getProjectDetail = (params: GetProjectParams) => request.get(`${BASE_URL}/projects/info`, params)

// 更新项目信息
export const updateProject = (query: GetProjectParams, params: UpdateProjectParams) => request.put(`${BASE_URL}/projects/update`, params, query)

// 删除项目
export const deleteProject = (params: GetProjectParams) => request.del(`${BASE_URL}/projects/delete`, params)

// 创建项目
export const createProject = (params: CreateProjectParams) => request.post(`${BASE_URL}/projects/create`, params)

// 获取api key列表
export const getApiKey = (params: GetProjectParams) => request.get(`${BASE_URL}/api-keys/projects/list`, params)
