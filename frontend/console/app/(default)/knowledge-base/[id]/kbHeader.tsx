"use client"

import { useEffect, useState } from "react"
import { useKnowledgeBase } from "./knowledgeProvider"
import { getKnowledgeBaseDetail } from "@/services/knowledgeBase"
import { message } from "antd"

export default function KbHeader({ kbId }: { kbId: string }) {
    const { knowledgeBase, setKnowledgeBase, update } = useKnowledgeBase()

    const fetchKbData = async () => {
        const response = await getKnowledgeBaseDetail({ kb_id: kbId });
        try {
            if (response?.status_code === 200) {
                setKnowledgeBase(response?.data);
            } else {
                message.error(response?.status_message || "Failed to fetch knowledge base");
            }
        } catch (error) {
            console.error(error);
        }
    }

    useEffect(() => {
        fetchKbData();
    }, [update])

    return <div className="p-4 sm:px-6 border-b border-gray-200 dark:border-gray-700">
    <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">{knowledgeBase?.name}</h1>
    <div className="mt-2 flex items-center">
        <span className="text-gray-400 dark:text-gray-500 mr-2">{knowledgeBase?.description}</span>
    </div>
</div>
}