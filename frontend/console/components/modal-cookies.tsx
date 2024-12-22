import { Dialog, DialogPanel, Transition, TransitionChild } from '@headlessui/react'

interface ModalCookiesProps {
  children: React.ReactNode
  title: string
  isOpen: boolean
  setIsOpen: (value: boolean) => void
}

export default function ModalCookies({
  children,
  title,
  isOpen,
  setIsOpen
}: ModalCookiesProps) {
  return (
    <Transition appear show={isOpen}>
      <Dialog as="div" onClose={() => setIsOpen(false)}>
        <TransitionChild
          as="div"
          className="fixed inset-0 bg-gray-900 bg-opacity-30 z-50 transition-opacity"
          enter="transition ease-out duration-200"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="transition ease-out duration-100"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
          aria-hidden="true"
        />
        <TransitionChild
          as="div"
          className="fixed inset-0 z-50 overflow-hidden flex items-center my-4 justify-center px-4 sm:px-6"
          enter="transition ease-in-out duration-200"
          enterFrom="opacity-0 translate-y-4"
          enterTo="opacity-100 translate-y-0"
          leave="transition ease-in-out duration-200"
          leaveFrom="opacity-100 translate-y-0"
          leaveTo="opacity-0 translate-y-4"
        >
          <DialogPanel className="bg-white dark:bg-gray-800 rounded-lg shadow-lg overflow-auto max-w-lg w-full max-h-full">
            <div className="p-5">
              {/* Modal header */}
              <div className="mb-2">
                <div className="flex justify-between items-center">
                  <Dialog.Title className="text-lg font-semibold text-gray-800 dark:text-gray-100">{title}</Dialog.Title>
                  <button className="text-gray-400 dark:text-gray-500 hover:text-gray-500 dark:hover:text-gray-400" onClick={(e) => { e.stopPropagation(); setIsOpen(false); }}>
                    <div className="sr-only">Close</div>
                    <svg className="fill-current" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M7.95 6.536l4.242-4.243a1 1 0 111.415 1.414L9.364 7.95l4.243 4.242a1 1 0 11-1.415 1.415L7.95 9.364l-4.243 4.243a1 1 0 01-1.414-1.415L6.536 7.95 2.293 3.707a1 1 0 011.414-1.414L7.95 6.536z" />
                    </svg>
                  </button>
                </div>
              </div>
              {children}
            </div>
          </DialogPanel>
        </TransitionChild>
      </Dialog>
    </Transition>
  )
}
