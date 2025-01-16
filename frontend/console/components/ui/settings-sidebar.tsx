'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'


export default function SettingsSidebar({ sidebarItems }: { sidebarItems: { group: string, items: { href: string, label: string, icon: JSX.Element }[] }[] }) {
  const pathname = usePathname()
  
  return (
    <div className="flex flex-nowrap overflow-x-scroll no-scrollbar md:block md:overflow-auto px-3 py-6 border-b md:border-b-0 md:border-r border-gray-200 dark:border-gray-700/60 min-w-[15rem] md:space-y-3">
      {sidebarItems?.map((group, index) => (
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