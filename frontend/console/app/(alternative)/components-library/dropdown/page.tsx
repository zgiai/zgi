export const metadata = {
  title: 'Dropdown - Mosaic',
  description: 'Page description',
}

import DateSelect from '@/components/date-select'
import DropdownFull from '@/components/dropdown-full'
import DropdownFilter from '@/components/dropdown-filter'
import DropdownProfile from '@/components/dropdown-profile'
import DropdownSwitch from '@/components/dropdown-switch'
import DropdownNotifications from '@/components/dropdown-notifications'
import DropdownHelp from '@/components/dropdown-help'
import EditMenu from '@/components/edit-menu'
import Datepicker from '@/components/datepicker'

export default function DropdownLibrary() {
  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

        {/* Page header */}
        <div className="mb-8">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Dropdown</h1>
        </div>

        <div>

          {/* Components */}
          <div className="space-y-8 mt-8 mb-80">

            {/* Classic Dropdown */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Classic Dropdown</h2>
              {/* Start */}
              <DateSelect />
              {/* End */}
            </div>

            {/* Full-width Dropdown */}
            <div className="w-80">
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Full-width Dropdown</h2>
              <DropdownFull />
            </div>

            {/* Filter */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Filter</h2>
              <DropdownFilter />
            </div>

            {/* Profile */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Profile</h2>
              <DropdownProfile />
            </div>

            {/* Switch Account */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Switch Account</h2>
              <DropdownSwitch />
            </div>

            {/* Notification */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Notification</h2>
              <DropdownNotifications />
            </div>

            {/* Help Center */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Help Center</h2>
              <DropdownHelp />
            </div>

            {/* Quick Selection */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Quick Selection</h2>
              <EditMenu />
            </div>

            {/* Datepicker (built with flatpickr) */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Datepicker (built with <a className="underline hover:no-underline" href="https://github.com/flatpickr/flatpickr" target="_blank" rel="noreferrer">flatpickr</a>)</h2>
              <Datepicker />
            </div>

          </div>

        </div>

      </div>
    </div>
  )
}
