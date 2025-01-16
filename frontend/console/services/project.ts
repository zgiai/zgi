import { BASE_URL } from "@/config";
import request from "@/utils/request";
import { UpdateProjectParams, CreateProjectParams, ListProjectParams, GetProjectParams } from "@/interfaces/request";

// Get project list
export const getProject = (params: ListProjectParams) => request.get(`${BASE_URL}/projects/list`, params)

// Get project detail
export const getProjectDetail = (params: GetProjectParams) => request.get(`${BASE_URL}/projects/info`, params)

// Update project information
export const updateProject = (query: GetProjectParams, params: UpdateProjectParams) => request.put(`${BASE_URL}/projects/update`, params, query)

// Delete project
export const deleteProject = (params: GetProjectParams) => request.del(`${BASE_URL}/projects/delete`, params)

// Create project
export const createProject = (params: CreateProjectParams) => request.post(`${BASE_URL}/projects/create`, params)
