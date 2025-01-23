"use client"

import { getDocumentList } from "@/services/knowledgeBase";
import { useState, useEffect } from "react";
import { message } from 'antd';
import Link from "next/link";
import { formatBytes } from "@/utils/common";
import { DeleteDocumentModal, UpdateDocumentModal } from "./documentsModal";

interface Document {
    id: number;
    kb_id: number;
    file_name: string;
    file_type: string;
    file_size: number;
    file_hash: string;
    status: number;
    error_message: string | null;
    chunk_count: number;
    token_count: number;
    vector_count: number;
    title: string | null;
    language: string | null;
    author: string | null;
    source_url: string | null;
    meta_info: any;
    tags: string[] | null;
    chunk_size: number;
    chunk_overlap: number;
    embedding_model: string | null;
    created_at: string;
    updated_at: string;
    processed_at: string | null;
}

const statusMap = {
    "0": {
        label: "Pending",
        color: "text-gray-500"
    },
    "1": {
        label: "Processing",
        color: "text-yellow-500"
    },
    "2": {
        label: "Available",
        color: "text-green-500"
    },
    "-1": {
        label: "Failed",
        color: "text-red-500"
    },
}

export default function KBPage({ id }: { id: string }) {
    const [documentList, setDocumentList] = useState<Document[]>([]);
    const [total, setTotal] = useState(0);
    const [isDeleteOpen, setIsDeleteOpen] = useState(false);
    const [isUpdateOpen, setIsUpdateOpen] = useState(false);
    const [currentDocument, setCurrentDocument] = useState<Document | null>(null);

    const fetchDocumentList = async () => {
        try {
            const response = await getDocumentList(id);
            if (response?.status_code === 200) {
                setDocumentList(response?.data?.items);
                setTotal(response?.data?.total);
            } else {
                message.error(response?.status_message || "Failed to fetch documents");
            }
        } catch (error) {
            console.error(error);
        }
    }

    useEffect(() => {
        fetchDocumentList();
    }, []);

    return <div>
        <DeleteDocumentModal isOpen={isDeleteOpen} setIsOpen={setIsDeleteOpen} currentDocument={currentDocument} getDocumentList={fetchDocumentList} />
        <UpdateDocumentModal isOpen={isUpdateOpen} setIsOpen={setIsUpdateOpen} currentDocument={currentDocument} getDocumentList={fetchDocumentList} />
        <header className="px-5 py-4 flex flex-row justify-between">
            <h2 className="font-semibold text-gray-800 dark:text-gray-100 flex-nowrap text-nowrap mr-4">
                All Documents <span className="text-gray-400 dark:text-gray-500 font-medium ml-2">{total}</span>
            </h2>
            <div className="flex flex-col md:flex-row gap-2 md:items-center">
                <div className="flex flex-row gap-2 items-center flex-wrap">
                    <Link
                        href={`/knowledge-base/${id}/create`}
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">
                        <svg className="fill-current text-gray-400 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                            <path d="M15 7H9V1c0-.6-.4-1-1-1S7 .4 7 1v6H1c-.6 0-1 .4-1 1s.4 1 1 1h6v6c0 .6.4 1 1 1s1-.4 1-1V9h6c.6 0 1-.4 1-1s-.4-1-1-1z" />
                        </svg>
                        <span className="ml-2">Add Document</span>
                    </Link>
                </div>
            </div>
        </header>
        <div className="overflow-x-auto">
            <table className="table-auto w-full dark:text-gray-300 border-b border-gray-200">
                {/* Table header */}
                <thead className="text-xs font-semibold uppercase text-gray-500 dark:text-gray-400 bg-gray-100 dark:bg-gray-900/20 border-t border-b border-gray-100 dark:border-gray-700/60">
                    <tr>
                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                            <div className="font-semibold text-left">ID</div>
                        </th>
                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                            <div className="font-semibold text-left">Name</div>
                        </th>
                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                            <div className="font-semibold text-left">Type</div>
                        </th>
                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                            <div className="font-semibold text-left">Size</div>
                        </th>
                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                            <div className="font-semibold text-left">Uploaded-At</div>
                        </th>
                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                            <div className="font-semibold text-left">Status</div>
                        </th>
                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                            <div className="font-semibold text-left">Action</div>
                        </th>
                    </tr>
                </thead>
                {/* Table body */}
                <tbody className="text-sm divide-y divide-gray-100 dark:divide-gray-700/60">
                    {documentList.length === 0 && <tr>
                        <td colSpan={5} className="text-center py-4">No data</td>
                    </tr>}
                    {documentList.map((document: any, index: number) => (
                        <DocumentTableRow key={index} document={document} setIsDeleteOpen={setIsDeleteOpen} setIsUpdateOpen={setIsUpdateOpen} setCurrentDocument={setCurrentDocument} />
                    ))}
                </tbody>
            </table>
        </div>
    </div>;
}

function DocumentTableRow({ document, setIsDeleteOpen, setIsUpdateOpen, setCurrentDocument }: { document: Document, setIsDeleteOpen: (open: boolean) => void, setIsUpdateOpen: (open: boolean) => void, setCurrentDocument: (document: Document) => void }) {
    return <tr className="hover:bg-gray-50 dark:hover:bg-gray-900 cursor-pointer">
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{document.id}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left overflow-hidden text-ellipsis max-w-[200px]">{document.title || document.file_name}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{document.file_type}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{formatBytes(document.file_size)}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(document.created_at))}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className={`text-left font-medium ${statusMap[String(document.status) as keyof typeof statusMap].color}`}>{statusMap[String(document.status) as keyof typeof statusMap].label}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="flex flex-row gap-2">
                <button
                    className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600"
                    onClick={() => {
                        setIsUpdateOpen(true);
                        setCurrentDocument(document);
                    }}
                >
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M11.7.3c-.4-.4-1-.4-1.4 0l-10 10c-.2.2-.3.4-.3.7v4c0 .6.4 1 1 1h4c.3 0 .5-.1.7-.3l10-10c.4-.4.4-1 0-1.4l-4-4zM4.6 14H2v-2.6l6-6L10.6 8l-6 6zM12 6.6L9.4 4 11 2.4 13.6 5 12 6.6z" />
                    </svg>
                </button>
                <button
                    className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600"
                    onClick={() => {
                        setIsDeleteOpen(true);
                        setCurrentDocument(document);
                    }}
                >
                    <svg className="fill-current text-red-500 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M5 7h2v6H5V7zm4 0h2v6H9V7zm3-6v2h4v2h-1v10c0 .6-.4 1-1 1H2c-.6 0-1-.4-1-1V5H0V3h4V1c0-.6.4-1 1-1h6c.6 0 1 .4 1 1zM6 2v1h4V2H6zm7 3H3v9h10V5z" />
                    </svg>
                </button>
            </div>

        </td>
    </tr>
}