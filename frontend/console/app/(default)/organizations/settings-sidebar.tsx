'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'

export default function SettingsSidebar() {
  const pathname = usePathname()

  const sidebarItems = [
    {
      group: "Organizations Settings",
      items: [
        {
          href: "/organizations/members",
          label: "Members",
          icon: <svg className={`shrink-0 fill-current`} viewBox="0 0 16 16" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="7526" width="16" height="16">
            <path d="M8 9a4 4 0 1 1 0-8 4 4 0 0 1 0 8Zm0-2a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm-5.143 7.91a1 1 0 1 1-1.714-1.033A7.996 7.996 0 0 1 8 10a7.996 7.996 0 0 1 6.857 3.877 1 1 0 1 1-1.714 1.032A5.996 5.996 0 0 0 8 12a5.996 5.996 0 0 0-5.143 2.91Z" />
          </svg>
        },
        {
          href: "/organizations/create",
          label: "New Organization",
          icon: <svg className={`shrink-0 fill-current`} viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="8774" width="16" height="16"><path d="M512 62c-248.4 0-450 201.6-450 450s201.6 450 450 450 450-201.6 450-450-201.6-450-450-450zM725.282 544.733h-172.602v172.611c0 20.753-17.487 38.232-38.242 38.232-20.753 0-38.232-17.478-38.232-38.232v-172.611h-172.62c-20.745 0-38.232-17.478-38.232-38.232 0-20.764 17.487-38.242 38.242-38.242h172.611v-172.611c0-20.753 17.478-38.232 38.232-38.232s38.242 17.478 38.242 38.232v172.62h172.602c20.764 0 38.242 17.469 38.242 38.232 0 21.843-17.478 38.232-38.242 38.232z" p-id="8775"></path></svg>
        },
        // {
        //   href: "/organizations/settings",
        //   label: "Settings",
        //   icon: <svg className={`shrink-0 fill-current`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
        //     <path d="M10.5 1a3.502 3.502 0 0 1 3.355 2.5H15a1 1 0 1 1 0 2h-1.145a3.502 3.502 0 0 1-6.71 0H1a1 1 0 0 1 0-2h6.145A3.502 3.502 0 0 1 10.5 1ZM9 4.5a1.5 1.5 0 1 1 3 0 1.5 1.5 0 0 1-3 0ZM5.5 9a3.502 3.502 0 0 1 3.355 2.5H15a1 1 0 1 1 0 2H8.855a3.502 3.502 0 0 1-6.71 0H1a1 1 0 1 1 0-2h1.145A3.502 3.502 0 0 1 5.5 9ZM4 12.5a1.5 1.5 0 1 0 3 0 1.5 1.5 0 0 0-3 0Z" fillRule="evenodd" />
        //   </svg>
        // },
      ]
    }
  ]

  return (
    <div className="flex flex-nowrap overflow-x-scroll no-scrollbar md:block md:overflow-auto px-3 py-6 border-b md:border-b-0 md:border-r border-gray-200 dark:border-gray-700/60 min-w-[15rem] md:space-y-3">
      {sidebarItems.map((group, index) => (
        <div key={index}>
          <div className="text-xs font-semibold text-gray-400 dark:text-gray-500 uppercase mb-3">{group.group}</div>
          <ul className="flex flex-nowrap md:block mr-3 md:mr-0">
            {group.items.map((item, idx) => (
              <li key={idx} className="mr-0.5 md:mr-0 md:mb-0.5">
                <Link href={item.href} className={`flex items-center px-2.5 py-2 rounded-lg whitespace-nowrap ${pathname.includes(item.href) && 'bg-[linear-gradient(135deg,var(--tw-gradient-stops))] from-violet-500/[0.12] dark:from-violet-500/[0.24] to-violet-500/[0.04]'}`}>
                  <span className={`shrink-0 fill-current mr-2 ${pathname.includes(item.href) ? 'text-violet-500 dark:text-violet-400' : 'text-gray-600 dark:text-gray-300 hover:text-gray-700 dark:hover:text-gray-200'}`}>
                    {item.icon}
                  </span>
                  <span className={`text-sm font-medium ${pathname.includes(item.href) ? 'text-violet-500 dark:text-violet-400' : 'text-gray-600 dark:text-gray-300 hover:text-gray-700 dark:hover:text-gray-200'}`}>{item.label}</span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  )
}