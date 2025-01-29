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
                    href: `/knowledge-base/${id}/hitTest`,
                    label: "Hit Test",
                    icon: <svg className={`shrink-0 fill-current`} viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" width="16" height="16">
                        <path d="M340.6652507 173.99233877l11.21565469 82.5042762L593.02969685 419.17541262c34.96727325 23.82846293 43.95912419 71.51915235 20.1655899 106.45266091-23.84592669 34.9661093-71.5121664 44.03596743-106.47943964 20.19004074L265.57520555 383.19054507l-80.9173413 19.74178702-164.98177366-112.40104391c-10.15614578-6.93104867-15.18124373-19.27723349-12.71643022-31.32885106 2.44851257-12.05278265 11.9142309-21.44980765 23.97516345-23.84592669l89.71941205-17.7939217c16.33738866-3.25886179 27.06054599-18.98383133 24.09508637-35.38176228l-16.25938034-89.98603548c-2.18072519-12.10401109 3.10401024-24.32910905 13.42315748-31.01914908 10.32031118-6.6900389 23.63984669-6.55148829 33.80297841 0.39702414L340.6652507 173.99233877z"></path>
                        <path d="M176.05488754 450.38317682l14.77606855-3.58602866c-1.06067285 11.34489145-1.74178531 22.82833351-1.74178532 34.44916224 0 199.90480327 162.61941817 362.53237248 362.54052238 362.53237248 199.88850347 0 362.52422144-162.62756807 362.52422144-362.53237248 0-199.92226816-162.63688306-362.54983623-362.52422144-362.54983623-60.75175139 0-117.96986766 15.20802247-168.30001379 41.70854172l-1.61254855-11.77684423-70.6063451-48.0877147c69.66675911-44.13958941 151.96379022-70.00323982 240.51890744-70.00323982 248.90765312 0 450.70909326 201.76767545 450.70909327 450.70909326S800.53851022 931.97286855 551.6308571 931.97286855c-248.94141781 0-450.73470805-201.78514034-450.73470805-450.72655815 0-26.38059747 2.72445099-52.08823239 7.07891428-77.21022691L176.05488754 450.38317682z"></path>
                        <path d="M551.6308571 314.65431723c-10.38784057 0-20.50905771 1.10258745-30.37995121 2.89676629l-90.11643734-60.81345877c35.86378069-19.27606841 76.88306005-30.2600283 120.49638742-30.2600283 140.68759211 0 254.76289309 114.03920725 254.76289308 254.76987904 0 140.7132069-114.07530098 254.75125021-254.76289308 254.7512502-140.71204295 0-254.78617885-114.03804331-254.78617885-254.7512502 0-7.7413979 0.49133227-15.39780153 1.17244473-22.98434902l91.78720028 61.91721131c17.62160526 73.08862237 83.38331875 127.64399502 161.82536989 127.64399502 91.85007275 0 166.57569337-74.71048477 166.57569337-166.57685731C718.20655047 389.36363805 643.48092985 314.65431723 551.6308571 314.65431723z"></path>
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
                        <div className={`bg-gray-100 dark:bg-gray-900 flex pr-[1px] border-r border-gray-200 dark:border-gray-700/60 border-solid`}>
                            <div className={`flex-col md:flex-row md:-mr-px hidden md:flex overflow-hidden rounded-l-xl`}>
                                <button
                                    onClick={() => setHiddenSidebar(!hiddenSidebar)}
                                    className="text-gray-600 dark:text-gray-300 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 flex items-center justify-center"
                                >
                                    <svg className={`w-4 h-4 shrink-0 fill-current text-gray-800 dark:text-gray-200 ${hiddenSidebar ? 'rotate-90 ml-1' : '-rotate-90 mr-1'}`} viewBox="0 0 12 12">
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