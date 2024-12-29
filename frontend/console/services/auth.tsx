import request from "@/utils/request"
import { BASE_URL } from "@/config"
import { LoginParams,RegisterParams } from "@/interfaces/request";

export const register = (params: RegisterParams) => request.post(`${BASE_URL}/register`, params);

export const login = (params: LoginParams) => request.post(`${BASE_URL}/login`, params);

export const getUserInfo = () => request.get(`${BASE_URL}/me`);
