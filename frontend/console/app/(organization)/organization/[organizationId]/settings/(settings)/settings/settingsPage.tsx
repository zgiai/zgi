"use client"

import { getUserInfo } from "@/services/auth";
import { useState, useEffect } from "react"
import { getOrganizationDetail } from "@/services/organization"
import { message } from "antd"
import { useParams } from "next/navigation"
import { DeleteOrganizationModal, EditOrganizationModal, QuitOrganizationModal } from "./settingsModal"

export default function SettingsPage() {
    const params = useParams()
    const organizationId = params.organizationId as string || ""

    const [isAdmin, setIsAdmin] = useState(false);
    const [organizationInfo, setOrganizationInfo] = useState<any>({});

    const [isDeleteOrgModalOpen, setIsDeleteOrgModalOpen] = useState(false);
    const [isQuitOrgModalOpen, setIsQuitOrgModalOpen] = useState(false);
    const [editOrgModalOpen, setEditOrgModalOpen] = useState(false);

    const getOrganizationInfo = async () => {
        try {
            const res = await getOrganizationDetail({ organization_id: organizationId })
            if (res.status_code === 200) {
                setOrganizationInfo(res?.data);
            } else {
                message.error(res?.status_message || 'Failed to fetch organization info');
            }
        } catch (error) {
            console.error('Error fetching organization info:', error);
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
        <DeleteOrganizationModal isOpen={isDeleteOrgModalOpen} setIsOpen={setIsDeleteOrgModalOpen} orgId={organizationId} />
        <EditOrganizationModal isOpen={editOrgModalOpen} setIsOpen={setEditOrgModalOpen} orgId={organizationId} organizationInfo={organizationInfo} isAdmin={isAdmin} />
        <QuitOrganizationModal isOpen={isQuitOrgModalOpen} setIsOpen={setIsQuitOrgModalOpen} orgId={organizationId} />
        <div className="flex flex-col px-4 py-4 w-full max-w-[96rem] mx-auto gap-4">
            <div className="flex justify-between py-4 border-b border-gray-200 dark:border-gray-700/60 items-center flex-wrap gap-4">
                <div className="flex-1">
                    <span className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Organization Settings</span>
                </div>
            </div>
            <div className="flex flex-col gap-4 p-4 border border-gray-200 dark:border-gray-700/60 rounded-lg bg-white dark:bg-gray-800 shadow-sm">
                <div className="flex flex-row gap-2 border-b border-gray-200 dark:border-gray-700/60">
                    <div className="flex flex-col gap-2 flex-1 pb-4">
                        <div>
                            <span className="text-lg text-gray-800 dark:text-gray-100 font-bold">{organizationInfo?.name || 'Organization Name'}</span>
                        </div>
                        <div>
                            <span className="text-lg text-gray-800 dark:text-gray-100">{organizationInfo?.description || 'no description'}</span>
                        </div>
                    </div>
                    <div>
                        {isAdmin && <button
                            className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                            onClick={() => {
                                setEditOrgModalOpen(true);
                            }}
                        >
                            Edit
                        </button>}
                    </div>
                </div>
                <div className="flex flex-row gap-2">
                    <button
                        className={`btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white`}
                        onClick={() => {
                            setIsQuitOrgModalOpen(true);
                        }}
                    >
                        Quit Organization
                    </button>
                    {isAdmin && <button
                        className={`btn text-white bg-red-500 hover:bg-red-600`}
                        onClick={() => {
                            setIsDeleteOrgModalOpen(true);
                        }}
                    >
                        Delete Organization
                    </button>}
                </div>

            </div>
        </div>
    </>
}