"use client"

import { getApiKey } from "@/services/project";
import { message } from "antd";
import { useParams } from "next/navigation";
import { useState, useEffect } from "react";

export default function ApiKeyPage() {
    const params = useParams();
    const projectId = params.projectId as string || "";
    const [apiKeyList, setApiKeyList] = useState<any>([]);
    const [totalApiKey, setTotalApiKey] = useState<number>(0);

    useEffect(() => {
        init();
    }, []);

    const init = async () => {
        try {
            const res = await getApiKey({ project_id: projectId });
            console.log(res);
            if (res.status_code === 200) {
                setApiKeyList(res?.data?.api_keys);
                setTotalApiKey(res?.data?.total);
            } else {
                message.error(res?.message || "Failed to get api key list");
            }
        } catch (error) {
            console.error(error);
        }
    }

    return <div className="flex flex-col px-4 py-4">
        <div className="flex justify-between p-4 border-b border-gray-200 dark:border-gray-700/60 items-center flex-wrap gap-4">
            <div className="flex-1">
                <span className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Api Keys</span>
            </div>
        </div>
        <div className="bg-white dark:bg-gray-800 shadow-sm rounded-xl relative">
            <header className="px-5 py-4">
                <h2 className="font-semibold text-gray-800 dark:text-gray-100">All Api Keys <span className="text-gray-400 dark:text-gray-500 font-medium">{totalApiKey}</span></h2>
            </header>
            <div>
                {/* Table */}
                <div className="overflow-x-auto">
                    <table className="table-auto w-full dark:text-gray-300">
                        {/* Table header */}
                        <thead className="text-xs font-semibold uppercase text-gray-500 dark:text-gray-400 bg-gray-50 dark:bg-gray-900/20 border-t border-b border-gray-100 dark:border-gray-700/60">
                            <tr>
                                <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                                    <div className="font-semibold text-left">Name</div>
                                </th>
                                <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                                    <div className="font-semibold text-left">Key</div>
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
                            {apiKeyList.map((apiKey: any, index: number) => (
                                <ApiKeyCard key={index} apiKey={apiKey} />
                            ))}
                        </tbody>
                    </table>

                </div>
            </div>
        </div>
    </div>
}

function ApiKeyCard({ apiKey }: { apiKey: any }) {
    return <tr className="">
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">{apiKey.name}</td>
        <td
            className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap hover:bg-gray-100 dark:hover:bg-gray-700/50 cursor-pointer"
            onClick={() => {
                navigator.clipboard.writeText(apiKey.key);
                message.success("Copied to clipboard");
            }}
            title="Click to copy"
        >{apiKey.key}</td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">{apiKey.status}</td>
    </tr>
}