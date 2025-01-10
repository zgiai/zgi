import ModalAction from "@/components/modal-action";
import { removeOrgMember, unsetOrgAdmin, setOrgAdmin, searchUserByEmail, addOrgMember } from "@/services/organization";
import { message } from "antd";
import { useState } from "react";

export function DeleteMemberModal({ isOpen, setIsOpen, currentMember, getMemberList, orgId }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentMember: any, getMemberList: () => void, orgId: string }) {
    const [loading, setLoading] = useState(false);
    const handleDeleteMember = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await removeOrgMember({ user_ids: [currentMember?.user_id], organization_id: orgId });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Delete member success");
                getMemberList();
            } else {
                message.error(res.status_message || "Delete member failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }
    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Delete Member</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to delete this member?</div>
        <form onSubmit={handleDeleteMember} className="flex justify-end gap-4">
            <button className="btn bg-red-500 text-white" type="submit" disabled={loading}>{loading ? "Deleting..." : "Delete"} </button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>
}

export function SetAdminModal({ isOpen, setIsOpen, currentMember, getMemberList, orgId }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentMember: any, getMemberList: () => void, orgId: string }) {
    const [loading, setLoading] = useState(false);

    const handleSetAdmin = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await setOrgAdmin({ organization_id: orgId, user_ids: [currentMember?.user_id] });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Set organization admin success");
                getMemberList();
            } else {
                message.error(res.status_message || "Set organization admin failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Set Organization Admin</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to set this member as organization admin?</div>
        <form onSubmit={handleSetAdmin} className="flex justify-end gap-4">
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? "Setting..." : "Set Admin"}</button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>
}

export function UnsetAdminModal({
    isOpen, setIsOpen, currentMember, getMemberList, orgId
}: {
    isOpen: boolean, setIsOpen: (value: boolean) => void, currentMember: any, getMemberList: () => void, orgId: string
}) {
    const [loading, setLoading] = useState(false);

    const handleUnsetAdmin = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await unsetOrgAdmin({ user_ids: [currentMember?.id], organization_id: orgId });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Unset admin success");
                getMemberList();
            } else {
                message.error(res.status_message || "Unset admin failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Unset Admin</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to unset this member as admin?</div>
        <form onSubmit={handleUnsetAdmin} className="flex justify-end gap-4">
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? "Unsetting..." : "Unset Admin"}</button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>
}

export const AddMemberModal = ({ isOpen, setIsOpen, getMemberList, orgId }: { isOpen: boolean, setIsOpen: (value: boolean) => void, getMemberList: () => void, orgId: string }) => {
    const [email, setEmail] = useState('')
    const [loading, setLoading] = useState(false)
    const [searchLoading, setSearchLoading] = useState(false)
    const [searchMember, setSearchMember] = useState<any>(null)

    const handleSearchUserByEmail = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setSearchLoading(true);
        try {
            const res = await searchUserByEmail({ email: email, organization_id: orgId });
            if (res.status_code === 200) {
                console.log(res.data);
                setSearchMember(res.data);
            } else {
                message.error(res.status_message || "Search user failed");
                setSearchMember(null);
            }
        } catch (error) {
            console.error(error);
            setSearchMember(null);
            setEmail('');
            setSearchLoading(false);
        } finally {
            setSearchLoading(false);
        }
    }

    const handleAddMember = async () => {
        setLoading(true);
        try {
            const res = await addOrgMember({ user_ids: [searchMember?.user_id], organization_id: orgId });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Add member success");
                getMemberList();
            } else {
                message.error(res.status_message || "Add member failed");
            }
        } catch (error) {
            console.error(error);
            setIsOpen(false);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Add Member</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Enter email to add member</div>
        <form onSubmit={handleSearchUserByEmail} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
                <div className="flex flex-col md:flex-row gap-1 md:items-center">
                    <label htmlFor="email" className="text-gray-800 dark:text-gray-100">Email</label>
                    <input type="email" id="email" value={email} onChange={(e) => setEmail(e.target.value)} className="form-input" placeholder="Enter email" />
                    <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={searchLoading}>{searchLoading ? "Searching..." : "Search"}</button>
                </div>
            </div>
        </form>
        <div className="mt-4">
            {searchMember && <div className="bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 p-4 rounded-md shadow-sm flex flex-col gap-2">
                <div>Email: {searchMember?.email}</div>
                <div>Username: {searchMember?.username}</div>
                <div>User_id: {searchMember?.user_id}</div>
                <button
                    className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white"
                    onClick={() => { handleAddMember() }}
                    disabled={loading}
                >
                    {loading ? "Adding..." : "Add Member"}
                </button>
            </div>}
        </div>
    </ModalAction>
}
