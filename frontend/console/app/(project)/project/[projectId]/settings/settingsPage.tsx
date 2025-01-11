"use client"


import { getUserInfo } from "@/services/auth";
import { useState, useEffect } from "react"
import { getProjectDetail } from "@/services/project"
import { message } from "antd"
import { useParams } from "next/navigation"
import { DeleteProjectModal, EditProjectModal } from "./settingsModal"

export default function SettingsPage() {
    const params = useParams()
    const projectId = params.projectId as string || ""
    const [isAdmin, setIsAdmin] = useState(false);
    const [projectInfo, setProjectInfo] = useState<any>({});
    const [isDeleteProjectModalOpen, setIsDeleteProjectModalOpen] = useState(false);
    const [editProjectModalOpen, setEditProjectModalOpen] = useState(false);

    const getOrganizationInfo = async () => {
        try {
            const res = await getProjectDetail({ project_id: projectId })
            if (res.status_code === 200) {
                setProjectInfo(res?.data);
            } else {
                message.error(res?.status_message || 'Failed to fetch project info');
            }
        } catch (error) {
            console.error('Error fetching project info:', error);
        };
    }

    const getUserRole = async () => {
        try {
            const res = await getUserInfo()
            if (res.status_code === 200) {
                const data = res.data;
                if (data?.user_type === 1 || data?.user_type === 2) {
                    setIsAdmin(true);
                } else {
                    setIsAdmin(false);
                }
            } else {
                console.error('Failed to fetch user role');
            }
        } catch (error) {
            console.error('Error fetching user role:', error);
        };
    }

    useEffect(() => {
        getOrganizationInfo();
        getUserRole();
    }, [])

    return <>
        <DeleteProjectModal isOpen={isDeleteProjectModalOpen} setIsOpen={setIsDeleteProjectModalOpen} projectId={projectId} />
        <EditProjectModal isOpen={editProjectModalOpen} setIsOpen={setEditProjectModalOpen} projuctId={projectId} projectInfo={projectInfo} isAdmin={isAdmin} />
        <div className="flex flex-col px-4 py-4 w-full max-w-[96rem] mx-auto gap-4">
            <div className="flex justify-between py-4 border-b border-gray-200 dark:border-gray-700/60 items-center flex-wrap gap-4">
                <div className="flex-1">
                    <span className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Project Settings</span>
                </div>
            </div>
            <div className="flex flex-col gap-4 p-4 border border-gray-200 dark:border-gray-700/60 rounded-lg bg-white dark:bg-gray-800 shadow-sm">
                <div className="flex flex-row gap-2 border-b border-gray-200 dark:border-gray-700/60">
                    <div className="flex flex-col gap-2 flex-1 pb-4">
                        <div>
                            <span className="text-lg text-gray-800 dark:text-gray-100 font-bold">{projectInfo?.name || 'Project Name'}</span>
                        </div>
                        <div>
                            <span className="text-lg text-gray-800 dark:text-gray-100">{projectInfo?.description || 'no description'}</span>
                        </div>
                    </div>
                    <div>
                        {isAdmin && <button
                            className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                            onClick={() => {
                                setEditProjectModalOpen(true);
                            }}
                        >
                            Edit
                        </button>}
                    </div>
                </div>
                <div className="flex flex-row gap-2">
                    {isAdmin && <button
                        className={`btn text-white bg-red-500 hover:bg-red-600`}
                        onClick={() => {
                            setIsDeleteProjectModalOpen(true);
                        }}
                    >
                        Delete Project
                    </button>}
                </div>
            </div>
        </div>
    </>
}