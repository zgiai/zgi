"use client";

import { useState } from "react";
import { hitTest } from "@/services/knowledgeBase";
import { message } from "antd";
import { SettingModal } from "./settingModal";

interface Chunk {
    id: string;
    score: number;
    chunk_index: number;
    document_id: number;
    text: string;
}

export default function Page({ kb_id }: { kb_id: string }) {
    const [searchText, setSearchText] = useState("");
    const [hitResult, setHitResult] = useState<Chunk[]>([]);
    const [topK, setTopK] = useState<number>(2);
    const [currentChunk, setCurrentChunk] = useState<Chunk | null>(null);
    const [isChunksModalOpen, setIsChunksModalOpen] = useState<boolean>(false);
    const [searchFlag, setSearchFlag] = useState<boolean>(false)

    const [isLoading, setIsLoading] = useState<boolean>(false)

    const handleSearch = async () => {
        setIsLoading(true)
        if (searchText.trim() === '') {
            setIsLoading(false)
            message.error('Search text cannot be empty')
            return
        }
        if (!searchFlag) setSearchFlag(true)
        try {
            const res = await hitTest(parseInt(kb_id), {
                text: searchText,
                top_k: topK
            })
            if (res?.status_code === 200) {
                setHitResult(res?.data || [])
            } else {
                message.error(res?.status_message || 'Failed to search')
            }
        } catch (error) {
            console.log(error)
            setIsLoading(false)
        } finally {
            setIsLoading(false)
        }
    }

    return (
        <>
            <SettingModal isOpen={isChunksModalOpen} setIsOpen={setIsChunksModalOpen} topK={topK} setTopK={setTopK} />
            <div className="w-full h-full min-h-[80vh] flex flex-col">
                <h1 className="text-2xl text-gray-800 dark:text-gray-100 px-4 font-bold mt-4 flex justify-between">
                    <span>Hit Test</span>
                    <button
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white disabled:bg-gray-700 disabled:text-gray-100 disabled:hover:bg-gray-600 disabled:hover:text-gray-100"
                        disabled={isLoading}
                        onClick={() => { setIsChunksModalOpen(true) }}
                    >{"Search Settings"}</button>
                </h1>
                <div className="p-4 flex gap-4 flex-col lg:flex-row w-full h-full flex-1">
                    <div className="flex-[2] h-full">

                        <div className="flex flex-col gap-2">
                            <div>Search Text</div>
                            <textarea
                                className="form-input w-full px-2 py-1 min-h-[200px]"
                                value={searchText}
                                onChange={(e) => setSearchText(e.target.value)}
                            />
                            <div className="flex justify-end mt-2">
                                <button
                                    className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white disabled:bg-gray-700 disabled:text-gray-100 disabled:hover:bg-gray-600 disabled:hover:text-gray-100"
                                    disabled={isLoading}
                                    onClick={handleSearch}
                                >{isLoading ? "Searching..." : "Search"}</button>
                            </div>
                        </div>

                    </div>
                    <div className="flex-[3] bg-gray-100 h-full rounded-xl w-full flex flex-col px-2 pb-2 gap-2 max-h-[80vh] overflow-y-auto">
                        {!searchFlag && (
                            <div className="w-full h-full flex items-center justify-center text-gray-500 text-xl">
                                search result will be displayed here
                            </div>
                        )}
                        {
                            searchFlag && (
                                <div className="flex items-center p-4">
                                    Total hits:<span className="font-bold">{isLoading ? "Loading..." : hitResult.length}</span>
                                </div>
                            )
                        }
                        {!isLoading && hitResult.map((chunk) => (
                            <div key={chunk?.id}
                                className="flex flex-col rounded-md border border-gray-300 dark:border-gray-700 p-4 shadow-md bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 cursor-pointer"
                                onClick={
                                    () => {

                                    }
                                }
                            >
                                <div className="flex items-center h-full gap-x-4">
                                    <span className="text-sm text-gray-600 dark:text-gray-400">Chunk-{chunk?.chunk_index - 1}</span>
                                    <span className="text-sm text-gray-600 dark:text-gray-400">SCORE:<span className="font-bold">{chunk?.score.toFixed(2)}</span></span>
                                </div>
                                <div className="mt-3">
                                    <span className={`text-gray-800 dark:text-gray-200 break-all overflow-hidden text-ellipsis`}>{chunk?.text}</span>
                                </div>
                            </div>
                        ))}
                    </div>
                </div>
            </div>
        </>
    );
}