"use client"

import { message, Slider } from "antd";
import { useState, useEffect } from "react";
import ModalAction from "@/components/modal-action";
import Tooltip from "@/components/tooltip";

export function SettingModal({ isOpen, setIsOpen, topK, setTopK }: { isOpen: boolean, setIsOpen: (value: boolean) => void, topK: any, setTopK: any }) {
    const [newTopK, setNewTopK] = useState(topK);

    useEffect(() => {
        setNewTopK(topK)
    }, [topK])

    const handleSave = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        setTopK(newTopK)
        setIsOpen(false)
    }

    return <ModalAction isOpen={isOpen} setIsOpen={setIsOpen}>
        <div className="text-lg text-gray-800 dark:text-gray-100 font-bold mb-6">Search Setting</div>
        {/* <div className="text-lg text-gray-800 dark:text-gray-100 mb-6">Are you sure you want to set this member as admin?</div> */}
        <form onSubmit={handleSave} className="flex flex-col gap-4">
            <div className="flex items-center">
                <label className="block text-lg font-medium mb-1" htmlFor="separators">
                    Top K
                </label>
                <Tooltip className="ml-2" bg="dark" size="md" position="right">
                    <div className="text-sm text-gray-200">
                        The number of results to return.
                    </div>
                </Tooltip>
            </div>
            <div className="flex items-center gap-4">
                <div className="w-full">
                    <Slider
                        min={1}
                        max={10}
                        onChange={(value: number) => setNewTopK(value)}
                        value={typeof newTopK === 'number' ? newTopK : 0}
                    />
                </div>
                <div className="flex items-center border border-solid border-[#EAEAEA] h-[30px] rounded-[4px] px-[10px] text-lg font-medium">
                    {typeof newTopK === 'number' ? newTopK : 0}
                </div>
            </div>

            <div className="flex justify-end gap-4">
                <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" type="submit" >{"Save"}</button>
                <button className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" type="button" onClick={() => setIsOpen(false)}>Cancel</button>
            </div>
        </form>
    </ModalAction>
}