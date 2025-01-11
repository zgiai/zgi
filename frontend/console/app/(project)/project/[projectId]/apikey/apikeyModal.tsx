import ModalAction from "@/components/modal-action";
import { createApiKey, deleteApiKey, updateApiKey, enableApiKey, disableApiKey } from "@/services/apikey";
import { message } from "antd";
import { useEffect, useState } from "react";

export function CreateApiKeyModal({ isOpen, setIsOpen, projectId, getApiKeyList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, projectId: string, getApiKeyList: () => void }) {
    const [apiKeyName, setApiKeyName] = useState("");
    const [loading, setLoading] = useState(false);

    const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await createApiKey({ name: apiKeyName }, { project_id: projectId });
            if (res.status_code === 200) {
                setIsOpen(false);
                setApiKeyName("");
                message.success("Create api key success");
                getApiKeyList();
            } else {
                message.error(res.status_message || "Create api key failed");
            }

        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Create API Key</div>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
                <label htmlFor="name" className="text-gray-800 dark:text-gray-100">Api Key Name</label>
                <input id="name" className="form-input w-full" placeholder="my-api-key" type="text" value={apiKeyName} onChange={(e) => setApiKeyName(e.target.value.trim())} />
            </div>
            <div className="flex justify-end gap-4">
                <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? "Creating..." : "Create"}</button>
                <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
            </div>
        </form>
    </ModalAction>
}

export function DeleteApiKeyModal({ isOpen, setIsOpen, currentApiKey, getApiKeyList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentApiKey: any, getApiKeyList: () => void }) {
    const [loading, setLoading] = useState(false);
    const handleDeleteApiKey = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await deleteApiKey({ api_key_uuid: currentApiKey?.uuid });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Delete api key success");
                getApiKeyList();
            } else {
                message.error(res.status_message || "Delete api key failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }
    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Delete API Key</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to delete this API key?</div>
        <form onSubmit={handleDeleteApiKey} className="flex justify-end gap-4">
            <button className="btn bg-red-500 text-white" type="submit" disabled={loading}>{loading ? "Deleting..." : "Delete"} </button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>
}

export function UpdateApiKeyModal({ isOpen, setIsOpen, currentApiKey, getApiKeyList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentApiKey: any, getApiKeyList: () => void }) {
    const [apiKeyName, setApiKeyName] = useState(currentApiKey?.name);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
        setApiKeyName(currentApiKey?.name);
    }, [currentApiKey]);

    const handleUpdateApiKey = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await updateApiKey({ name: apiKeyName }, { api_key_uuid: currentApiKey?.uuid });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Update api key success");
                getApiKeyList();
            } else {
                message.error(res.status_message || "Update api key failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Update API Key</div>
        <form onSubmit={handleUpdateApiKey} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
                <label htmlFor="name" className="text-gray-800 dark:text-gray-100">Api Key Name</label>
                <input id="name" className="form-input w-full" placeholder="my-api-key" type="text" value={apiKeyName} onChange={(e) => setApiKeyName(e.target.value.trim())} />
            </div>
            <div className="flex justify-end gap-4">
                <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? "Updating..." : "Update"}</button>
                <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
            </div>
        </form>
    </ModalAction>
}

export const DisableApiKeyModal = ({ isOpen, setIsOpen, currentApiKey, getApiKeyList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentApiKey: any, getApiKeyList: () => void }) => {

    const [loading, setLoading] = useState(false);
    const handleDisableApiKey = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await disableApiKey({ api_key_uuid: currentApiKey?.uuid });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Disable api key success");
                getApiKeyList();
            } else {
                message.error(res.status_message || "Disable api key failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Disable API Key</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to disable this API key?</div>
        <form className="flex justify-end gap-4" onSubmit={handleDisableApiKey}>
            <button className="btn bg-red-500 text-white hover:bg-red-600" type="submit" disabled={loading} >{loading ? "Disabling..." : "Disable"}</button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)} >Cancel</button>
        </form>
    </ModalAction>
}

export const EnableApiKeyModal = ({ isOpen, setIsOpen, currentApiKey, getApiKeyList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentApiKey: any, getApiKeyList: () => void }) => {

    const [loading, setLoading] = useState(false);
    const handleEnableApiKey = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await enableApiKey({ api_key_uuid: currentApiKey?.uuid });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Enable api key success");
                getApiKeyList();
            } else {
                message.error(res.status_message || "Enable api key failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Enable API Key</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to enable this API key?</div>
        <form className="flex justify-end gap-4" onSubmit={handleEnableApiKey}>
            <button className="btn bg-green-500 text-white hover:bg-green-600" type="submit" disabled={loading} >{loading ? "Enabling..." : "Enable"}</button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)} >Cancel</button>
        </form>
    </ModalAction>
}