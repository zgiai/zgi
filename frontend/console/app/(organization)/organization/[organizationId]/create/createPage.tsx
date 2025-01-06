"use client"

import { useState } from "react"
import { CreateProjectParams } from "@/interfaces/request"
import { message } from "antd"
import Link from "next/link"
import { createProject } from "@/services/project"
import { useParams } from "next/navigation"

export default function CreateProjectPage() {

    const { organizationId } = useParams()
    const [pageStatus, setPageStatus] = useState<number>(1)
    const [formData, setFormData] = useState<CreateProjectParams>({
        name: "",
        description: "",
        organization_id: organizationId as string
    })

    const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault()
        if (formData.name.length < 3) {
            message.error("Project name must be at least 3 characters")
            return
        } else {
            try {
                const response = await createProject(formData)
                if (response?.status_code === 200) {
                    message.success("Project created successfully")
                    window.location.href = '/organizations'
                } else {
                    message.error(response?.status_message || "Failed to create project")
                }
            } catch (error) {
                message.error("Failed to create project")
            }
        }
    }

    // const handleNext = () => {
    //     if (formData.name.length < 3) {
    //         message.error("Name must be at least 3 characters")
    //         return
    //     } else {
    //         setPageStatus(2)
    //     }
    // }

    return <div className="flex flex-col px-4 py-4">
        <div className="flex justify-between p-4 border-b border-gray-200 dark:border-gray-700/60 items-center flex-wrap gap-4">
            <div className="flex-1">
                <span className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Create Project</span>
            </div>
        </div>
        <CreateProgress step={pageStatus} />
        <div className="flex flex-col gap-4 p-4 border border-gray-200 dark:border-gray-700/60 rounded-lg bg-white dark:bg-gray-800 shadow-sm">
            <form className="flex flex-col gap-4" onSubmit={handleSubmit}>
                <div className={`flex-col gap-4 ${pageStatus === 1 ? "flex" : "hidden"}`}>
                    <div>
                        <label htmlFor="name" className="text-gray-800 dark:text-gray-100 font-bold">Name</label>
                        <input id="name" className="form-input w-full" placeholder="my-organization" type="text" value={formData.name} onChange={(e) => setFormData({ ...formData, name: e.target.value.trim() })} />
                    </div>
                    <div>
                        <label htmlFor="description" className="text-gray-800 dark:text-gray-100 font-bold">Description</label>
                        <textarea id="description" className="form-textarea w-full min-h-[100px]" placeholder="my-organization-description" value={formData.description} onChange={(e) => setFormData({ ...formData, description: e.target.value.trim() })} />
                    </div>
                </div>
                <div className="flex items-center gap-4">
                    <button
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                        type="submit"
                    >
                        <span className="">Create</span>
                    </button>
                    <Link
                        className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300"
                        href={`/organization/${organizationId}/projects`}
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
                <span className={`${step === 2 ? "text-gray-800 dark:text-gray-100 font-bold" : "text-gray-500 dark:text-gray-400"}`}>Create Project</span>
            </div>
        </div>
    </div>
}
