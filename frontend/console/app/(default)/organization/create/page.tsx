"use client"

import { useState } from "react"
import { CreateOrganizationParams } from "@/interfaces/request"
import { createOrganization } from "@/services/organization"
import { message } from "antd"

export default function CreateOrganizationPage() {
    const [pageStatus, setPageStatus] = useState<number>(1)
    const [formData, setFormData] = useState<CreateOrganizationParams>({
        name: "",
        description: "",
        project: {
            name: "",
            description: ""
        }
    })


    const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault()
        const response = await createOrganization(formData)
        if (response?.status_code === 200) {
            message.success(response?.status_message || "Organization created successfully")
            window.location.href = '/organization'
        } else {
            message.error(response?.status_message || "Failed to create organization")
        }
    }

    const handleNext = () => {
        if (formData.name.length < 3) {
            message.error("Name must be at least 3 characters")
            return
        } else {
            setPageStatus(2)
        }
    }

    return <div className="flex flex-col px-4 py-4">
        <div className="flex justify-between p-4 border-b border-gray-200 dark:border-gray-700/60 items-center flex-wrap gap-4">
            <div className="flex-1">
                <span className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Create Organization</span>
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
                <div className={`flex-col gap-4 ${pageStatus === 2 ? "flex" : "hidden"}`}>
                    <div>
                        <label htmlFor="name" className="text-gray-800 dark:text-gray-100 font-bold">Name</label>
                        <input
                            id="name"
                            className="form-input w-full"
                            placeholder="my-project"
                            type="text"
                            value={formData.project?.name}
                            onChange={(e) => setFormData({ ...formData, project: { description: formData.project?.description || "", name: e.target.value.trim() } })}
                        />
                    </div>
                    <div>
                        <label htmlFor="description" className="text-gray-800 dark:text-gray-100 font-bold">Description</label>
                        <textarea
                            id="description"
                            className="form-textarea w-full min-h-[100px]"
                            placeholder="my-project-description"
                            value={formData.project?.description}
                            onChange={(e) => setFormData({ ...formData, project: { description: e.target.value.trim(), name: formData.project?.name || "" } })}
                        />
                    </div>
                </div>
                <div className="flex items-center">
                    {pageStatus === 1 && <button
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                        onClick={handleNext}
                        type="button"
                    >
                        <span className="">Next</span>
                    </button>}
                    {pageStatus === 2 && <button
                        className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                        type="submit"
                    >
                        <span className="">Create</span>
                    </button>}
                </div>
            </form>
        </div>
    </div>
}

function CreateProgress({ step = 1, setStep = () => { } }: { step?: number, setStep?: (step: number) => void }) {
    return <div className="py-4">
        <div className="flex flex-row gap-2 items-center flex-wrap">
            <div className="flex flex-row gap-2 items-center">
                <button
                    className={`${step === 1 ? "text-gray-800 dark:text-gray-100 font-bold" : "text-gray-500 dark:text-gray-400"}`}
                    type="button"
                    onClick={() => setStep(1)}
                >
                    Create Organization
                </button>
            </div>
            <div className="text-gray-800 dark:text-gray-100">
                <svg viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" width="16" height="16"><path d="M320 828.8L636.16 512 320 195.2a32 32 0 1 1 45.44-45.44L704 489.6a32 32 0 0 1 0 45.44l-339.2 339.2a32 32 0 0 1-44.8-45.44z" fill="currentColor" ></path></svg>
            </div>
            <div className="flex flex-row gap-2 items-center">
                <span className={`${step === 2 ? "text-gray-800 dark:text-gray-100 font-bold" : "text-gray-500 dark:text-gray-400"}`}>Create Project</span>
            </div>
        </div>
    </div>
}
