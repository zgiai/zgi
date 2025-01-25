"use client"

import { useState, useEffect } from "react"
import { message, Upload, GetProp, UploadFile } from "antd"
import Tooltip from "@/components/tooltip"
import type { UploadProps } from 'antd';
import Link from "next/link"
import { BASE_URL } from "@/config"

const { Dragger } = Upload;
type FileType = Parameters<GetProp<UploadProps, 'beforeUpload'>>[0];

export default function CreateDocumentPage({ kb_id }: { kb_id: string }) {
    const [token, setToken] = useState('')
    const [pageStatus, setPageStatus] = useState<number>(2)

    const uploadDocumentTypeArr: string[] = ['text/plain', 'application/pdf']

    const [fileList, setFileList] = useState<UploadFile[]>([]);
    const [chunkRule, setChunkRule] = useState({
        chunk_size: 1000,
        chunk_overlap: 100,
        separator: '\\n\\n',
    })
    const [uploading, setUploading] = useState(false);

    const handleUpload = async () => {
        const formData = new FormData();
        if (!chunkRule?.chunk_size || !chunkRule?.chunk_overlap || !chunkRule?.separator) {
            message.error("Chunk rule parameter cannot be empty");
            return;
        }

        fileList.forEach((file) => {
            formData.append('files[]', file as FileType);
        });
        setUploading(true);
        try {
            const res = await fetch(`${BASE_URL}/knowledge/${kb_id}/documents`, {
                method: 'POST',
                body: formData,
            }).then(resData => resData.json())
            if (res?.status_code === 200) {
                setUploading(false);
                return
            } else {
                message.error(res?.status_message || "Failed to upload document");
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
                message.success(`${info.file.name} file uploaded successfully.`);
            } else if (status === 'error') {
                message.error(`${info.file.name} file upload failed.`);
            }
        },
        onDrop(e) {
            console.log('Dropped files', e.dataTransfer.files);
        },
        maxCount: 1,
        headers: {
            authorization: 'Bearer ' + token,
        },
        beforeUpload(file) {
            const isTypeAllowed = uploadDocumentTypeArr.includes(file.type);
            // 10MB
            const isSizeAllowed = file.size / 1024 / 1024 < 10;
            if (isTypeAllowed && isSizeAllowed) {
                setFileList([...fileList, file]);
                return false;
            } else {
                message.error(`You can only upload ${uploadDocumentTypeArr.join(', ')} file!`);
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
        <PageStatusProgress />
        <div className="flex flex-col gap-4 p-4 border border-gray-200 dark:border-gray-700/60 rounded-lg bg-white dark:bg-gray-800 shadow-sm">
            {pageStatus === 1 && <h1 className="text-xl md:text-xl text-gray-800 dark:text-gray-100 font-bold" > Upload Document </h1>}
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
                                    <span className="font-semibold text-lg">Click or drag file to this area to upload</span>
                                </span>
                            </div>
                            <p className="text-gray-500 dark:text-gray-400 mt-2">
                                Currently support PDF、TXT files, with a maximum size of 10MB per file.
                            </p>
                        </div>
                    </Dragger>
                </div>
                <div className={`${pageStatus === 2 ? "" : "hidden"}`}>
                    <h1 className="text-xl md:text-xl text-gray-800 dark:text-gray-100 font-bold" > Chunk Rule Settings </h1>
                    <div className="mt-4">
                        <div className="flex items-center gap-1">
                            <label className="block text-lg font-medium mb-1" htmlFor="separators">
                                Separators
                            </label>
                            <Tooltip className="ml-2" bg="dark" size="md">
                                <div className="text-sm text-gray-200">
                                    The separator is used to split the text into vectors. You can use one or more separators. For example, if you want to use both "\n" and "\t" as separators, you can separate them with commas like this: "\n,\t".
                                </div>
                            </Tooltip>
                        </div>
                        <input id="separators" onChange={(e) => setChunkRule({ ...chunkRule, separator: e.target.value })} value={chunkRule?.separator} className="form-input w-full" type="text" />
                    </div>
                    <div className={`flex gap-4`}>
                        <div className="mt-4 flex-1">
                            <div className="flex items-center gap-1">
                                <label className="block text-lg font-medium mb-1" htmlFor="maxChunkSize">
                                    Max Chunk Size
                                </label>
                                <Tooltip className="ml-2" bg="dark" size="md">
                                    <div className="text-sm text-gray-200">
                                        The maximum size of a chunk in tokens.
                                    </div>
                                </Tooltip>
                            </div>
                            <div className="relative">
                                <input id="maxChunkSize" className="form-input w-full" onChange={(e) => setChunkRule({ ...chunkRule, chunk_size: parseInt(e.target.value) })} value={chunkRule?.chunk_size} type="number" />
                                <span className="absolute text-sm right-4 md:right-8 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">Tokens</span>
                            </div>
                        </div>
                        <div className="mt-4 flex-1">
                            <div className="flex items-center gap-1">
                                <label className="block text-lg font-medium mb-1" htmlFor="minChunkSize">
                                    Chunk Overlap
                                </label>
                                <Tooltip className="ml-2" bg="dark" size="md">
                                    <div className="text-sm text-gray-200">
                                        The overlap between chunks in bytes.Suggest to set it to 1/10-1/4 of the max chunk size.
                                    </div>
                                </Tooltip>
                            </div>
                            <div className="relative">
                                <input id="minChunkSize" className="form-input w-full" onChange={(e) => setChunkRule({ ...chunkRule, chunk_overlap: parseInt(e.target.value) })} value={chunkRule?.chunk_overlap} type="number" defaultValue={100} />
                                <span className="absolute right-4 md:right-8 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400 text-sm">Tokens</span>
                            </div>

                        </div>
                    </div>
                </div>
                <div className="flex items-center gap-4 mt-8">
                    {(pageStatus === 1 || pageStatus === 2) && <button
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white disabled:bg-gray-500 disabled:text-gray-100 disabled:hover:bg-gray-500 disabled:hover:text-gray-100 disabled:dark:bg-gray-500 disabled:dark:text-gray-100 disabled:cursor-not-allowed"
                        type="button"
                        onClick={handleNext}
                        disabled={pageStatus === 1 ? fileList.length === 0 : uploading}
                    >
                        <span className="">{pageStatus === 1 ? "Next Step" : uploading ? "Uploading..." : "Upload"}</span>
                    </button>}
                    <Link
                        className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300"
                        href={`/knowledge-base/${kb_id}`}
                    >
                        <span className="">Cancel</span>
                    </Link>
                </div>
            </form>
        </div>
    </div>
}

function PageStatusProgress() {
    return (
        <ul className="inline-flex flex-wrap text-sm font-medium">
            <li className="flex items-center">
                <button className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500">
                    Upload
                </button>
                <svg className="fill-current text-gray-400 dark:text-gray-600 mx-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
                </svg>
            </li>
            <li className="flex items-center">
                <button className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500">
                    Settings
                </button>
                <svg className="fill-current text-gray-400 dark:text-gray-600 mx-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
                </svg>
            </li>
            <li className="flex items-center">
                <span className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500">
                    Complete
                </span>
            </li>
        </ul>
    )
}