import { BASE_URL } from "@/config";
import request from "@/utils/request";

export const getKnowledgeBaseList = (params: {
    organization_id?: string;
    page_num?: number;
    page_size?: number;
    query_name?: string;
}) => request.get(`${BASE_URL}/knowledge`, params);

export const getKnowledgeBaseDetail = (params: {
    kb_id: string;
}) => request.get(`${BASE_URL}/knowledge/${params?.kb_id}`);

export const createKnowledgeBase = (params: {
    name: string;
    description: string;
}) => request.post(`${BASE_URL}/knowledge/create`, params);

export const updateKnowledgeBase = (kb_id: string, params: {
    name: string;
    description: string;
}) => request.put(`${BASE_URL}/knowledge/${kb_id}`, params);

export const deleteKnowledgeBase = (params: {
    kb_id: string;
}) => request.del(`${BASE_URL}/knowledge/${params?.kb_id}`);

export const cloneKnowledgeBase = (kb_id: string, params: {
    name: string;
}) => request.post(`${BASE_URL}/knowledge/${kb_id}/clone`, params);

export const getDocumentList = (kb_id: string, params?: {
    page_num?: number;
    page_size?: number;
    search?:string;
    file_type?:string;
    status?:string;
}) => request.get(`${BASE_URL}/knowledge/${kb_id}/documents`, params);

export const getDocument = (doc_id: string) => request.get(`${BASE_URL}/knowledge/documents/${doc_id}`);

export const deleteDocument = (doc_id: number) => request.del(`${BASE_URL}/knowledge/documents/${doc_id}`);

export const updateDocument = (doc_id: number, params: {
    title?: string,
    language?: string,
    author?: "",
    source_url?: "",
    metadata?: {},
    tags?: []
}) => request.put(`${BASE_URL}/knowledge/documents/${doc_id}`, params);

export const getChunkList = (doc_id: string, params?: {
    page_num?: number;
    page_size?: number;
    search?: string;
}) => request.get(`${BASE_URL}/knowledge/documents/${doc_id}/chunks`, params);

export const getChunk = (chunk_id: string) => request.get(`${BASE_URL}/knowledge/chunks/${chunk_id}`);

export const updateChunk = (chunk_id: number, params: {
    content: string;
}) => request.put(`${BASE_URL}/knowledge/documents/chunks/${chunk_id}`, params);

export const hitTest = (kb_id: number, params: {
    text: string;
    top_k?: number;
}) => request.post(`${BASE_URL}/knowledge/${kb_id}/search`,params);

