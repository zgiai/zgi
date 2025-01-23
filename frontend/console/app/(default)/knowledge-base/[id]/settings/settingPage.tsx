"use client"

import { updateKnowledgeBase, getKnowledgeBaseDetail } from "@/services/knowledgeBase";
import { message } from "antd";
import { useState, useEffect } from "react";

export default function SettingPage({ id }: { id: string }) {
    const [loading, setLoading] = useState(false);
    const [kbName, setKbName] = useState("");
    const [kbDescription, setKbDescription] = useState("");

    const fetchKnowledgeBase = async () => {
        try {
            const res = await getKnowledgeBaseDetail({ kb_id: id });
            if (res.status_code === 200) {
                setKbName(res.data.name);
                setKbDescription(res.data.description);
            }
        } catch (error) {
            console.error(error);
        }
    }

    useEffect(() => {
        fetchKnowledgeBase();
    }, []);

    const handleUpdateKb = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await updateKnowledgeBase(id, { name: kbName, description: kbDescription });
            if (res.status_code === 200) {
                message.success("Update knowledge base success");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <div>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold px-5 py-4">Knowledge Base Setting</div>
        <form className="flex flex-col gap-4 px-5 mb-4" onSubmit={handleUpdateKb}>
            <div className="flex gap-2 sm:items-center flex-col sm:flex-row text-nowrap">
                <label htmlFor="name" className="text-gray-800 dark:text-gray-100 sm:w-1/4 sm:text-right">Knowledge Base Name</label>
                <input id="name" className="form-input flex-1" placeholder="Knowledge Base Name" type="text" value={kbName} onChange={(e) => setKbName(e.target.value.trim())} />
            </div>
            <div className="flex gap-2 flex-col sm:flex-row text-nowrap">
                <label htmlFor="description" className="text-gray-800 dark:text-gray-100 sm:w-1/4 sm:text-right mt-2 sm:mt-0">Knowledge Base Description</label>
                <textarea id="description" className="form-input flex-1 min-h-24" placeholder="Description" value={kbDescription} onChange={(e) => setKbDescription(e.target.value.trim())} />
            </div>
            <div className="flex justify-end gap-4">
                <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white px-8" type="submit" disabled={loading}>{loading ? "Saving..." : "Save"}</button>
            </div>
        </form>
    </div>
}