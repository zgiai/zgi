'use client'

import { useState } from 'react'

export default function AccordionBasic({
  children,
  title,
}: {
  children: React.ReactNode
  title: string
}) {

  const [open, setOpen] = useState<boolean>(false)

  return (
    <div className="px-5 py-4 rounded-lg border border-gray-200 dark:border-gray-700/60">
      <button
        className="flex items-center justify-between w-full group mb-1"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <div className="text-sm text-gray-800 dark:text-gray-100 font-medium">{title}</div>
        <svg className={`w-8 h-8 shrink-0 fill-current text-gray-400 dark:text-gray-500 group-hover:text-gray-500 dark:group-hover:text-gray-400 ml-3 ${open && 'rotate-180'}`} viewBox="0 0 32 32">
          <path d="M16 20l-5.4-5.4 1.4-1.4 4 4 4-4 1.4 1.4z" />
        </svg>
      </button>
      <div className={`text-sm ${!open && 'hidden'}`}>
        {children}
      </div>
    </div>
  )
}
