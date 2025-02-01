"use client"

import { useState, useEffect } from "react"
import { message, Upload, GetProp, UploadFile } from "antd"
import Tooltip from "@/components/tooltip"
import type { UploadProps } from 'antd';
import Link from "next/link"
import { BASE_URL } from "@/config"
import { useAppProvider } from "@/app/app-provider"
import { knowledgeBaseLang } from "@/app/(default)/knowledge-base/lang"

const { Dragger } = Upload;
type FileType = Parameters<GetProp<UploadProps, 'beforeUpload'>>[0];

export default function CreateDocumentPage({ kb_id }: { kb_id: string }) {
    const [token, setToken] = useState('')
    const [pageStatus, setPageStatus] = useState<number>(1)
    const { language } = useAppProvider();

    const uploadDocumentTypeArr: string[] = ['text/plain', 'application/pdf']

    const [fileList, setFileList] = useState<UploadFile[]>([]);
    const [chunkRule, setChunkRule] = useState({
        chunk_size: 1000,
        chunk_overlap: 100,
        separator: '\\n\\n',
    })
    const [chunkRuleErrors, setChunkRuleErrors] = useState({
        chunk_size: false,
        chunk_overlap: false,
        separator: false,
    })
    const [uploading, setUploading] = useState(false);

    const handleUpload = async () => {
        const formData = new FormData();
        const errors = {
            chunk_size: !chunkRule.chunk_size,
            chunk_overlap: !chunkRule.chunk_overlap,
            separator: !chunkRule.separator,
        };
        setChunkRuleErrors(errors);

        if (errors.chunk_size || errors.chunk_overlap || errors.separator) {
            message.error(knowledgeBaseLang[language].pleaseFillInAllChunkRuleParameters);
            return;
        }
        if (!chunkRule?.chunk_size || !chunkRule?.chunk_overlap || !chunkRule?.separator) {
            message.error(knowledgeBaseLang[language].chunkRuleParameterCannotBeEmpty);
            return;
        }
        const chunk_rule = {
            chunk_size: chunkRule?.chunk_size,
            chunk_overlap: chunkRule?.chunk_overlap,
            separator: chunkRule?.separator?.split(','),
        }
        formData.append('chunk_rule', JSON.stringify(chunk_rule));
        formData.append('file', fileList[0] as FileType);
        setUploading(true);
        try {
            const res = await fetch(`${BASE_URL}/knowledge/${kb_id}/documents`, {
                headers: {
                    authorization: 'Bearer ' + token,
                },
                method: 'POST',
                body: formData,
            }).then(resData => resData.json())
            if (res?.code === 200) {
                setUploading(false);
                setPageStatus(3)
                return
            } else {
                message.error(res?.status_message || knowledgeBaseLang[language].failedToUploadDocument);
                return
            }
        } catch (error) {
            console.error(error)
            setUploading(false);
        } finally {
            setUploading(false);
        }

    };

    const props: UploadProps = {
        name: 'file',
        multiple: true,
        action: `${BASE_URL}/knowledge/${kb_id}/documents`,
        onChange(info) {
            const { status } = info.file;
            if (status !== 'uploading') {
                console.log(info.file, info.fileList);
            }
            if (status === 'done') {
                message.success(`${info.file.name} ${knowledgeBaseLang[language].fileUploadedSuccessfully}.`);
            } else if (status === 'error') {
                message.error(`${info.file.name} ${knowledgeBaseLang[language].fileUploadFailed}.`);
            }
        },
        onDrop(e) {
            console.log('Dropped files', e.dataTransfer.files);
        },
        maxCount: 1,
        beforeUpload(file) {
            const isTypeAllowed = uploadDocumentTypeArr.includes(file.type);
            // 10MB
            const isSizeAllowed = file.size / 1024 / 1024 < 10;
            if (isTypeAllowed && isSizeAllowed) {
                setFileList([...fileList, file]);
                return false;
            } else {
                message.error(`${knowledgeBaseLang[language].youCanOnlyUpload}`);
                return false;
            }
        },
        onRemove: (file) => {
            const index = fileList.indexOf(file);
            const newFileList = fileList.slice();
            newFileList.splice(index, 1);
            setFileList(newFileList);
        },
        fileList,
    };

    useEffect(() => {
        const token = localStorage.getItem('token')
        if (token) {
            setToken(token)
        }
    }, [])

    const handleNext = () => {
        if (pageStatus === 1) {
            setPageStatus(2)
        } else {
            handleUpload()
        }
    }

    return <div className="flex flex-col px-4 py-4 w-full max-w-[96rem] mx-auto gap-4">
        <PageStatusProgress pageStatus={pageStatus} setStatus={setPageStatus} language={language} />
        <div className="flex flex-col gap-4 p-4 border border-gray-200 dark:border-gray-700/60 rounded-lg bg-white dark:bg-gray-800 shadow-sm">
            {pageStatus === 1 && <h1 className="text-xl md:text-xl text-gray-800 dark:text-gray-100 font-bold" > {knowledgeBaseLang[language].uploadDocument} </h1>}
            <form className="flex flex-col gap-4">
                <div className={`${pageStatus === 1 ? "" : "hidden"}`}>
                    <Dragger {...props}>
                        <div
                            className="py-4"
                        >
                            <div className="flex justify-center text-gray-800 dark:text-gray-100 items-center">
                                <span>
                                    <svg viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" width="40" height="40">
                                        <path d="M770.1 880H253.9C173 880 112 835.7 112 776.9V439.1c0-58.8 61-103.1 141.8-103.1h81.9c26.5 0 48 21.5 48 48s-21.5 48-48 48h-81.9c-24.9 0-40.5 7.7-45.8 12.1V772c5.4 4.4 21 12.1 45.8 12.1h516.3c24.9 0 40.5-7.7 45.9-12.1V444.1c-5.4-4.4-21-12.1-45.9-12.1h-81.9c-26.5 0-48-21.5-48-48s21.5-48 48-48h81.9C851 336 912 380.3 912 439.1v337.8c0 58.8-61 103.1-141.9 103.1z m47.6-434.3h0.6-0.6z" fill="currentColor"></path>
                                        <path d="M512 687.2c-26.5 0-48-21.5-48-48V130.4c0-26.5 21.5-48 48-48s48 21.5 48 48v508.8c0 26.5-21.5 48-48 48z" fill="currentColor"></path>
                                        <path d="M691.9 294.3c-8.9 0-17.9-2.4-26-7.6L486 170.8c-22.3-14.4-28.7-44.1-14.3-66.4C486 82.1 515.7 75.6 538 90l179.9 115.9c22.3 14.4 28.7 44.1 14.3 66.4-9.1 14.3-24.6 22-40.3 22z" fill="currentColor"></path>
                                        <path d="M332.1 294.3c-15.8 0-31.2-7.8-40.4-22-14.4-22.3-7.9-52 14.3-66.4L486 90.1c22.2-14.4 52-8 66.3 14.4 14.4 22.3 7.9 52-14.3 66.4L358.1 286.7c-8.1 5.2-17.1 7.6-26 7.6z" fill="currentColor"></path>
                                    </svg>
                                </span>
                                <span className="flex justify-center items-center ml-2">
                                    <span className="font-semibold text-lg">{knowledgeBaseLang[language].clickOrDragFileToThisAreaToUpload}</span>
                                </span>
                            </div>
                            <p className="text-gray-500 dark:text-gray-400 mt-2">
                                {knowledgeBaseLang[language].currentlySupport} 
                            </p>
                        </div>
                    </Dragger>
                </div>
                <div className={`${pageStatus === 2 ? "" : "hidden"}`}>
                    <h1 className="text-xl md:text-xl text-gray-800 dark:text-gray-100 font-bold" > {knowledgeBaseLang[language].chunkRuleSettings} </h1>
                    <div className="mt-4">
                        <div className="flex items-center gap-1">
                            <label className="block text-lg font-medium mb-1" htmlFor="separators">
                                {knowledgeBaseLang[language].separators}
                            </label>
                            <Tooltip className="ml-2" bg="dark" size="md">
                                <div className="text-sm text-gray-200">
                                    {knowledgeBaseLang[language].separatorDescription}
                                </div>
                            </Tooltip>
                        </div>
                        <input id="separators" onChange={(e) => setChunkRule({ ...chunkRule, separator: e.target.value })} value={chunkRule?.separator} className={`form-input w-full ${chunkRuleErrors.separator ? 'border-red-500' : ''}`} type="text" />
                        {chunkRuleErrors.separator && <p className="text-red-500 text-sm mt-1">{knowledgeBaseLang[language].pleaseFillInTheSeparator}</p>}
                    </div>
                    <div className={`flex gap-4`}>
                        <div className="mt-4 flex-1">
                            <div className="flex items-center gap-1">
                                <label className="block text-lg font-medium mb-1" htmlFor="maxChunkSize">
                                    {knowledgeBaseLang[language].maxChunkSize}
                                </label>
                                <Tooltip className="ml-2" bg="dark" size="md">
                                    <div className="text-sm text-gray-200">
                                        {knowledgeBaseLang[language].maxChunkSizeDescription}
                                    </div>
                                </Tooltip>
                            </div>
                            <div className="relative">
                                <div className="relative">
                                    <input id="maxChunkSize" className={`form-input w-full ${chunkRuleErrors.chunk_size ? 'border-red-500' : ''}`} onChange={(e) => setChunkRule({ ...chunkRule, chunk_size: parseInt(e.target.value) })} value={chunkRule?.chunk_size} type="number" />
                                    <span className="absolute text-sm right-4 md:right-8 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">Tokens</span>
                                </div>
                                {chunkRuleErrors.chunk_size && <p className="text-red-500 text-sm mt-1">{knowledgeBaseLang[language].pleaseFillInTheMaxChunkSize}</p>}
                            </div>
                        </div>
                        <div className="mt-4 flex-1">
                            <div className="flex items-center gap-1">
                                <label className="block text-lg font-medium mb-1" htmlFor="minChunkSize">
                                    {knowledgeBaseLang[language].chunkOverlap}
                                </label>
                                <Tooltip className="ml-2" bg="dark" size="md">
                                    <div className="text-sm text-gray-200">
                                        {knowledgeBaseLang[language].chunkOverlapDescription}
                                    </div>
                                </Tooltip>
                            </div>
                            <div className="relative">
                                <div className="relative">
                                    <input id="minChunkSize" className={`form-input w-full ${chunkRuleErrors.chunk_overlap ? 'border-red-500' : ''}`} onChange={(e) => setChunkRule({ ...chunkRule, chunk_overlap: parseInt(e.target.value) })} value={chunkRule?.chunk_overlap} type="number" defaultValue={100} />
                                    <span className="absolute right-4 md:right-8 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400 text-sm">Tokens</span>
                                </div>
                                {chunkRuleErrors.chunk_overlap && <p className="text-red-500 text-sm mt-1">{knowledgeBaseLang[language].pleaseFillInTheChunkOverlap}</p>}
                            </div>

                        </div>
                    </div>
                </div>
                <div className={`${pageStatus === 3 ? "" : "hidden"} flex flex-col items-center`}>
                    <h1 className="text-4xl font-bold text-gray-800 dark:text-gray-100 ">{knowledgeBaseLang[language].documentUploadedSuccessfully}</h1>
                </div>
                <div className={`flex items-center gap-4 mt-8 ${pageStatus === 3 ? "justify-center" : "justify-start"}`}>
                    {(pageStatus === 1 || pageStatus === 2) && <button
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white disabled:bg-gray-500 disabled:text-gray-100 disabled:hover:bg-gray-500 disabled:hover:text-gray-100 disabled:cursor-not-allowed"
                        type="button"
                        onClick={handleNext}
                        disabled={pageStatus === 1 ? fileList.length === 0 : uploading}
                    >
                        <span className="">{pageStatus === 1 ? knowledgeBaseLang[language].nextStep : uploading ? "Uploading..." : knowledgeBaseLang[language].upload}</span>
                    </button>}
                    {pageStatus !== 3 && <Link
                        className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300"
                        href={`/knowledge-base/${kb_id}`}
                    >
                        <span className="">{knowledgeBaseLang[language].cancel}</span>
                    </Link>}
                    {pageStatus === 3 && <Link
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white px-10"
                        href={`/knowledge-base/${kb_id}`}
                    >
                        <span className="">{knowledgeBaseLang[language].back}</span>
                    </Link>}
                </div>
            </form>
        </div>
    </div>
}

function PageStatusProgress({ pageStatus, setStatus, language }: { pageStatus: number, setStatus: (status: number) => void, language: 'en' | 'zh' }) {

    return (
        <ul className="inline-flex flex-wrap text-sm font-medium">
            <li className="flex items-center">
                <button
                    className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500"
                    onClick={() => {
                        if (pageStatus == 2) {
                            setStatus(1)
                        }
                    }}
                >
                    {knowledgeBaseLang[language].upload}
                </button>
                <svg className="fill-current text-gray-400 dark:text-gray-600 mx-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
                </svg>
            </li>
            <li className="flex items-center">
                <span
                    className="text-gray-500 dark:text-gray-400 cursor-default"
                >
                    {knowledgeBaseLang[language].chunkRuleSettings}
                </span>
                <svg className="fill-current text-gray-400 dark:text-gray-600 mx-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
                </svg>
            </li>
            <li className="flex items-center">
                <span className="text-gray-500 dark:text-gray-400 cursor-default">
                    {knowledgeBaseLang[language].complete}
                </span>
            </li>
        </ul>
    )
}