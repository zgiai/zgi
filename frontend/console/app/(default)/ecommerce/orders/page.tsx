export const metadata = {
  title: 'Orders - Mosaic',
  description: 'Page description',
}

import { SelectedItemsProvider } from '@/app/selected-items-context'
import DeleteButton from '@/components/delete-button'
import DateSelect from '@/components/date-select'
import FilterButton from '@/components/dropdown-filter'
import OrdersTable from './orders-table'
import PaginationClassic from '@/components/pagination-classic'

import Image01 from '@/public/images/icon-01.svg'
import Image02 from '@/public/images/icon-02.svg'
import Image03 from '@/public/images/icon-03.svg'

function OrdersContent() {

  // Some dummy orders data
  const orders = [
    {
      id: 0,
      image: Image01,
      order: '#123567',
      date: '22/01/2024',
      customer: 'Patricia Semklo',
      total: '$129.00',
      status: 'Refunded',
      items: '1',
      location: '🇨🇳 Shanghai, CN',
      type: 'Subscription',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 1,
      image: Image01,
      order: '#779912',
      date: '22/01/2024',
      customer: 'Dominik Lamakani',
      total: '$89.00',
      status: 'Approved',
      items: '2',
      location: '🇲🇽 Mexico City, MX',
      type: 'Subscription',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 2,
      image: Image02,
      order: '#889924',
      date: '22/01/2024',
      customer: 'Ivan Mesaros',
      total: '$89.00',
      status: 'Approved',
      items: '2',
      location: '🇮🇹 Milan, IT',
      type: 'One-time',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 3,
      image: Image01,
      order: '#897726',
      date: '22/01/2024',
      customer: 'Maria Martinez',
      total: '$59.00',
      status: 'Pending',
      items: '1',
      location: '🇮🇹 Bologna, IT',
      type: 'One-time',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 4,
      image: Image03,
      order: '#123567',
      date: '22/01/2024',
      customer: 'Vicky Jung',
      total: '$39.00',
      status: 'Refunded',
      items: '1',
      location: '🇬🇧 London, UK',
      type: 'Subscription',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 5,
      image: Image01,
      order: '#896644',
      date: '21/01/2024',
      customer: 'Tisho Yanchev',
      total: '$59.00',
      status: 'Approved',
      items: '1',
      location: '🇫🇷 Paris, FR',
      type: 'One-time',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 6,
      image: Image03,
      order: '#136988',
      date: '21/01/2024',
      customer: 'James Cameron',
      total: '$89.00',
      status: 'Approved',
      items: '1',
      location: '🇫🇷 Marseille, FR',
      type: 'Subscription',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 7,
      image: Image03,
      order: '#442206',
      date: '21/01/2024',
      customer: 'Haruki Masuno',
      total: '$129.00',
      status: 'Approved',
      items: '2',
      location: '🇺🇸 New York, USA',
      type: 'Subscription',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 8,
      image: Image02,
      order: '#764321',
      date: '21/01/2024',
      customer: 'Joe Huang',
      total: '$89.00',
      status: 'Pending',
      items: '2',
      location: '🇨🇳 Shanghai, CN',
      type: 'One-time',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
    {
      id: 9,
      image: Image01,
      order: '#908764',
      date: '21/01/2024',
      customer: 'Carolyn McNeail',
      total: '$59.00',
      status: 'Refunded',
      items: '1',
      location: '🇬🇧 Sheffield, UK',
      type: 'Subscription',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    }
  ]

  return (
    <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">
      {/* Page header */}
      <div className="sm:flex sm:justify-between sm:items-center mb-8">

        {/* Left: Title */}
        <div className="mb-4 sm:mb-0">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Orders</h1>
        </div>

        {/* Right: Actions */}
        <div className="grid grid-flow-col sm:auto-cols-max justify-start sm:justify-end gap-2">

          {/* Delete button */}
          <DeleteButton />

          {/* Dropdown */}
          <DateSelect />

          {/* Filter button */}
          <FilterButton align="right" />

          {/* Add order button */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">
            <svg className="fill-current shrink-0 xs:hidden" width="16" height="16" viewBox="0 0 16 16">
              <path d="M15 7H9V1c0-.6-.4-1-1-1S7 .4 7 1v6H1c-.6 0-1 .4-1 1s.4 1 1 1h6v6c0 .6.4 1 1 1s1-.4 1-1V9h6c.6 0 1-.4 1-1s-.4-1-1-1z" />
            </svg>
            <span className="max-xs:sr-only">Add Order</span>
          </button>

        </div>

      </div>

      {/* Table */}
      <OrdersTable orders={orders} />

      {/* Pagination */}
      <div className="mt-8">
        <PaginationClassic />
      </div>    
    </div>
  )
}

export default function Orders() {
  return (
    <SelectedItemsProvider>
      <OrdersContent />
    </SelectedItemsProvider>
  )
}