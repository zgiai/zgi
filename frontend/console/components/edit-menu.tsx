'use client'

import { Menu, MenuButton, MenuItems, MenuItem, Transition } from '@headlessui/react'
import Link from 'next/link'

export default function EditMenu({
  align,
  className = '',
}: React.HTMLAttributes<HTMLDivElement> & {
  align?: 'left' | 'right'
  className?: string
}) {
  return (
    <Menu as="div" className={`relative inline-flex ${className}`}>
      {({ open }) => (
        <>
          <MenuButton
            className={`rounded-full ${open ? 'bg-gray-100 dark:bg-gray-700/60 text-gray-500 dark:text-gray-400' : 'text-gray-400 hover:text-gray-500 dark:text-gray-500 dark:hover:text-gray-400'}`}
          >
            <span className="sr-only">Menu</span>
            <svg className="w-8 h-8 fill-current" viewBox="0 0 32 32">
              <circle cx="16" cy="16" r="2" />
              <circle cx="10" cy="16" r="2" />
              <circle cx="22" cy="16" r="2" />
            </svg>
          </MenuButton>
          <Transition
            as="div"
            className={`origin-top-right z-10 absolute top-full min-w-[9rem] bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 py-1.5 rounded-lg shadow-lg overflow-hidden mt-1 ${align === 'right' ? 'right-0' : 'left-0'}`}
            enter="transition ease-out duration-200 transform"
            enterFrom="opacity-0 -translate-y-2"
            enterTo="opacity-100 translate-y-0"
            leave="transition ease-out duration-200"
            leaveFrom="opacity-100"
            leaveTo="opacity-0"
          >
            <MenuItems as="ul" className="focus:outline-none">
              <MenuItem as="li">
                {({ active }) => (
                  <Link className={`font-medium text-sm flex py-1 px-3 ${active ? 'text-gray-800 dark:text-gray-200' : 'text-gray-600 dark:text-gray-300'}`} href="#0">
                    Option 1
                  </Link>
                )}
              </MenuItem>
              <MenuItem as="li">
                {({ active }) => (
                  <Link className={`font-medium text-sm flex py-1 px-3 ${active ? 'text-gray-800 dark:text-gray-200' : 'text-gray-600 dark:text-gray-300'}`} href="#0">
                    Option 2
                  </Link>
                )}
              </MenuItem>
              <MenuItem as="li">
                {({ active }) => (
                  <Link className={`font-medium text-sm flex py-1 px-3 ${active ? 'text-red-600' : 'text-red-500'}`} href="#0">
                    Remove
                  </Link>
                )}
              </MenuItem>                            
            </MenuItems>
          </Transition>
        </>
      )}
    </Menu>
  )
}