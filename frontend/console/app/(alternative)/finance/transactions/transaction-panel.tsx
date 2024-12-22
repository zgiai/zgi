'use client'

import { useEffect, useRef } from 'react'
import { Transition } from '@headlessui/react'
import { useFlyoutContext } from '@/app/flyout-context'
import { useTransactionDetail } from './transaction-context'
import { TransactionsProperties } from './transactions-properties'

import Image from 'next/image'

export default function TransactionPanel() {

  const {
    flyoutOpen,
    setFlyoutOpen,
  } = useFlyoutContext()  
  
  const { transaction } = useTransactionDetail()

  const {
    statusColor,
    amountColor,
  } = TransactionsProperties()

  const panelContent = useRef<HTMLDivElement>(null)
  const closeBtn = useRef<HTMLButtonElement>(null)  

  // close on click outside
  useEffect(() => {
    const clickHandler = ({ target }: { target: EventTarget | null }) => {
      if (!panelContent.current || !closeBtn.current ) return
      if (!flyoutOpen || panelContent.current.contains(target as Node) || closeBtn.current.contains(target as Node)) return      
      setFlyoutOpen(false)
    }
    document.addEventListener('click', clickHandler)
    return () => document.removeEventListener('click', clickHandler)
  })

  // close if the esc key is pressed
  useEffect(() => {
    const keyHandler = ({ keyCode }: { keyCode: number }): void => {
      if (!flyoutOpen || keyCode !== 27) return
      setFlyoutOpen(false)
    }
    document.addEventListener('keydown', keyHandler)
    return () => document.removeEventListener('keydown', keyHandler)
  })

  return (
    <Transition
      show={flyoutOpen}
      unmount={false}
      as="div"
      id="transaction-details"
      ref={panelContent}
      className="absolute inset-0 sm:left-auto z-20 shadow-xl"
      enter="transition-transform duration-200 ease-in-out"
      enterFrom="translate-x-full"
      enterTo="translate-x-0"
      leave="transition-transform duration-200 ease-in-out"
      leaveFrom="translate-x-0"
      leaveTo="translate-x-full"
    >
      <div className="sticky top-16 bg-gradient-to-b from-gray-100 to-white dark:from-[#151D2C] dark:to-gray-900 overflow-x-hidden overflow-y-auto no-scrollbar shrink-0 border-l border-gray-200 dark:border-gray-700/60 w-full sm:w-[390px] h-[calc(100dvh-64px)]">
        <button ref={closeBtn} onClick={() => setFlyoutOpen(false)} className="absolute top-0 right-0 mt-6 mr-6 group p-2">
          <svg
            className="fill-gray-400 group-hover:fill-gray-600 pointer-events-none"
            width="16"
            height="16"
            viewBox="0 0 16 16"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path d="m7.95 6.536 4.242-4.243a1 1 0 1 1 1.415 1.414L9.364 7.95l4.243 4.242a1 1 0 1 1-1.415 1.415L7.95 9.364l-4.243 4.243a1 1 0 0 1-1.414-1.415L6.536 7.95 2.293 3.707a1 1 0 0 1 1.414-1.414L7.95 6.536Z" />
          </svg>
        </button>
        {transaction &&
          <div className="py-8 px-4 lg:px-8">
            <div className="max-w-sm mx-auto lg:max-w-none">
              <div className="text-gray-800 dark:text-gray-100 font-semibold text-center mb-1">Bank Transfer</div>
              <div className="text-sm text-center italic">{transaction.date}, 8:56 PM</div>
              {/* Details */}
              <div className="drop-shadow-md mt-12">
                {/* Top */}
                <div className="bg-white dark:bg-gray-800 rounded-t-xl px-5 pb-2.5 text-center">
                  <div className="mb-3 text-center">
                    <Image className="inline-flex w-12 h-12 rounded-full -mt-6" src={transaction.image} width={48} height={48} alt="Transaction 04" />
                  </div>
                  <div className={`text-2xl font-semibold mb-1 ${amountColor(transaction.amount)}`}>{transaction?.amount}</div>
                  <div className="text-sm font-medium text-gray-800 dark:text-gray-100 mb-3">{transaction.name}</div>
                  <div className={`text-xs inline-flex font-medium rounded-full text-center px-2.5 py-1 ${statusColor(transaction.status)}`}>{transaction.status}</div>
                </div>
                {/* Divider */}
                <div className="flex justify-between items-center" aria-hidden="true">
                  <svg className="fill-white dark:fill-gray-800" width="20" height="20"xmlns="http://www.w3.org/2000/svg">
                    <path d="M0 20c5.523 0 10-4.477 10-10S5.523 0 0 0h20v20H0Z" />
                  </svg>
                  <div className="grow w-full h-5 bg-white dark:bg-gray-800 flex flex-col justify-center">
                    <div className="h-px w-full border-t border-dashed border-gray-200 dark:border-gray-700" />
                  </div>
                  <svg className="fill-white dark:fill-gray-800 rotate-180" width="20" height="20" xmlns="http://www.w3.org/2000/svg">
                    <path d="M0 20c5.523 0 10-4.477 10-10S5.523 0 0 0h20v20H0Z" />
                  </svg>
                </div>
                {/* Bottom */}
                <div className="bg-white dark:bg-gray-800 rounded-b-xl p-5 pt-2.5 text-sm space-y-3">
                  <div className="flex justify-between space-x-1">
                    <span className="italic">IBAN:</span>
                    <span className="font-medium text-gray-700 dark:text-gray-100 text-right">IT17 2207 1010 0504 0006 88</span>
                  </div>
                  <div className="flex justify-between space-x-1">
                    <span className="italic">BIC:</span>
                    <span className="font-medium text-gray-700 dark:text-gray-100 text-right">BARIT22</span>
                  </div>
                  <div className="flex justify-between space-x-1">
                    <span className="italic">Reference:</span>
                    <span className="font-medium text-gray-700 dark:text-gray-100 text-right">Freelance Work</span>
                  </div>
                  <div className="flex justify-between space-x-1">
                    <span className="italic">Emitter:</span>
                    <span className="font-medium text-gray-700 dark:text-gray-100 text-right">Acme LTD UK</span>
                  </div>
                </div>
              </div>
              {/* Receipts */}
              <div className="mt-6">
                <div className="text-sm font-semibold text-gray-800 dark:text-gray-100 mb-2">Receipts</div>
                <form className="rounded bg-gray-100 dark:bg-gray-700/30 border border-dashed border-gray-300 dark:border-gray-700/60 text-center px-5 py-8">
                  <svg className="inline-flex fill-gray-400 dark:fill-gray-500 mb-3" width="16" height="16" viewBox="0 0 16 16" xmlns="http://www.w3.org/2000/svg">
                    <path d="M8 4c-.3 0-.5.1-.7.3L1.6 10 3 11.4l4-4V16h2V7.4l4 4 1.4-1.4-5.7-5.7C8.5 4.1 8.3 4 8 4ZM1 2h14V0H1v2Z" />
                  </svg>
                  <label htmlFor="upload" className="block text-sm text-gray-500 dark:text-gray-400 italic">
                    We accept PNG, JPEG, and PDF files.
                  </label>
                  <input className="sr-only" id="upload" type="file" />
                </form>
              </div>
              {/* Notes */}
              <div className="mt-6">
                <div className="text-sm font-semibold text-gray-800 dark:text-gray-100 mb-2">Notes</div>
                <form>
                  <label className="sr-only" htmlFor="notes">
                    Write a note
                  </label>
                  <textarea id="notes" className="form-textarea w-full focus:border-gray-300" rows={4} placeholder="Write a note…" defaultValue={''} />
                </form>
              </div>
              {/* Download / Report */}
              <div className="flex items-center space-x-3 mt-6">
                <div className="w-1/2">
                  <button className="btn w-full border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300">
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 rotate-180" width="16" height="16" viewBox="0 0 16 16" xmlns="http://www.w3.org/2000/svg">
                      <path d="M8 4c-.3 0-.5.1-.7.3L1.6 10 3 11.4l4-4V16h2V7.4l4 4 1.4-1.4-5.7-5.7C8.5 4.1 8.3 4 8 4ZM1 2h14V0H1v2Z" />
                    </svg>
                    <span className="ml-2">Download</span>
                  </button>
                </div>
                <div className="w-1/2">
                  <button className="btn w-full border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-red-500">
                    <svg className="fill-current shrink-0" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M7.001 3h2v4h-2V3Zm1 7a1 1 0 1 1 0-2 1 1 0 0 1 0 2ZM15 16a1 1 0 0 1-.6-.2L10.667 13H1a1 1 0 0 1-1-1V1a1 1 0 0 1 1-1h14a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1ZM2 11h9a1 1 0 0 1 .6.2L14 13V2H2v9Z" />
                    </svg>
                    <span className="ml-2">Report</span>
                  </button>
                </div>
              </div>
            </div>
          </div>
        }
      </div>
    </Transition>
  )
}