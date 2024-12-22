'use client'

import { Menu, MenuButton, MenuItems, MenuItem, Transition } from '@headlessui/react'
import Link from 'next/link'

export default function TransactionDropdown({
  align,
}: {
  align?: 'left' | 'right',
}) {
  return (
    <Menu as="div" className="relative inline-flex">
      <MenuButton className="inline-flex justify-center items-center group">
        <div className="flex items-center truncate">
          <span className="truncate font-medium text-violet-500 group-hover:text-violet-600 dark:group-hover:text-violet-400">My Personal Account</span>
          <svg className="w-3 h-3 shrink-0 ml-1 fill-current text-violet-400" viewBox="0 0 12 12">
            <path d="M5.9 11.4L.5 6l1.4-1.4 4 4 4-4L11.3 6z" />
          </svg>
        </div>
      </MenuButton>
      <Transition
        as="div"
        className={`origin-top-right z-10 absolute top-full min-w-[11rem] bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 py-1.5 rounded-lg shadow-lg overflow-hidden mt-1 ${
          align === 'right' ? 'right-0' : 'left-0'
        }`}
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
                Business Account
              </Link>
            )}
          </MenuItem>
          <MenuItem as="li">
            {({ active }) => (
              <Link className={`font-medium text-sm flex py-1 px-3 ${active ? 'text-gray-800 dark:text-gray-200' : 'text-gray-600 dark:text-gray-300'}`} href="#0">
                Family Account
              </Link>
            )}
          </MenuItem>
        </MenuItems>
      </Transition>
    </Menu>
  )
}