"use client"
import { useParams } from "next/navigation"

import SettingsSidebar from "@/components/ui/settings-sidebar"

export default function Layout({ children }: { children: React.ReactNode }) {
    const { organizationId } = useParams()

    const sidebarItems = [
        {
            group: "Organization Settings",
            items: [
                {
                    href: `/organization/${organizationId}/settings/settings`,
                    label: "Settings",
                    icon: <svg className={`shrink-0 fill-current`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10.5 1a3.502 3.502 0 0 1 3.355 2.5H15a1 1 0 1 1 0 2h-1.145a3.502 3.502 0 0 1-6.71 0H1a1 1 0 0 1 0-2h6.145A3.502 3.502 0 0 1 10.5 1ZM9 4.5a1.5 1.5 0 1 1 3 0 1.5 1.5 0 0 1-3 0ZM5.5 9a3.502 3.502 0 0 1 3.355 2.5H15a1 1 0 1 1 0 2H8.855a3.502 3.502 0 0 1-6.71 0H1a1 1 0 1 1 0-2h1.145A3.502 3.502 0 0 1 5.5 9ZM4 12.5a1.5 1.5 0 1 0 3 0 1.5 1.5 0 0 0-3 0Z" fillRule="evenodd" />
                    </svg>
                },
                {
                    href: `/organization/${organizationId}/settings/members`,
                    label: "Members",
                    icon: <svg className={`shrink-0 fill-current`} viewBox="0 0 16 16" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="7526" width="16" height="16">
                        <path d="M8 9a4 4 0 1 1 0-8 4 4 0 0 1 0 8Zm0-2a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm-5.143 7.91a1 1 0 1 1-1.714-1.033A7.996 7.996 0 0 1 8 10a7.996 7.996 0 0 1 6.857 3.877 1 1 0 1 1-1.714 1.032A5.996 5.996 0 0 0 8 12a5.996 5.996 0 0 0-5.143 2.91Z" />
                    </svg>
                },
                // {
                //   href: `/organization/${organizationId}/settings/create`,
                //   label: "New Project",
                //   icon: <svg className={`shrink-0 fill-current`} viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="8774" width="16" height="16"><path d="M512 62c-248.4 0-450 201.6-450 450s201.6 450 450 450 450-201.6 450-450-201.6-450-450-450zM725.282 544.733h-172.602v172.611c0 20.753-17.487 38.232-38.242 38.232-20.753 0-38.232-17.478-38.232-38.232v-172.611h-172.62c-20.745 0-38.232-17.478-38.232-38.232 0-20.764 17.487-38.242 38.242-38.242h172.611v-172.611c0-20.753 17.478-38.232 38.232-38.232s38.242 17.478 38.242 38.232v172.62h172.602c20.764 0 38.242 17.469 38.242 38.232 0 21.843-17.478 38.232-38.242 38.232z" p-id="8775"></path></svg>
                // },

            ]
        }
    ]

    return (
        <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">
            {/* Page header */}
            <div className="mb-8">
                {/* Title */}
                <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Organizations Settings</h1>
            </div>
            {/* Content */}
            <div className="bg-white dark:bg-gray-800 shadow-sm rounded-xl mb-8">
                <div className="flex flex-col md:flex-row md:-mr-px">
                    <SettingsSidebar sidebarItems={sidebarItems} />
                    {children}
                </div>
            </div>
        </div>
    )
}