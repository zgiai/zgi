'use client';

import { motion } from 'framer';
import { FC, useState, useEffect } from 'react';
import Link from 'next/link';
import { getKnowledgeBaseList } from '@/services/knowledgeBase';
import { AddKbsModal } from './kbsModal';

interface KnowledgeBase {
    id: number;
    name: string;
    description: string;
    visibility: "PUBLIC" | "PRIVATE";
    status: number;
    collection_name: string;
    model: string;
    dimension: number;
    document_count: number;
    total_chunks: number;
    total_tokens: number;
    meta_info: any;
    tags: string[] | null;
    owner_id: number;
    organization_id: number | null;
    created_at: string;
    updated_at: string;
    owner_name: string;
}

interface KnowledgeBaseCardProps {
    kb: KnowledgeBase;
    index: number;
}

const KnowledgeBaseCard: FC<KnowledgeBaseCardProps> = ({ kb, index }) => {
    return (
        <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.3, delay: index * 0.1 }}
            whileHover={{ scale: 1.02 }}
            whileTap={{ scale: 0.98 }}
            className="bg-white rounded-lg shadow-md p-6 hover:shadow-lg cursor-pointer"
        >
            <h3 className="text-xl font-semibold text-gray-800">{kb.name}</h3>
            <div className="flex gap-2 text-sm text-gray-500 mb-2">
                <span>{kb?.document_count} files</span>
                <span>{kb?.total_tokens}tokens</span>
            </div>
            <p className="text-gray-600 mb-4">{kb.description}</p>
            <div className="flex flex-wrap gap-2 mb-4">
                {kb?.tags?.map((tag, idx) => (
                    <span
                        key={idx}
                        className="px-3 py-1 bg-blue-100 text-blue-600 rounded-full text-sm"
                    >
                        {tag}
                    </span>
                ))}
            </div>
        </motion.div>
    );
};

// const formatBytes = (bytes: number): string => {
//     if (bytes === 0) return '0 Bytes';
//     const k = 1024;
//     const sizes = ['Bytes', 'KB', 'MB', 'GB'];
//     const i = Math.floor(Math.log(bytes) / Math.log(k));
//     return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
// };

const KnowledgeBasePage: FC = () => {
    const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([]);
    const [createModalOpen, setCreateModalOpen] = useState(false);
    const [editModalOpen, setEditModalOpen] = useState(false);
    const [deleteModalOpen, setDeleteModalOpen] = useState(false);

    const fetchKnowledgeBases = async () => {
        try {
            const res = await getKnowledgeBaseList({ page_num: 1, page_size: 100 });
            if (res?.status_code === 200) {
                setKnowledgeBases(res?.data?.items || []);
            }
        } catch (error) {
            console.error('Error fetching knowledge bases:', error);
        }
    };
    useEffect(() => {
        fetchKnowledgeBases();
    }, []);

    return (
        <>
            <AddKbsModal isOpen={createModalOpen} setIsOpen={setCreateModalOpen} getKbsList={fetchKnowledgeBases} />
            <div className="min-h-screen bg-gray-50">
                <motion.div
                    initial={{ opacity: 0, y: -20 }}
                    animate={{ opacity: 1, y: 0 }}
                    className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12"
                >
                    <motion.h1
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        className="text-4xl font-bold text-gray-900 mb-8 flex flex-col md:flex-row justify-between"
                    >
                        <span className="mr-2">Knowledge Base</span>
                        <button
                            className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                            onClick={() => setCreateModalOpen(true)}
                        >
                            <svg className="fill-current text-gray-400 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                                <path d="M15 7H9V1c0-.6-.4-1-1-1S7 .4 7 1v6H1c-.6 0-1 .4-1 1s.4 1 1 1h6v6c0 .6.4 1 1 1s1-.4 1-1V9h6c.6 0 1-.4 1-1s-.4-1-1-1z" />
                            </svg>
                            <span className="ml-2">New Knowledge Base</span>
                        </button>
                    </motion.h1>

                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                        {knowledgeBases.length === 0 && (
                            <motion.div
                                initial={{ opacity: 0, y: 20 }}
                                animate={{ opacity: 1, y: 0 }}
                                transition={{ duration: 0.3 }}
                                whileHover={{ scale: 1.02 }}
                                whileTap={{ scale: 0.98 }}
                                className="bg-white rounded-lg shadow-md p-6 hover:shadow-lg cursor-pointer flex items-center justify-center gap-4"
                                onClick={() => setCreateModalOpen(true)}
                            >
                                <span className='text-gray-600'>
                                    <svg viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" width="52" height="52"><path d="M853.333333 256H298.666667c-23.466667 0-42.666667 19.2-42.666667 42.666667v554.666666c0 23.466667 19.2 42.666667 42.666667 42.666667h554.666666c23.466667 0 42.666667-19.2 42.666667-42.666667V298.666667c0-23.466667-19.2-42.666667-42.666667-42.666667z m-106.666666 362.666667h-128v128h-85.333334v-128h-128v-85.333334h128v-128h85.333334v128h128v85.333334z" fill="currentColor" p-id="4273"></path><path d="M170.666667 170.666667h512v42.666666h42.666666V170.666667c0-23.466667-19.2-42.666667-42.666666-42.666667H170.666667c-23.466667 0-42.666667 19.2-42.666667 42.666667v512c0 23.466667 19.2 42.666667 42.666667 42.666666h42.666666v-42.666666H170.666667V170.666667z" fill="currentColor"></path></svg>
                                </span>
                                <div className='flex justify-center flex-col'>
                                    <p className="text-gray-600">No knowledge bases found.</p>
                                    <span className="text-gray-600">Create a knowledge base now.</span>
                                </div>
                            </motion.div>
                        )}
                        {knowledgeBases.map((kb, index) => (
                            <KnowledgeBaseCard key={kb.id} kb={kb} index={index} />
                        ))}
                    </div>
                </motion.div>
            </div>
        </>
    );
};

export default KnowledgeBasePage;