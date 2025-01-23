import ModalAction from "@/components/modal-action";
import { deleteDocument, updateDocument } from "@/services/knowledgeBase";
import { message } from "antd";
import { useState, useEffect } from "react";

export function UpdateDocumentModal({ isOpen, setIsOpen, currentDocument, getDocumentList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentDocument: any, getDocumentList: () => void }) {
    const [documentTitle, setDocumentTitle] = useState(currentDocument?.title || currentDocument?.file_name);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
        setDocumentTitle(currentDocument?.title || currentDocument?.file_name);
    }, [currentDocument]);

    const handleUpdateDocument = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await updateDocument(currentDocument?.id, { title: documentTitle });
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Update document success");
                getDocumentList();
            } else {
                message.error(res.status_message || "Update document failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Update Document</div>
        <form onSubmit={handleUpdateDocument} className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
                <label htmlFor="title" className="text-gray-800 dark:text-gray-100">Document Title</label>
                <input id="title" className="form-input w-full" placeholder="Document Title" type="text" value={documentTitle} onChange={(e) => setDocumentTitle(e.target.value.trim())} />
            </div>
            <div className="flex justify-end gap-4">
                <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" disabled={loading}>{loading ? "Updating..." : "Update"}</button>
                <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
            </div>
        </form>
    </ModalAction>
}

export function DeleteDocumentModal({ isOpen, setIsOpen, currentDocument, getDocumentList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentDocument: any, getDocumentList: () => void }) {
    const [loading, setLoading] = useState(false);

    const handleDeleteDocument = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            const res = await deleteDocument(currentDocument?.id);
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success("Delete document success");
                getDocumentList();
            } else {
                message.error(res.status_message || "Delete document failed");
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Delete Document</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to delete this document?</div>
        <form className="flex justify-end gap-4" onSubmit={handleDeleteDocument}>
            <button className="btn bg-red-500 text-white" type="submit" disabled={loading}>{loading ? "Deleting..." : "Delete"}</button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>
}
