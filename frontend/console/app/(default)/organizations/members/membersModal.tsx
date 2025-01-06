import ModalAction from "@/components/modal-action";
import { adminDeleteUser, setAdmin, unSetAdmin } from "@/services/admin";
import { message } from "antd";
import { useState } from "react";

export function DeleteMemberModal({ isOpen, setIsOpen, currentMember, getMemberList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentMember: any, getMemberList: () => void }) {
    const [loading, setLoading] = useState(false);
    const handleDeleteMember = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await adminDeleteUser({ user_id: currentMember?.id });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Delete member success");
                getMemberList();
            } else {
                message.error(res.message || "Delete member failed");
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
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>
}

export function SetAdminModal({ isOpen, setIsOpen, currentMember, getMemberList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentMember: any, getMemberList: () => void }) {
    const [loading, setLoading] = useState(false);

    const handleSetAdmin = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await setAdmin({ user_id: currentMember?.id });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Set admin success");
                getMemberList();
            } else {
                message.error(res.message || "Set admin failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Set Admin</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to set this member as admin?</div>
        <form onSubmit={handleSetAdmin} className="flex justify-end gap-4">
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? "Setting..." : "Set Admin"}</button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>
}

export function UnsetAdminModal({ isOpen, setIsOpen, currentMember, getMemberList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentMember: any, getMemberList: () => void }) {
    const [loading, setLoading] = useState(false);

    const handleUnsetAdmin = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await unSetAdmin({ user_id: currentMember?.id });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Unset admin success");
                getMemberList();
            } else {
                message.error(res.message || "Unset admin failed");
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
