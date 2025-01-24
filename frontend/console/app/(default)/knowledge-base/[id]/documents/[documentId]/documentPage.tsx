"use client"

import { useEffect, useState } from "react";
import { getDocument } from "@/services/knowledgeBase";
import { message } from "antd";
import { formatBytes } from "@/utils/common"

interface Document {
    id?: number;
    kb_id?: number;
    file_name?: string;
    file_type?: string;
    file_size?: number;
    file_hash?: string;
    status?: number;
    error_message?: string | null;
    chunk_count?: number;
    token_count?: number;
    vector_count?: number;
    title?: string | null;
    language?: string | null;
    author?: string | null;
    source_url?: string | null;
    meta_info?: any;
    tags?: string[] | null;
    chunk_size?: number;
    chunk_overlap?: number;
    embedding_model?: string | null;
    created_at?: string;
    updated_at?: string;
    processed_at?: string | null;
}

interface Chunk {
    id?: number;
    kb_id?: number;
    file_name?: string;
    file_type?: string;
    file_size?: number;
    file_hash?: string;
    status?: number;
    error_message?: string | null;
    chunk_count?: number;
    token_count?: number;
    vector_count?: number;
    title?: string | null;
    language?: string | null;
    author?: string | null;
    source_url?: string | null;
    meta_info?: any;
    tags?: string[] | null;
    chunk_size?: number;
    chunk_overlap: number;
    embedding_model?: string | null;
    created_at?: string;
    updated_at?: string;
    processed_at?: string | null;
}

export default function DocumentPage({ kbId, documentId }: { kbId: string, documentId: string }) {

    const [document, setDocument] = useState<Document>({});
    const [chunkList, setChunkList] = useState<Chunk[]>([]);

    const fetchDocument = async () => {
        try {
            const res = await getDocument(documentId);
            if (res?.status_code === 200) {
                setDocument(res.data);
                console.log(res.data);
            } else {
                message.error(res?.message || "Failed to fetch document");
            }
        } catch (err) {
            console.error(err)
        }


    }

    useEffect(() => {
        fetchDocument();
    }, [kbId, documentId]);

    return <div>
        <h1 className="text-2xl font-bold px-4 py-4 text-gray-900 dark:text-gray-100">{document?.title || document?.file_name}</h1>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-x-8 gap-y-4 p-4 border-b border-gray-300 dark:border-gray-700">
            <div className="flex-1 flex flex-col h-full">
                <div className="flex items-center">
                    <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">File Name </span>
                    <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{document?.file_name}</span>
                </div>
            </div>
            <div className="flex-1 flex flex-col h-full">
                <div className="flex items-center h-full">
                    <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">File Type </span>
                    <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{document?.file_type}</span>
                </div>
            </div>
            <div className="flex-1 flex flex-col h-full">
                <div className="flex items-center h-full">
                    <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">File Size </span>
                    <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{formatBytes(document?.file_size || 0)}</span>
                </div>
            </div>
            <div className="flex-1 flex flex-col h-full">
                <div className="flex items-center h-full">
                    <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">Create At </span>
                    <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(document?.created_at || ""))}</span>
                </div>
            </div>
            <div className="flex-1 flex flex-col h-full">
                <div className="flex items-center h-full">
                    <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">Last Update At </span>
                    <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(document?.updated_at || ""))}</span>
                </div>
            </div>
            <div className="flex-1 flex flex-col h-full">
                <div className="flex items-center h-full">
                    <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">File Hash </span>
                    <span className="text-sm text-gray-600 dark:text-gray-400 w-40 break-all">{document?.file_hash}</span>
                </div>
            </div>
        </div>
    </div>;
}