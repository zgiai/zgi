"use client"

import { useEffect, useState } from "react";
import { getChunkList, getDocument, updateChunk } from "@/services/knowledgeBase";
import { message } from "antd";
import { formatBytes } from "@/utils/common"
import PaginationClassic from "@/components/pagination-classic";
import PaginationLibrary from "@/app/(alternative)/components-library/pagination/page";
import PaginationNumeric from "@/components/pagination-numeric";
import ChunksModal from "./chunksModal";

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
    chunk_index: number,
    chunk_meta_info: any,
    content: string,
    created_at: string,
    document_id: number,
    embedding_model: string | null,
    id: number,
    token_count: number,
    updated_at: string,
    vector_id: string | null
}

export default function DocumentPage({ kbId, documentId }: { kbId: string, documentId: string }) {

    const [document, setDocument] = useState<Document>({
        created_at: undefined,
        updated_at: undefined,
    });
    const [chunkList, setChunkList] = useState<Chunk[]>([]);
    const [total, setTotal] = useState<number>(0);
    const [currentPage, setCurrentPage] = useState<number>(1);
    const [documentInfoOpen, setDocumentInfoOpen] = useState<boolean>(false);
    const [isChunksModalOpen, setIsChunksModalOpen] = useState<boolean>(false);
    const [currentChunk, setCurrentChunk] = useState<Chunk | null>(null);
    const [searchQuery, setSearchQuery] = useState<string>("");

    const pageSize = 10;

    const fetchDocument = async () => {
        try {
            const res = await getDocument(documentId);
            if (res?.status_code === 200) {
                setDocument(res?.data || {});
            } else {
                message.error(res?.status_message || "Failed to fetch document");
            }
        } catch (err) {
            console.error(err)
        }
    }

    const fetchChunkList = async () => {
        try {
            const res = await getChunkList(documentId, {
                page_num: currentPage,
                page_size: pageSize,
                search: searchQuery
            });
            if (res?.status_code === 200) {
                setChunkList(res?.data?.items || []);
                setTotal(res?.data?.total || 0);
            } else {
                message.error(res?.status_message || "Failed to fetch chunk list");
            }
        } catch (err) {
            console.error(err)
        }
    }

    const saveChunk = async (content: string) => {
        if (!currentChunk){
            message.error("Please select a chunk");
            return false
        }
        try {
            const res = await updateChunk(currentChunk?.id, { content })
            if (res?.status_code === 200) {
                message.success("Update chunk success");
                fetchChunkList();
                return true
            } else {
                message.error(res?.status_message || "Failed to update chunk");
                return false
            }
        } catch (err) {
            console.error(err)
            return false
        }
    }

    useEffect(() => {
        fetchDocument();
    }, [documentId]);

    useEffect(() => {
        fetchChunkList();
    }, [currentPage, pageSize, documentId]);



    return <div>
        <ChunksModal isOpen={isChunksModalOpen} onClose={() => setIsChunksModalOpen(false)} chunk={currentChunk || {}} onSave={saveChunk} />
        <h1 className="text-2xl font-bold px-4 pt-4 pb-2 text-gray-900 dark:text-gray-100">{document?.title || document?.file_name}</h1>
        <div className="flex flex-col border-b border-gray-300 dark:border-gray-700">
            <div className={`grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-x-8 gap-y-4 p-4 ${documentInfoOpen ? 'hidden' : 'grid'}`}>
                <div className="flex-1 flex flex-col h-full">
                    <div className="flex items-center">
                        <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">File Name </span>
                        <span className="text-sm text-gray-600 dark:text-gray-400 w-40 break-all">{document?.file_name}</span>
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
                        <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{document?.created_at ? new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(document?.created_at)) : ""}</span>
                    </div>
                </div>
                <div className="flex-1 flex flex-col h-full">
                    <div className="flex items-center h-full">
                        <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">Last Update At </span>
                        <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{document?.updated_at ? new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(document?.updated_at)) : ""}</span>
                    </div>
                </div>
                <div className="flex-1 flex flex-col h-full">
                    <div className="flex items-center h-full">
                        <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">File Hash </span>
                        <span className="text-sm text-gray-600 dark:text-gray-400 w-40 break-all">{document?.file_hash}</span>
                    </div>
                </div>
                <div className="flex-1 flex flex-col h-full">
                    <div className="flex items-center h-full">
                        <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">Chunk Size </span>
                        <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{document?.chunk_size}</span>
                    </div>
                </div>
                <div className="flex-1 flex flex-col h-full">
                    <div className="flex items-center h-full">
                        <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">Chunk Overlap </span>
                        <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{document?.chunk_overlap}</span>
                    </div>
                </div>
                <div className="flex-1 flex flex-col h-full">
                    <div className="flex items-center h-full">
                        <span className="text-sm text-gray-800 dark:text-gray-200 flex-1 font-bold">Chunk Count </span>
                        <span className="text-sm text-gray-600 dark:text-gray-400 w-40">{document?.chunk_count}</span>
                    </div>
                </div>
            </div>
            <div className="flex items-center justify-center">
                <button
                    className="flex items-center justify-center w-full hover:bg-gray-100 dark:hover:bg-gray-700"
                    onClick={() => setDocumentInfoOpen(!documentInfoOpen)}
                >
                    <svg className={`w-4 h-4 shrink-0 fill-current text-gray-800 dark:text-gray-200 ${!documentInfoOpen ? 'rotate-180 mt-1':'mb-1'}`} viewBox="0 0 12 12">
                        <path d="M5.9 11.4L.5 6l1.4-1.4 4 4 4-4L11.3 6z" />
                    </svg>
                </button>
            </div>
        </div>
        <ChunkListComponent
            chunkList={chunkList}
            total={total}
            setCurrentChunk={setCurrentChunk}
            setIsChunksModalOpen={setIsChunksModalOpen}
            searchQuery={searchQuery}
            setSearchQuery={setSearchQuery}
            fetchChunkList={fetchChunkList}
            currentPage={currentPage}
            setCurrentPage={setCurrentPage}
        />
        <div className="flex items-center justify-center py-4">
            <PaginationNumeric
                current={currentPage}
                pageSize={pageSize}
                total={total}
                onChange={setCurrentPage}
            />
        </div>
    </div>;
}

function ChunkListComponent({
    chunkList, total, setCurrentChunk, setIsChunksModalOpen, searchQuery, setSearchQuery, fetchChunkList, currentPage, setCurrentPage
}: {
    chunkList: Chunk[], total: number, setCurrentChunk: (chunk: Chunk) => void, setIsChunksModalOpen: (isOpen: boolean) => void, searchQuery: string, setSearchQuery: (query: string) => void, fetchChunkList: () => void, currentPage: number, setCurrentPage: (page: number) => void
}) {
    const [showMore, setShowMore] = useState<boolean>(false);

    return <div>
        <h1 className=" font-bold px-4 py-4 text-gray-900 dark:text-gray-100 flex items-center justify-between">
            <div>
                <span className="text-gray-800 dark:text-gray-200 mr-2 text-2xl">All Chunks</span>
                <span className="text-gray-600 dark:text-gray-400 text-lg">{total}</span>
            </div>
            <div className="flex items-center gap-x-2">
                <input
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    placeholder="Search chunks"
                    className="form-input font-normal"
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                            if (currentPage === 1) {
                                fetchChunkList()
                            } else {
                                setCurrentPage(1)
                            }
                        }
                    }}
                />
                <button
                    className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                    onClick={() => setShowMore(!showMore)}
                >
                    <span className="hidden sm:inline">View All</span>
                </button>
            </div>
        </h1>
        <div className="flex flex-col gap-y-2 p-4">
            {chunkList.map((chunk) => (
                <div key={chunk?.id}
                    className="flex-1 flex flex-col h-full rounded-md border border-gray-300 dark:border-gray-700 p-4 shadow-md bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer"
                    onClick={
                        () => {
                            setCurrentChunk(chunk)
                            setIsChunksModalOpen(true)
                        }
                    }
                >
                    <div className="flex items-center h-full gap-x-4">
                        <span className="text-sm text-gray-600 dark:text-gray-400">Chunk-{chunk?.chunk_index + 1}</span>
                        <span className="text-sm text-gray-600 dark:text-gray-400">{chunk?.token_count} tokens</span>
                    </div>
                    <div className="mt-3">
                        <span className={`text-gray-800 dark:text-gray-200 break-all overflow-hidden text-ellipsis ${showMore ? '' : 'line-clamp-2'}`}>{chunk?.content}</span>
                    </div>
                </div>
            ))}
        </div>
    </div>;
}
