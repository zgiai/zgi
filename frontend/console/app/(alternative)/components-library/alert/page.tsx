export const metadata = {
  title: 'Alert - Mosaic',
  description: 'Page description',
}

import Banners01 from './banners-01'
import Banners02 from './banners-02'
import Toasts01 from './toasts-01'
import Toasts02 from './toasts-02'
import Toasts03 from './toasts-03'
import Notifications from './notifications'

export default function AlertLibrary() {
  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">
        {/* Page header */}
        <div className="mb-8">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Alert & Banner</h1>
        </div>

        <div>

          {/* Components */}
          <div className="space-y-8 mt-8">

            {/* Banner */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Banner</h2>
              <Banners01 />
            </div>

            {/* Banner 2 */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Banner 2</h2>
              <Banners02 />
            </div>

            {/* Toast */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Toast</h2>
              <Toasts01 />
            </div>

            {/* Toast 2 */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Toast 2</h2>
              <Toasts02 />
            </div>

            {/* Toast 3 */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Toast 3</h2>
              <Toasts03 />
            </div>
            
            {/* Notifications */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Notifications</h2>
              <Notifications />
            </div>                

          </div>

        </div>
      </div>
    </div>
  )
}
