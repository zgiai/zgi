"use client"

import { useState,useEffect } from "react"
import { CreateProjectParams } from "@/interfaces/request"
import { message, Upload, } from "antd"
import type { UploadProps } from 'antd';
import Link from "next/link"
import { useParams } from "next/navigation"
import { BASE_URL } from "@/config"

const { Dragger } = Upload;

export default function CreateDocumentPage({ kb_id }: { kb_id: string }) {
    const [token,setToken] = useState('')
    const { organizationId } = useParams()
    const [pageStatus, setPageStatus] = useState<number>(1)
    const [formData, setFormData] = useState<CreateProjectParams>({
        name: "",
        description: "",
        organization_id: organizationId as string
    })

    const uploadDocumentTypeArr: string[] = ['text/plain', 'application/pdf']

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
            const isSizeAllowed = file.size / 1024 / 1024 < 15;
            if (isTypeAllowed && isSizeAllowed) {
                return true;
            } else {
                message.error(`You can only upload ${uploadDocumentTypeArr.join(', ')} file!`);
                return Upload.LIST_IGNORE;
            }
        },
    };

    useEffect(() => {
        const token = localStorage.getItem('token')
        if (token) {
            setToken(token)
        }
    }, [])

    // const handleNext = () => {
    //     if (formData.name.length < 3) {
    //         message.error("Name must be at least 3 characters")
    //         return
    //     } else {
    //         setPageStatus(2)
    //     }
    // }

    return <div className="flex flex-col px-4 py-4 w-full max-w-[96rem] mx-auto">
        <div className="flex justify-between pb-4 border-b border-gray-200 dark:border-gray-700/60 items-center flex-wrap gap-4">
            <div className="flex-1">
                <span className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Add Document</span>
            </div>
        </div>
        <CreateProgress step={pageStatus} />
        <div className="flex flex-col gap-4 p-4 border border-gray-200 dark:border-gray-700/60 rounded-lg bg-white dark:bg-gray-800 shadow-sm">
            <form className="flex flex-col gap-4">
                <div>
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
                                Currently support PDF、TXT files, with a maximum size of 15MB per file.
                            </p>
                        </div>

                    </Dragger>
                </div>
                <div className="flex items-center gap-4 mt-8">
                    {/* <button
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                        type="submit"
                    >
                        <span className="">Create</span>
                    </button> */}
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

function CreateProgress({ step = 1, setStep = () => { } }: { step?: number, setStep?: (step: number) => void }) {
    return <div className="py-4">
        <div className="flex flex-row gap-2 items-center flex-wrap">
            {/* <div className="text-gray-800 dark:text-gray-100">
                <svg viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" width="16" height="16"><path d="M320 828.8L636.16 512 320 195.2a32 32 0 1 1 45.44-45.44L704 489.6a32 32 0 0 1 0 45.44l-339.2 339.2a32 32 0 0 1-44.8-45.44z" fill="currentColor" ></path></svg>
            </div> */}
            <div className="flex flex-row gap-2 items-center">
                <span className={`${step === 2 ? "text-gray-800 dark:text-gray-100 font-bold" : "text-gray-500 dark:text-gray-400"}`}>Upload Document</span>
            </div>
        </div>
    </div>
}