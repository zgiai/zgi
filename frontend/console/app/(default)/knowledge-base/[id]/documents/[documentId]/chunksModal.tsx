import React, { useState, useEffect, MouseEvent } from 'react';

const EditModal = ({ isOpen, onClose, chunk, onSave }: {
    isOpen: boolean;
    onClose: () => void;
    chunk: any;
    onSave: (content: string) => Promise<boolean>;
}) => {
    const [content, setContent] = useState(chunk?.content);
    const [loading, setLoading] = useState<boolean>(false);

    useEffect(() => {
        setContent(chunk?.content || '');
    }, [chunk]);

    const handleOutsideClick = (e: MouseEvent<HTMLDivElement>) => {
        const target = e.target as HTMLElement;
        if (target?.id === 'modal-backdrop') {
            onClose();
        }
    };

    const handleSave = async () => {
        setLoading(true);
        const res = await onSave(content);
        setLoading(false);
        if (!res) {
            return;
        } else {
            onClose();
        }
    };

    useEffect(() => {
        const handleScroll = (e: Event) => {
            if (isOpen) {
                e.preventDefault();
            }
        };
        if (isOpen) {
            window.addEventListener('scroll', handleScroll, { passive: false });
        }
        return () => {
            window.removeEventListener('scroll', handleScroll);
        };
    }, [isOpen]);

    if (!isOpen) return null;

    return (
        <div id='modal-backdrop' className='fixed inset-0 flex justify-center md:justify-end items-center z-50 ' onClick={(e) => handleOutsideClick(e)}>
            <div className='flex flex-col bg-white dark:bg-gray-800 p-4 rounded h-[85vh] shadow-lg w-full mx-4 sm:max-w-xl md:max-w-2xl lg:max-w-2xl z-50 border border-solid border-gray-300 dark:border-gray-600 md:mr-8 mt-[64px]'>
                <div>
                    <h2 className='text-2xl font-bold mb-4'>Edit Chunk</h2>
                    <p className='text-gray-500 mb-4'>Edit the content of the <span className='font-bold'>chunk-{chunk?.chunk_index + 1}</span></p>
                </div>
                <textarea
                    className='form-input resize-none flex-1'
                    value={content}
                    onChange={(e) => setContent(e.target.value)}
                />
                <div className='flex justify-end mt-4 gap-x-2'>
                    <button className='btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white disabled:bg-gray-700 disabled:text-gray-100 disabled:hover:bg-gray-600 disabled:hover:text-gray-100' onClick={handleSave} disabled={loading}>{loading ? 'Saving...' : 'Save'}</button>
                    <button className='btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300' onClick={onClose}>Cancel</button>
                </div>
            </div>
            <div className='fixed inset-0 flex justify-center items-center' id='modal-backdrop' onClick={(e) => handleOutsideClick(e)}>
            </div>
        </div>
    );
};

export default EditModal;