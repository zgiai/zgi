import React, { useState, useEffect, MouseEvent } from 'react';

const EditModal = ({ isOpen, onClose, initialContent, onSave }:{
    isOpen: boolean;
    onClose: () => void;
    initialContent: string;
    onSave: (content: string) => void;
}) => {
    const [content, setContent] = useState(initialContent);

    const handleOutsideClick = (e: MouseEvent<HTMLDivElement>) => {
        const target = e.target as HTMLElement;
        if (target?.id === 'modal-backdrop') {
            onClose();
        }
    };

    const handleSave = () => {
        onSave(content);
        onClose();
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
        <div id='modal-backdrop' className='fixed inset-0 flex justify-center items-center z-50' onClick={(e) => handleOutsideClick(e)}>
            <div className='bg-white p-4 rounded shadow-lg w-full mx-4 sm:max-w-xs md:max-w-md lg:max-w-lg z-50'>
                <textarea
                    className='w-full h-32 border border-gray-300 rounded p-2 resize-none'
                    value={content}
                    onChange={(e) => setContent(e.target.value)}
                />
                <div className='flex justify-end mt-4'>
                    <button className='bg-gray-300 text-gray-700 px-4 py-2 rounded mr-2' onClick={onClose}>取消</button>
                    <button className='bg-gray-700 text-white px-4 py-2 rounded' onClick={handleSave}>保存</button>
                </div>
            </div>
            <div className='fixed inset-0 flex justify-center items-center' onClick={(e) => handleOutsideClick(e)}>
            </div>
        </div>
    );
};

export default EditModal;