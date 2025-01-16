"use client"

import { useState } from "react"
import { CreateOrganizationParams } from "@/interfaces/request"
import { createOrganization } from "@/services/organization"
import { message } from "antd"
import Link from "next/link"

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
        if ((formData?.project?.name || "").length < 3) {
            message.error("Project name must be at least 3 characters")
            return
        } else {
            const response = await createOrganization(formData)
            if (response?.status_code === 200) {
                message.success("Organization created successfully")
                window.location.href = '/organizations'
            } else {
                message.error(response?.status_message || "Failed to create organization")
            }
        }
    }

    const handleNext = () => {
        if (formData.name.length < 3) {
            message.error("Organization name must be at least 3 characters")
            return
        } else {
            setPageStatus(2)
        }
    }

    return <div className="flex flex-col px-4 py-4 w-full mx-auto">
        <div className="flex justify-between border-b py-4 border-gray-200 dark:border-gray-700/60 items-center flex-wrap gap-4">
            <div className="flex-1">
                <span className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Create Organization</span>
            </div>
        </div>
        <CreateProgress step={pageStatus} setStep={setPageStatus} />
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
                <div className="flex items-center gap-4">
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
                    <Link
                        className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300"
                        href="/organizations"
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
        <div className="flex flex-row items-center flex-wrap">
            <div className="flex flex-row items-center">
                <button
                    className={`${step === 1 ? "text-gray-800 dark:text-gray-100 font-bold" : "text-gray-500 dark:text-gray-400"}`}
                    type="button"
                    onClick={() => {setStep(1)}}
                >
                    Create Organization
                </button>
            </div>
            <div className="text-gray-800 dark:text-gray-100">
                <svg className="fill-current text-gray-400 dark:text-gray-600 mx-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
                </svg>
            </div>
            <div className="flex flex-row gap-2 items-center">
                <span className={`${step === 2 ? "text-gray-800 dark:text-gray-100 font-bold" : "text-gray-500 dark:text-gray-400"}`}>Create Project</span>
            </div>
        </div>
    </div>
}
