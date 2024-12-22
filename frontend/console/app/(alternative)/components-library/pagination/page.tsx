export const metadata = {
  title: 'Pagination - Mosaic',
  description: 'Page description',
}

import PaginationNumeric from '@/components/pagination-numeric'
import PaginationClassic from '@/components/pagination-classic'
import PaginationNumeric02 from '@/components/pagination-numeric-2'

export default function PaginationLibrary() {
  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

        {/* Page header */}
        <div className="mb-8">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Pagination</h1>
        </div>

        <div>

          {/* Components */}
          <div className="space-y-8 mt-8">

            {/* Option 1 */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Option 1</h2>
              <div className="px-6 py-8 bg-gray-100 dark:bg-gray-800/50 rounded-lg">
                <PaginationNumeric />
              </div>
            </div>

            {/* Option 2 */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Option 2</h2>
              <div className="px-6 py-8 bg-gray-100 dark:bg-gray-800/50 rounded-lg">
                <PaginationClassic />
              </div>
            </div>

            {/* Option 3 */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Option 3</h2>
              <div className="px-6 py-8 bg-gray-100 dark:bg-gray-800/50 rounded-lg">
                <PaginationNumeric02 />
              </div>
            </div>

          </div>

        </div>

      </div>
    </div>
  )
}
