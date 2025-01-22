"use client"

import { getDocumentList } from "@/services/knowledgeBase";
import { useState, useEffect } from "react";
import { message } from 'antd';
import Link from "next/link";
import { formatBytes } from "@/utils/common";


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

export default function KBPage({ id }: { id: string }) {
    const [documentList, setDocumentList] = useState<Document[]>([]);
    const [total, setTotal] = useState(0);

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
                            <div className="font-semibold text-left">Size</div>
                        </th>
                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                            <div className="font-semibold text-left">Uploaded-Time</div>
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
                        <DocumentTableRow key={index} document={document} />
                    ))}
                </tbody>
            </table>
        </div>
    </div>;
}

function DocumentTableRow({ document }: { document: Document }) {
    return <tr className="hover:bg-gray-50 dark:hover:bg-gray-900 cursor-pointer">
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{document.id}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{document.file_name}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{formatBytes(document.file_size)}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(document.created_at))}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{document.status}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">

        </td>
    </tr>
}