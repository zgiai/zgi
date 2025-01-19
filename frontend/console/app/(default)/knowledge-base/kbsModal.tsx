import ModalAction from '@/components/modal-action';
import { createKnowledgeBase, deleteKnowledgeBase } from '@/services/knowledgeBase';
import { message } from 'antd';
import { useState } from 'react';

export const AddKbsModal = ({ isOpen, setIsOpen, getKbsList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, getKbsList: () => void }) => {
    const [formData, setFormData] = useState({
        name: '',
        description: '',
    });
    const [loading, setLoading] = useState(false);
    const [errors, setErrors] = useState({
        name: false,
        description: false,
    });

    const handleAddEntry = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);

        // Validation logic
        const newErrors = {
            name: !formData.name.trim(),
            description: !formData.description.trim(),
        };
        setErrors(newErrors);

        if (newErrors.name || newErrors.description) {
            message.error('Please fill in all fields.');
            setLoading(false);
            return;
        }

        try {
            const res = await createKnowledgeBase(formData);
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success('Add knowledge base entry success');
                getKbsList();
            } else {
                message.error(res.status_message || 'Add knowledge base entry failed');
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    };

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Add Knowledge Base Entry</div>
        <form onSubmit={handleAddEntry} className="flex flex-col gap-4">
            <div className="flex flex-row gap-4 items-center flex-wrap text-right">
                <label className="text-gray-800 dark:text-gray-100 w-24 text-right">Name</label>
                <input
                    type="text"
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    className={`form-input w-full max-w-xs ${errors.name ? 'border-red-500' : ''}`}
                    placeholder="My-Knowledge-Base"
                />
            </div>
            <div className="flex flex-row gap-4 items-center flex-wrap text-right">
                <label className="text-gray-800 dark:text-gray-100 w-24 text-right">Description</label>
                <input
                    type="text"
                    value={formData.description}
                    onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                    className={`form-input w-full max-w-xs ${errors.description ? 'border-red-500' : ''}`}
                    placeholder="My-Knowledge-Base-description"
                />
            </div>
            <div className="flex justify-end gap-4">
                <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800" type="submit" disabled={loading}>{loading ? 'Creating...' : 'Create'}</button>
                <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" disabled={loading} onClick={() => setIsOpen(false)}>Cancel</button>
            </div>
        </form>
    </ModalAction>;
};

export const DeleteKbsModal = ({ isOpen, setIsOpen, currentEntry, getKbsList }: { isOpen: boolean, setIsOpen: (value: boolean) => void, currentEntry: any, getKbsList: () => void }) => {
    const [loading, setLoading] = useState(false);

    const handleDeleteEntry = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setLoading(true);
        try {
            // Call the service to delete the knowledge base entry
            const res = await deleteKnowledgeBase({ kb_id: currentEntry.id }); // Assuming deleteKbsEntry is defined in your services
            if (res.status_code === 200) {
                setIsOpen(false);
                message.success('Delete knowledge base entry success');
                getKbsList();
            } else {
                message.error(res.status_message || 'Delete knowledge base entry failed');
            }
        } catch (error) {
            console.error(error);
        } finally {
            setLoading(false);
        }
    };

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Delete Knowledge Base Entry</div>
        <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to delete {currentEntry.name}?</div>
        <form onSubmit={handleDeleteEntry} className="flex justify-end gap-4">
            <button className="btn bg-red-500 text-white" type="submit" disabled={loading}>{loading ? 'Deleting...' : 'Delete'}</button>
            <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
        </form>
    </ModalAction>;
};