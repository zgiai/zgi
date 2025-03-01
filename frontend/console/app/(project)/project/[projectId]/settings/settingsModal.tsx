"use client";

import ModalAction from "@/components/modal-action";
import { deleteProject, updateProject } from "@/services/project";
import { message } from "antd";
import { useState, useEffect } from "react";
import { useAppProvider } from "@/app/app-provider";
import { projectLang } from "@/app/(project)/project/lang";


export const DeleteProjectModal = ({
    isOpen, setIsOpen, projectId
}: {
    isOpen: boolean, setIsOpen: (value: boolean) => void, projectId: string
}) => {
    const [loading, setLoading] = useState(false);
    const { language } = useAppProvider();
    const handleDeleteOrg = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await deleteProject({ project_id: projectId });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Delete project success");
                location.href = `/organizations`;
            } else {
                message.error(res.status_message || "Delete project failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">{projectLang[language].deleteProject}</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">{projectLang[language].deleteProjectConfirmation}</div>
        <form onSubmit={handleDeleteOrg} className="flex justify-end gap-4">
            <button className="btn bg-red-500 text-white" type="submit" disabled={loading}>{loading ? projectLang[language].deleting : projectLang[language].delete} </button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>{projectLang[language].cancel}</button>
        </form>
    </ModalAction>;
};

export const EditProjectModal = ({
    isOpen, setIsOpen, projuctId, projectInfo, isAdmin
}: {
    isOpen: boolean, setIsOpen: (value: boolean) => void, projuctId: string, projectInfo: any, isAdmin: boolean
}) => {
    const { language } = useAppProvider();
    const [loading, setLoading] = useState(false);
    const [active, setActive] = useState(true);
    const [formData, setFormData] = useState({
        name: "",
        description: "",
        status: "active",
    });

    useEffect(() => {
        setFormData({ status: projectInfo?.status, name: projectInfo?.name || "", description: projectInfo?.description || "" });
        if (projectInfo?.status === 'active') {
            setActive(true);
        } else {
            setActive(false);
        }
    }, [projectInfo]);

    const handleDeleteOrg = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await updateProject({ project_id: projuctId }, {...formData,status: active ? 'active' : 'disabled'});
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success(projectLang[language].editProjectSuccess);
                location.href = "/organizations";
            } else {
                message.error(res.status_message || projectLang[language].editProjectFailed);
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">{projectLang[language].editProject}</div>
        <form onSubmit={handleDeleteOrg} className="flex gap-4 flex-col">
            <div className="flex flex-row gap-4 items-center flex-wrap">
                <label className="text-gray-800 dark:text-gray-100 w-24 text-right">{projectLang[language].name}</label>
                <input
                    type="text"
                    placeholder="My orgnization"
                    className="form-input w-full max-w-xs"
                    value={formData?.name}
                    onChange={(e) => { setFormData({ ...formData, name: e.target.value }) }}
                />
            </div>
            <div className="flex flex-row gap-4 items-center flex-wrap text-right">
                <label className="text-gray-800 dark:text-gray-100 w-24">{projectLang[language].description}</label>
                <input
                    type="text"
                    placeholder="My orgnization"
                    className="form-input w-full max-w-xs"
                    value={formData?.description}
                    onChange={(e) => { setFormData({ ...formData, description: e.target.value }) }}
                />
            </div>
            {isAdmin && <div className="flex flex-row gap-4 items-center flex-wrap text-right">
                <label className="text-gray-800 dark:text-gray-100 w-24">{projectLang[language].status}</label>
                <div className="flex flex-row gap-4 items-center">
                    <div className="flex items-center">
                        <div className="form-switch">
                            <input type="checkbox" id="switch-1" className="sr-only" checked={active} onChange={() => setActive(!active)} />
                            <label className="bg-gray-400 dark:bg-gray-700" htmlFor="switch-1">
                                <span className="bg-white shadow-sm" aria-hidden="true"></span>
                                <span className="sr-only">Switch label</span>
                            </label>
                        </div>
                        <div className="text-sm text-gray-400 dark:text-gray-500 italic ml-2">{active ? projectLang[language].active : projectLang[language].disabled}</div>
                    </div>
                </div>
            </div>}
            <div className="flex flex-row gap-4 items-center justify-end">
                <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? projectLang[language].saving : projectLang[language].save} </button>
                <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>{projectLang[language].cancel}</button>
            </div>
        </form>
    </ModalAction>
}   