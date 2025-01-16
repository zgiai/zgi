import { BASE_URL } from "@/config";
import request from "@/utils/request";

export const getKnowledgeBaseList = (params: {
    organization_id?: string;
    page_num?: number;
    page_size?: number;
}) => request.get(`${BASE_URL}/knowledge`, params);

export const getKnowledgeBaseDetail = (params: {
    kb_id: string;
}) => request.get(`${BASE_URL}/knowledge/${params.kb_id}`);

export const createKnowledgeBase = (params: {
    name: string;
    discription: string;
}) => request.post(`${BASE_URL}/knowledge`, params);

export const updateKnowledgeBase = (id: string, params: {
    name: string;
    discription: string;
}) => request.put(`${BASE_URL}/knowledge/${id}`, params);

export const deleteKnowledgeBase = (params: {
    kb_id: string;
}) => request.del(`${BASE_URL}/knowledge/${params.kb_id}`);

export const cloneKnowledgeBase = (id: string, params: {
    name: string;
}) => request.post(`${BASE_URL}/knowledge/${id}/clone`,params);

