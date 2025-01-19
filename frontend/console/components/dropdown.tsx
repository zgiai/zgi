'use client'

import { Menu, MenuButton, MenuItems, MenuItem, Transition } from '@headlessui/react'

interface DropdownOption {
  label: string
  action: () => void
}

const DropdownHelp = ({ options }: { options: DropdownOption[] }) => {
  return (
    <Menu as="div" className="relative inline-flex">
      {({ open }) => (
        <>
          <MenuButton
            className={`w-8 h-8 flex items-center justify-center hover:bg-gray-100 dark:hover:bg-gray-800 lg:hover:bg-gray-200 dark:hover:bg-gray-700/50 dark:lg:hover:bg-gray-600 rounded-md ${open && 'bg-gray-200 dark:bg-gray-800'}`}
          >
            <span className='text-gray-500 dark:text-gray-400'>
              <svg viewBox="0 0 1024 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" width="32" height="32">
                <path d="M827.97734375 476.84375c19.41679687 0 35.15625 15.74033203 35.15625 35.15625 0 19.22167969-15.4265625 34.84072266-34.57441406 35.15185547l-0.58183594 0.00439453H196.02265625c-19.41679687 0-35.15625-15.74033203-35.15625-35.15625 0-19.22167969 15.4265625-34.84072266 34.57441406-35.15185547l0.58183594-0.00439453h631.9546875zM827.97734375 264.1484375c19.41679687 0 35.15625 15.74033203 35.15625 35.15625 0 19.22167969-15.4265625 34.84072266-34.57441406 35.15185547l-0.58183594 0.00439453H196.02265625c-19.41679687 0-35.15625-15.74033203-35.15625-35.15625 0-19.22167969 15.4265625-34.84072266 34.57441406-35.15185547l0.58183594-0.00439453h631.9546875zM827.97734375 689.5390625c19.41679687 0 35.15625 15.74033203 35.15625 35.15625 0 19.22167969-15.4265625 34.84072266-34.57441406 35.15185547l-0.58183594 0.00439453H196.02265625c-19.41679687 0-35.15625-15.74033203-35.15625-35.15625 0-19.22167969 15.4265625-34.84072266 34.57441406-35.15185547l0.58183594-0.00439453h631.9546875z" fill="currentColor"></path>
              </svg>
            </span>

          </MenuButton>
          <Transition
            show={open}
            enter="transition ease-out duration-100"
            enterFrom="transform opacity-0 scale-95"
            enterTo="transform opacity-100 scale-100"
            leave="transition ease-in duration-75"
            leaveFrom="transform opacity-100 scale-100"
            leaveTo="transform opacity-0 scale-95"
          >
            <MenuItems anchor='bottom end' static className="origin-top-right z-10 absolute top-full min-w-[11rem] bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 py-1.5 rounded-lg shadow-lg overflow-hidden mt-1 left-0">
              {options.map((option) => (
                <MenuItem key={option.label}>
                  <div className="block px-4 py-2 text-sm text-gray-800 dark:text-gray-100 hover:bg-gray-100 dark:hover:bg-gray-700/60" onClick={option.action}>
                    {option.label}
                  </div>
                </MenuItem>
              ))}
            </MenuItems>
          </Transition>
        </>
      )}
    </Menu>
  )
}

export default DropdownHelp