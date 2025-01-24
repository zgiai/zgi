"use client"
import { useParams } from "next/navigation"

import SettingsSidebar from "@/components/ui/settings-sidebar"
import { KnowledgeBaseProvider } from "./knowledgeProvider"
import KbHeader from "./kbHeader"
import { useState } from "react"

export default function Layout({ children }: { children: React.ReactNode }) {
    const { id } = useParams()
    const kbId = Array.isArray(id) ? id[0] : id;
    const sidebarItems = [
        {
            group: "Knowledge Base",
            items: [
                {
                    href: `/knowledge-base/${id}/documents`,
                    label: "Documents",
                    icon: <svg className={`shrink-0 fill-current`} viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" width="16" height="16">
                        <path d="M521.563 53v250.814l0.013 1.56c0.833 51.385 42.756 92.783 94.35 92.783H876v407.285C876 875.338 819.326 932 749.415 932h-494.83C184.674 932 128 875.338 128 805.442V179.558C128 109.662 184.674 53 254.585 53h266.978z m143.052 643.143l-382.056 1.15-0.706 0.01c-16.197 0.425-29.172 13.71-29.124 29.994 0.05 16.355 13.219 29.606 29.516 29.82l0.495 0.003 382.055-1.15 0.706-0.01c16.197-0.424 29.173-13.71 29.124-29.994-0.05-16.52-13.486-29.873-30.01-29.823zM488.449 484.446H282.203l-0.706 0.009c-16.198 0.375-29.214 13.62-29.214 29.905 0 16.356 13.13 29.645 29.425 29.91l0.495 0.004H488.45l0.706-0.009c16.198-0.375 29.214-13.62 29.214-29.905 0-16.52-13.396-29.914-29.92-29.914z m71.703-422.552l0.574 0.535 299.147 286.514a30.11 30.11 0 0 1 3.384 3.841 46.18 46.18 0 0 1 6.458 8.556H615.926l-0.951-0.007c-31.34-0.508-56.587-26.066-56.587-57.519V60.408c0.602 0.469 1.19 0.964 1.764 1.486z" fill="currentColor"></path>
                    </svg>
                },
                {
                    href: `/knowledge-base/${id}/settings`,
                    label: "Settings",
                    icon: <svg className={`shrink-0 fill-current`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10.5 1a3.502 3.502 0 0 1 3.355 2.5H15a1 1 0 1 1 0 2h-1.145a3.502 3.502 0 0 1-6.71 0H1a1 1 0 0 1 0-2h6.145A3.502 3.502 0 0 1 10.5 1ZM9 4.5a1.5 1.5 0 1 1 3 0 1.5 1.5 0 0 1-3 0ZM5.5 9a3.502 3.502 0 0 1 3.355 2.5H15a1 1 0 1 1 0 2H8.855a3.502 3.502 0 0 1-6.71 0H1a1 1 0 1 1 0-2h1.145A3.502 3.502 0 0 1 5.5 9ZM4 12.5a1.5 1.5 0 1 0 3 0 1.5 1.5 0 0 0-3 0Z" fillRule="evenodd" />
                    </svg>
                }
            ]
        }
    ]

    const [hiddenSidebar, setHiddenSidebar] = useState<boolean>(false);

    return (
        <KnowledgeBaseProvider>
            <div className="px-4 sm:px-6 lg:px-4 py-4 w-full max-w-[96rem] mx-auto">
                <div className="bg-white dark:bg-gray-800 shadow-sm rounded-xl mb-8 flex flex-col md:flex-row md:-mr-px relative">
                    <div className="h-[calc(100vh-64px)] sticky top-[64px] overflow-y-auto hidden md:flex">
                        <div className={`bg-gray-100 dark:bg-gray-900 flex pr-[1px] ${hiddenSidebar ? 'border-r border-gray-200 dark:border-gray-700/60 border-solid' : ''}`}>
                            <div className={`flex-col md:flex-row md:-mr-px hidden md:flex overflow-hidden rounded-l-xl`}>
                                <button onClick={() => setHiddenSidebar(!hiddenSidebar)} className="text-gray-600 dark:text-gray-300 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700">
                                    <svg className={`w-3 h-3 shrink-0 ml-1 fill-current text-gray-400 dark:text-gray-500 ${hiddenSidebar ? 'rotate-90' : '-rotate-90'}`} viewBox="0 0 12 12">
                                        <path d="M5.9 11.4L.5 6l1.4-1.4 4 4 4-4L11.3 6z" />
                                    </svg>
                                </button>
                            </div>
                        </div>
                        <div className={`${hiddenSidebar ? 'hidden' : 'flex'}`}>
                            <SettingsSidebar sidebarItems={sidebarItems} />
                        </div>
                    </div>
                    <div className="md:hidden">
                        <SettingsSidebar sidebarItems={sidebarItems} />
                    </div>
                    <div className="flex flex-col flex-1 min-h-[80vh]">
                        <KbHeader kbId={kbId} />
                        {children}
                    </div>
                </div>
            </div>
        </KnowledgeBaseProvider>
    )
}