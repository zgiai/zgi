"use client";

import ModalAction from "@/components/modal-action";
import { deleteOrganization, updateOrganization } from "@/services/organization";
import { message } from "antd";
import { useState, useEffect } from "react";

export const DeleteOrganizationModal = ({
    isOpen, setIsOpen, orgId
}: {
    isOpen: boolean, setIsOpen: (value: boolean) => void, orgId: string
}) => {
    const [loading, setLoading] = useState(false);
    const handleDeleteOrg = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await deleteOrganization({ organization_id: orgId });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Delete orgnization success");
                location.href = "/organizations";
            } else {
                message.error(res.status_message || "Delete orgnization failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Delete Orgnization</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to delete this orgnization?</div>
        <form onSubmit={handleDeleteOrg} className="flex justify-end gap-4">
            <button className="btn bg-red-500 text-white" type="submit" disabled={loading}>{loading ? "Deleting..." : "Delete"} </button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>;
};

export const QuitOrganizationModal = ({
    isOpen, setIsOpen, orgId
}: {
    isOpen: boolean, setIsOpen: (value: boolean) => void, orgId: string
}) => {
    const [loading, setLoading] = useState(false);

    const handleQuitOrg = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await deleteOrganization({ organization_id: orgId });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Quit orgnization success");
                location.href = "/organizations";
            } else {
                message.error(res.status_message || "Quit orgnization failed");
            }
        } catch (error) {
            console.error(error);
            setIsOpen(false);
        } finally {
            setLoading(false);
        }
    }
    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Quit Orgnization</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to quit this orgnization?</div>
        <form className="flex justify-end gap-4" onSubmit={handleQuitOrg}>
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? "Quitting..." : "Quit"}</button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>
}

export const EditOrganizationModal = ({
    isOpen, setIsOpen, orgId, organizationInfo, isAdmin
}: {
    isOpen: boolean, setIsOpen: (value: boolean) => void, orgId: string, organizationInfo: any, isAdmin: boolean
}) => {
    const [loading, setLoading] = useState(false);
    const [formData, setFormData] = useState({
        name: "",
        description: "",
        isActive: true
    });

    useEffect(() => {
        setFormData({ isActive: organizationInfo?.is_active, name: organizationInfo?.name || "", description: organizationInfo?.description || "" });
    }, [organizationInfo]);

    const handleDeleteOrg = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await updateOrganization({ organization_id: orgId }, formData);
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Delete orgnization success");
                location.href = "/organizations";
            } else {
                message.error(res.status_message || "Delete orgnization failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Delete Orgnization</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to delete this orgnization?</div>
        <form onSubmit={handleDeleteOrg} className="flex gap-4 flex-col">
            <div className="flex flex-row gap-4 items-center flex-wrap">
                <label className="text-gray-800 dark:text-gray-100 w-24 text-right">Name</label>
                <input
                    type="text"
                    placeholder="My orgnization"
                    className="form-input w-full max-w-xs"
                    value={formData?.name}
                    onChange={(e) => { setFormData({ ...formData, name: e.target.value }) }}
                />
            </div>
            <div className="flex flex-row gap-4 items-center flex-wrap text-right">
                <label className="text-gray-800 dark:text-gray-100 w-24">Description</label>
                <input
                    type="text"
                    placeholder="My orgnization"
                    className="form-input w-full max-w-xs"
                    value={formData?.description}
                    onChange={(e) => { setFormData({ ...formData, description: e.target.value }) }}
                />
            </div>
            {isAdmin && <div className="flex flex-row gap-4 items-center flex-wrap text-right">
                <label className="text-gray-800 dark:text-gray-100 w-24">Status</label>
                <div className="flex flex-row gap-4 items-center">
                    <div className="flex items-center">
                        <div className="form-switch">
                            <input type="checkbox" id="switch-1" className="sr-only" checked={formData?.isActive} onChange={() => setFormData({ ...formData, isActive: !formData?.isActive })} />
                            <label className="bg-gray-400 dark:bg-gray-700" htmlFor="switch-1">
                                <span className="bg-white shadow-sm" aria-hidden="true"></span>
                                <span className="sr-only">Switch label</span>
                            </label>
                        </div>
                        <div className="text-sm text-gray-400 dark:text-gray-500 italic ml-2">{formData?.isActive ? 'active' : 'disabled'}</div>
                    </div>
                </div>
            </div>}
            <div className="flex flex-row gap-4 items-center justify-end">
                <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? "Saving..." : "Save"} </button>
                <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
            </div>
        </form>
    </ModalAction>
}   