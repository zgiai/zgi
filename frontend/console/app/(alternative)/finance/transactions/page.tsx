export const metadata = {
  title: 'Transactions - Mosaic',
  description: 'Page description',
}

import { SelectedItemsProvider } from '@/app/selected-items-context'
import { FlyoutProvider } from '@/app/flyout-context'
import { TransactionDetailProvider } from './transaction-context'
import DeleteButton from '@/components/delete-button'
import SearchForm from '@/components/search-form'
import TransactionDropdown from './transaction-dropdown'
import OrdersTable from './transactions-table'
import PaginationClassic from '@/components/pagination-classic'
import TransactionPanel from './transaction-panel'

import Image01 from '@/public/images/transactions-image-01.svg'
import Image02 from '@/public/images/transactions-image-02.svg'
import Image03 from '@/public/images/user-36-05.jpg'
import Image04 from '@/public/images/transactions-image-03.svg'
import Image05 from '@/public/images/transactions-image-04.svg'
import Image06 from '@/public/images/transactions-image-05.svg'
import Image07 from '@/public/images/transactions-image-06.svg'
import Image08 from '@/public/images/transactions-image-07.svg'
import Image09 from '@/public/images/transactions-image-08.svg'

function Transactions() {

  // Some dummy transactions data
  const transactions = [
    {
      id: 0,
      image: Image01,
      name: 'Form Builder CP',
      date: '22/01/2024',
      status: 'Pending',
      amount: '-$1,299.22',
    },
    {
      id: 1,
      image: Image02,
      name: 'Imperial Hotel ****',
      date: '22/01/2024',
      status: 'Completed',
      amount: '-$1,029.77',
    },
    {
      id: 2,
      image: Image03,
      name: 'Aprilynne Pills',
      date: '22/01/2024',
      status: 'Pending',
      amount: '+$499.99',
    },
    {
      id: 3,
      image: Image04,
      name: 'Google Limited UK',
      date: '22/01/2024',
      status: 'Completed',
      amount: '-$1,029.77',
    },
    {
      id: 4,
      image: Image05,
      name: 'Acme LTD UK',
      date: '22/01/2024',
      status: 'Pending',
      amount: '+$2,179.36',
    },
    {
      id: 5,
      image: Image04,
      name: 'Google Limited UK',
      date: '22/01/2024',
      status: 'Canceled',
      amount: '-$1,029.77',
    },
    {
      id: 6,
      image: Image06,
      name: 'Uber',
      date: '22/01/2024',
      status: 'Completed',
      amount: '-$272.88',
    },
    {
      id: 7,
      image: Image07,
      name: 'PublicOne Inc.',
      date: '22/01/2024',
      status: 'Completed',
      amount: '-$199.87',
    },
    {
      id: 8,
      image: Image08,
      name: 'Github Inc.',
      date: '22/01/2024',
      status: 'Completed',
      amount: '-$42.87',
    },
    {
      id: 9,
      image: Image09,
      name: 'Form Builder PRO',
      date: '22/01/2024',
      status: 'Completed',
      amount: '-$112.44',
    },
  ]

  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">
        {/* Page header */}
        <div className="sm:flex sm:justify-between sm:items-center mb-4 md:mb-2">

          {/* Left: Title */}
          <div className="mb-4 sm:mb-0">
            <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">$47,347.09</h1>
          </div>

          {/* Right: Actions */}
          <div className="grid grid-flow-col sm:auto-cols-max justify-start sm:justify-end gap-2">

            {/* Delete button */}
            <DeleteButton />

            {/* Search form */}
            <div className="hidden sm:block">
              <SearchForm />
            </div>

            {/* Export button */}
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">Export Transactions</button>

          </div>

        </div>

        <div className="mb-5">
          <span>Transactions from </span>
          <TransactionDropdown />
        </div>

        {/* Filters */}
        <div className="mb-5">
          <ul className="flex flex-wrap -m-1">
            <li className="m-1">
              <button className="inline-flex items-center justify-center text-sm font-medium leading-5 rounded-full px-3 py-1 border border-transparent shadow-sm bg-gray-900 dark:bg-gray-100 text-white dark:text-gray-800 transition">
                View All
              </button>
            </li>
            <li className="m-1">
              <button className="inline-flex items-center justify-center text-sm font-medium leading-5 rounded-full px-3 py-1 border border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm bg-white dark:bg-gray-800 text-gray-500 dark:text-gray-400 transition">
                Completed
              </button>
            </li>
            <li className="m-1">
              <button className="inline-flex items-center justify-center text-sm font-medium leading-5 rounded-full px-3 py-1 border border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm bg-white dark:bg-gray-800 text-gray-500 dark:text-gray-400 transition">
                Pending
              </button>
            </li>
            <li className="m-1">
              <button className="inline-flex items-center justify-center text-sm font-medium leading-5 rounded-full px-3 py-1 border border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm bg-white dark:bg-gray-800 text-gray-500 dark:text-gray-400 transition">
                Canceled
              </button>
            </li>
          </ul>
        </div>        

        {/* Table */}
        <OrdersTable transactions={transactions} />

        {/* Pagination */}
        <div className="mt-8">
          <PaginationClassic />
        </div>    
      </div>

      <TransactionPanel />
    </div>
  )
}

export default function Orders() {
  return (
    <SelectedItemsProvider>
      <FlyoutProvider>
        <TransactionDetailProvider>
          <Transactions />
        </TransactionDetailProvider>
      </FlyoutProvider>
    </SelectedItemsProvider>
  )
}