"use client"

import PaginationClassic from "@/components/pagination-classic";
import PaginationNumeric from "@/components/pagination-numeric";
import { getApiKeyList } from "@/services/apikey";
import { message } from "antd";
import { useParams } from "next/navigation";
import { useState, useEffect } from "react";
import { DeleteApiKeyModal, UpdateApiKeyModal, DisableApiKeyModal, EnableApiKeyModal, CreateApiKeyModal } from "./apikeyModal";

export default function ApiKeyPage() {
    const params = useParams();
    const projectId = params.projectId as string || "";
    const [apiKeyList, setApiKeyList] = useState<any>([]);
    const [totalApiKey, setTotalApiKey] = useState<number>(0);
    const [currentPage, setCurrentPage] = useState<number>(1);
    const [isOpenCreateApiKeyModal, setIsOpenCreateApiKeyModal] = useState<boolean>(false);
    const [isOpenDeleteApiKeyModal, setIsOpenDeleteApiKeyModal] = useState<boolean>(false);
    const [isOpenUpdateApiKeyModal, setIsOpenUpdateApiKeyModal] = useState<boolean>(false);
    const [isOpenDisableApiKeyModal, setIsOpenDisableApiKeyModal] = useState<boolean>(false);
    const [isOpenEnableApiKeyModal, setIsOpenEnableApiKeyModal] = useState<boolean>(false);
    const [currentApiKey, setCurrentApiKey] = useState<any>({});

    useEffect(() => {
        getApiKeyListData();
    }, [currentPage]);

    const getApiKeyListData = async () => {
        try {
            const res = await getApiKeyList({ project_id: projectId, page_size: 20, page_num: currentPage });
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

    const onPageChange = (page: number) => {
        setCurrentPage(page);
    }

    return (<>
        <CreateApiKeyModal isOpen={isOpenCreateApiKeyModal} setIsOpen={setIsOpenCreateApiKeyModal} projectId={projectId} getApiKeyList={getApiKeyListData} />
        <DeleteApiKeyModal isOpen={isOpenDeleteApiKeyModal} setIsOpen={setIsOpenDeleteApiKeyModal} currentApiKey={currentApiKey} getApiKeyList={getApiKeyListData} />
        <UpdateApiKeyModal isOpen={isOpenUpdateApiKeyModal} setIsOpen={setIsOpenUpdateApiKeyModal} currentApiKey={currentApiKey} getApiKeyList={getApiKeyListData} />
        <DisableApiKeyModal isOpen={isOpenDisableApiKeyModal} setIsOpen={setIsOpenDisableApiKeyModal} currentApiKey={currentApiKey} getApiKeyList={getApiKeyListData} />
        <EnableApiKeyModal isOpen={isOpenEnableApiKeyModal} setIsOpen={setIsOpenEnableApiKeyModal} currentApiKey={currentApiKey} getApiKeyList={getApiKeyListData} />
        <div className="flex flex-col px-4 py-4 gap-4">
            <div className="flex justify-between p-4 border-b border-gray-200 dark:border-gray-700/60 items-center flex-wrap gap-4">
                <div className="flex-1">
                    <span className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Api Keys</span>
                </div>
                <button
                    className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                    onClick={() => {
                        setIsOpenCreateApiKeyModal(true);
                    }}
                >
                    <svg className="fill-current text-gray-400 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M15 7H9V1c0-.6-.4-1-1-1S7 .4 7 1v6H1c-.6 0-1 .4-1 1s.4 1 1 1h6v6c0 .6.4 1 1 1s1-.4 1-1V9h6c.6 0 1-.4 1-1s-.4-1-1-1z" />
                    </svg>
                    <span className="ml-2">Create</span>
                </button>
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
                                    <ApiKeyTableRow
                                        key={index}
                                        apiKey={apiKey}
                                        setIsOpenDeleteApiKeyModal={setIsOpenDeleteApiKeyModal}
                                        setIsOpenUpdateApiKeyModal={setIsOpenUpdateApiKeyModal}
                                        setCurrentApiKey={setCurrentApiKey}
                                        setIsOpenDisableApiKeyModal={setIsOpenDisableApiKeyModal}
                                        setIsOpenEnableApiKeyModal={setIsOpenEnableApiKeyModal}
                                    />
                                ))}
                            </tbody>
                        </table>
                    </div>

                </div>
            </div>
            <div className="sm:flex justify-center items-center mt-4 hidden">
                <PaginationNumeric onChange={onPageChange} current={currentPage} pageSize={20} total={totalApiKey} />
            </div>
            <div className="sm:hidden flex justify-center items-center mt-4">
                <PaginationClassic onChange={onPageChange} current={currentPage} pageSize={20} total={totalApiKey} />
            </div>
        </div>
    </>)
}

function ApiKeyTableRow(
    {
        apiKey, setIsOpenDeleteApiKeyModal, setIsOpenUpdateApiKeyModal, setCurrentApiKey, setIsOpenDisableApiKeyModal, setIsOpenEnableApiKeyModal
    }: {
        apiKey: any,
        setIsOpenDeleteApiKeyModal: (value: boolean) => void,
        setIsOpenUpdateApiKeyModal: (value: boolean) => void,
        setIsOpenDisableApiKeyModal: (value: boolean) => void,
        setIsOpenEnableApiKeyModal: (value: boolean) => void,
        setCurrentApiKey: (value: any) => void
    }) {
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
        <td className={`px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap font-bold ${apiKey.status === "active" ? "text-green-500" : "text-red-500"}`}>{apiKey.status}</td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap flex flex-row gap-2">
            <button
                className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600"
                onClick={() => {
                    setIsOpenUpdateApiKeyModal(true);
                    setCurrentApiKey(apiKey);
                }}
            >
                <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M11.7.3c-.4-.4-1-.4-1.4 0l-10 10c-.2.2-.3.4-.3.7v4c0 .6.4 1 1 1h4c.3 0 .5-.1.7-.3l10-10c.4-.4.4-1 0-1.4l-4-4zM4.6 14H2v-2.6l6-6L10.6 8l-6 6zM12 6.6L9.4 4 11 2.4 13.6 5 12 6.6z" />
                </svg>
            </button>
            <button
                className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600"
                onClick={() => {
                    setIsOpenDeleteApiKeyModal(true);
                    setCurrentApiKey(apiKey);
                }}
            >
                <svg className="fill-current text-red-500 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M5 7h2v6H5V7zm4 0h2v6H9V7zm3-6v2h4v2h-1v10c0 .6-.4 1-1 1H2c-.6 0-1-.4-1-1V5H0V3h4V1c0-.6.4-1 1-1h6c.6 0 1 .4 1 1zM6 2v1h4V2H6zm7 3H3v9h10V5z" />
                </svg>
            </button>
            {apiKey.status === "active" && <button
                className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600"
                onClick={() => {
                    setIsOpenDisableApiKeyModal(true);
                    setCurrentApiKey(apiKey);
                }}
            >
                <svg className="text-red-500 shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" width="16" height="16" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" stroke="currentColor">
                    <path d="M5 13a2 2 0 0 1 2 -2h10a2 2 0 0 1 2 2v6a2 2 0 0 1 -2 2h-10a2 2 0 0 1 -2 -2v-6z"></path>
                    <path d="M11 16a1 1 0 1 0 2 0a1 1 0 0 0 -2 0"></path>
                    <path d="M8 11v-4a4 4 0 1 1 8 0v4"></path>
                </svg>
            </button>}
            {apiKey.status === "disable" && <button
                className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600"
                onClick={() => {
                    setIsOpenEnableApiKeyModal(true);
                    setCurrentApiKey(apiKey);
                }}
            >
                <svg className="text-green-500 shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" width="16" height="16" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" stroke="currentColor">
                    <path d="M3 13a2 2 0 0 1 2 -2h10a2 2 0 0 1 2 2v6a2 2 0 0 1 -2 2h-10a2 2 0 0 1 -2 -2z"></path>
                    <path d="M9 16a1 1 0 1 0 2 0a1 1 0 0 0 -2 0"></path>
                    <path d="M13 11v-4a4 4 0 1 1 8 0v4"></path>
                </svg>
            </button>}
        </td>
    </tr>
}


