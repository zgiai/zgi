"use client"

import { useEffect, useState } from "react"
import { getOrgMembersList, getOrgPermission } from "@/services/organization"
import PaginationNumeric from "@/components/pagination-numeric"
import PaginationClassic from "@/components/pagination-classic"
import { message } from "antd"
import { useParams } from "next/navigation"
import { DeleteMemberModal, UnsetAdminModal, SetAdminModal, AddMemberModal, InviteMemberModal } from "./membersModal"
import { useAppProvider } from "@/app/app-provider"

const roleList = [
    { label: "All", value: -1, color: "" },
    { label: "Super Admin", value: 1, color: "text-red-500" },
    { label: "Admin", value: 2, color: "text-blue-500" },
    { label: "Member", value: 0, color: "" },
]

export default function MembersPage() {
    const { organizationId } = useParams()
    const [memberList, setMemberList] = useState<any[]>([])
    const [totalMember, setTotalMember] = useState(0)

    const [currentPage, setCurrentPage] = useState(1)

    const [isUnsetAdminOpen, setIsUnsetAdminOpen] = useState(false)
    const [isDeleteMemberOpen, setIsDeleteMemberOpen] = useState(false)
    const [isSetAdminOpen, setIsSetAdminOpen] = useState(false)
    const [isAddMemberOpen, setIsAddMemberOpen] = useState(false)
    const [isInviteMemberOpen, setIsInviteMemberOpen] = useState(false)

    const [currentMember, setCurrentMember] = useState<any>(null)
    const [isAdmin, setIsAdmin] = useState(false)
    const [isSuperAdmin, setIsSuperAdmin] = useState(false)

    const { userInfo } = useAppProvider()

    useEffect(() => {
        getMemberList()
    }, [currentPage])

    useEffect(() => {
        init()
    }, [])

    const onPageChange = (page: number) => {
        setCurrentPage(page)
        getMemberList()
    }

    const getMemberList = async () => {
        const res = await getOrgMembersList({
            organization_id: organizationId as string,
            page_size: 20,
            page_num: currentPage,
        })
        if (res?.status_code === 200) {
            setMemberList(res?.data?.members)
            setTotalMember(res?.data?.total)
        } else {
            message.error(res?.status_message || "Get member list failed")
        }
    }

    const init = async () => {
        if (userInfo?.user_type === 1 || userInfo?.user_type === 2) {
            setIsAdmin(true)
            setIsSuperAdmin(true)
            return
        } else {
            getOrgMemberInfo()
        }
    }

    const getOrgMemberInfo = async () => {
        try {
            const res = await getOrgPermission({ organization_id: organizationId as string })
            if (res?.status_code === 200) {
                setIsAdmin(res?.data?.is_admin || false)
            } else {
                message.error(res?.status_message || "Failed to fetch organization member info")
            }
        } catch (err) {
            console.log(err)
        }
    }

    return (
        <>
            {isAdmin && <DeleteMemberModal isOpen={isDeleteMemberOpen} setIsOpen={setIsDeleteMemberOpen} currentMember={currentMember} getMemberList={getMemberList} orgId={organizationId as string} />}
            {isAdmin && <SetAdminModal isOpen={isSetAdminOpen} setIsOpen={setIsSetAdminOpen} currentMember={currentMember} getMemberList={getMemberList} orgId={organizationId as string} />}
            {isAdmin && <UnsetAdminModal isOpen={isUnsetAdminOpen} setIsOpen={setIsUnsetAdminOpen} currentMember={currentMember} getMemberList={getMemberList} orgId={organizationId as string} />}
            {isAdmin && <AddMemberModal isOpen={isAddMemberOpen} setIsOpen={setIsAddMemberOpen} getMemberList={getMemberList} orgId={organizationId as string} />}
            {isAdmin && <InviteMemberModal isOpen={isInviteMemberOpen} setIsOpen={setIsInviteMemberOpen} orgId={organizationId as string} />}
            <div className="flex-1 p-4">
                <div className="bg-white dark:bg-gray-800 shadow-sm rounded-xl relative border border-gray-200 dark:border-gray-700/60">
                    <header className="px-5 py-4 flex flex-row justify-between">
                        <h2 className="font-semibold text-gray-800 dark:text-gray-100 flex-nowrap text-nowrap mr-4">All Members <span className="text-gray-400 dark:text-gray-500 font-medium">{totalMember}</span></h2>
                        <div className="flex flex-col md:flex-row gap-2 md:items-center">
                            <div className="flex flex-row gap-2 items-center flex-wrap">
                                {isAdmin && <button
                                    className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300"
                                    type="button"
                                    onClick={() => {
                                        setIsInviteMemberOpen(true)
                                    }}
                                >
                                    Invite Members
                                </button>}
                                {isAdmin && <button onClick={() => {
                                    setIsAddMemberOpen(true)
                                }} className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">
                                    <svg className="fill-current text-gray-400 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                                        <path d="M15 7H9V1c0-.6-.4-1-1-1S7 .4 7 1v6H1c-.6 0-1 .4-1 1s.4 1 1 1h6v6c0 .6.4 1 1 1s1-.4 1-1V9h6c.6 0 1-.4 1-1s-.4-1-1-1z" />
                                    </svg>
                                    <span className="ml-2">Add Members</span>
                                </button>}
                            </div>
                        </div>
                    </header>
                    <div>
                        {/* Table */}
                        <div className="overflow-x-auto">
                            <table className="table-auto w-full dark:text-gray-300">
                                {/* Table header */}
                                <thead className="text-xs font-semibold uppercase text-gray-500 dark:text-gray-400 bg-gray-50 dark:bg-gray-900/20 border-t border-b border-gray-100 dark:border-gray-700/60">
                                    <tr>
                                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                                            <div className="font-semibold text-left">ID</div>
                                        </th>
                                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                                            <div className="font-semibold text-left">Name</div>
                                        </th>
                                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                                            <div className="font-semibold text-left">Role</div>
                                        </th>
                                        <th className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
                                            <div className="font-semibold text-left">Action</div>
                                        </th>
                                    </tr>
                                </thead>
                                {/* Table body */}
                                <tbody className="text-sm divide-y divide-gray-100 dark:divide-gray-700/60">
                                    {memberList.length === 0 && <tr>
                                        <td colSpan={5} className="text-center py-4">No data</td>
                                    </tr>}
                                    {memberList.map((member: any, index: number) => (
                                        <MemberTableRow key={index} member={member} setCurrentMember={setCurrentMember} setIsSetAdminOpen={setIsSetAdminOpen} setIsUnsetAdminOpen={setIsUnsetAdminOpen} setIsDeleteMemberOpen={setIsDeleteMemberOpen} isAdmin={isAdmin} isSuperAdmin={isSuperAdmin} />
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    </div>
                </div>
                <div className="sm:flex justify-center items-center mt-4 hidden">
                    <PaginationNumeric onChange={onPageChange} current={currentPage} pageSize={20} total={totalMember} />
                </div>
                <div className="sm:hidden flex justify-center items-center mt-4">
                    <PaginationClassic onChange={onPageChange} current={currentPage} pageSize={20} total={totalMember} />
                </div>
            </div>
        </>)
}

function MemberTableRow(props: { member: any, setCurrentMember: (member: any) => void, setIsSetAdminOpen: (value: boolean) => void, setIsUnsetAdminOpen: (value: boolean) => void, setIsDeleteMemberOpen: (value: boolean) => void, isAdmin?: boolean, isSuperAdmin?: boolean }) {
    const { member, setCurrentMember, setIsSetAdminOpen, setIsUnsetAdminOpen, setIsDeleteMemberOpen, isAdmin, isSuperAdmin } = props
    return <tr>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{member.id}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div className="text-left">{member.username}</div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
            <div>
                <span className={`${member?.is_admin ? "text-red-500" : "text-blue-500"} font-medium`}>
                    {member?.is_admin ? "Admin" : "Member"}
                </span>
            </div>
            <div className="text-left">
                <span className={`${roleList.find(role => role.value === member.user_type)?.color} font-medium`}>
                    {roleList.find(role => role.value === member.user_type)?.label}
                </span>
            </div>
        </td>
        <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap flex flex-row gap-2">
            {member.user_type !== 1 && <>
                {isSuperAdmin && <button
                    className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600"
                    onClick={() => {
                        if (member.is_admin) {
                            setCurrentMember(member)
                            setIsUnsetAdminOpen(true)
                        } else {
                            setCurrentMember(member)
                            setIsSetAdminOpen(true)
                        }
                    }}
                >
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M11.7.3c-.4-.4-1-.4-1.4 0l-10 10c-.2.2-.3.4-.3.7v4c0 .6.4 1 1 1h4c.3 0 .5-.1.7-.3l10-10c.4-.4.4-1 0-1.4l-4-4zM4.6 14H2v-2.6l6-6L10.6 8l-6 6zM12 6.6L9.4 4 11 2.4 13.6 5 12 6.6z" />
                    </svg>
                </button>}
                {(isAdmin || isSuperAdmin) && !member.is_admin && <button
                    className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600"
                    onClick={() => {
                        setCurrentMember(member)
                        setIsDeleteMemberOpen(true)
                    }}
                >
                    <svg className="fill-current text-red-500 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M5 7h2v6H5V7zm4 0h2v6H9V7zm3-6v2h4v2h-1v10c0 .6-.4 1-1 1H2c-.6 0-1-.4-1-1V5H0V3h4V1c0-.6.4-1 1-1h6c.6 0 1 .4 1 1zM6 2v1h4V2H6zm7 3H3v9h10V5z" />
                    </svg>
                </button>}
            </>}
        </td>
    </tr>
}