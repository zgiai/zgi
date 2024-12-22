import Image from 'next/image'
import Image01 from '@/public/images/user-28-01.jpg'
import Image02 from '@/public/images/user-28-02.jpg'
import Image03 from '@/public/images/user-28-03.jpg'
import Image04 from '@/public/images/user-28-04.jpg'
import Image05 from '@/public/images/user-28-05.jpg'
import Image06 from '@/public/images/user-28-06.jpg'
import Image07 from '@/public/images/user-28-07.jpg'
import Image09 from '@/public/images/user-28-09.jpg'
import Image11 from '@/public/images/user-28-11.jpg'

export default function AnalyticsCard11() {
  return(
    <div className="col-span-full bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Top Products</h2>
      </header>
      <div className="p-3">

        {/* Table */}
        <div className="overflow-x-auto">
          <table className="table-auto w-full dark:text-gray-300">
            {/* Table header */}
            <thead className="text-xs uppercase text-gray-400 dark:text-gray-500 bg-gray-50 dark:bg-gray-700 dark:bg-opacity-50 rounded-sm">
              <tr>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Product</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Created by</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Category</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-center">Total impressions</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-center">Top country</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-center">CR</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Value</div>
                </th>
              </tr>
            </thead>
            {/* Table body */}
            <tbody className="text-sm divide-y divide-gray-100 dark:divide-gray-700/60">
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-violet-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M24.446 19.335a2.5 2.5 0 00-3.522 3.194c-.845.63-1.87.97-2.924.971a4.979 4.979 0 01-1.113-.135 4.436 4.436 0 01-1.343 1.682 6.91 6.91 0 006.9-1.165 2.5 2.5 0 002-4.547h.002zM20.431 11.938a2.5 2.5 0 10-.4 2.014 5.027 5.027 0 012.723 3.078c.148-.018.297-.028.446-.03a4.5 4.5 0 011.7.334 7.023 7.023 0 00-4.469-5.396zM14.969 20.25a2.49 2.49 0 00-1.932-1.234A4.624 4.624 0 0113 18.5a4.97 4.97 0 011.348-3.391 4.456 4.456 0 01-.788-2.016A6.989 6.989 0 0011 18.5c.003.391.04.781.11 1.166a2.5 2.5 0 103.859.584z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Form Builder CP</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex shrink-0 -space-x-3 -ml-px">
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image01} width={28} height={28} alt="User 01" />
                    </a>
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image02} width={28} height={28} alt="User 02" />
                    </a>
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image03} width={28} height={28} alt="User 03" />
                    </a>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-2" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M4.3 4.5c1.9-1.9 5.1-1.9 7 0 .7.7 1.2 1.7 1.4 2.7l2-.3c-.2-1.5-.9-2.8-1.9-3.8C10.1.4 5.7.4 2.9 3.1L.7.9 0 7.3l6.4-.7-2.1-2.1zM15.6 8.7l-6.4.7 2.1 2.1c-1.9 1.9-5.1 1.9-7 0-.7-.7-1.2-1.7-1.4-2.7l-2 .3c.2 1.5.9 2.8 1.9 3.8 1.4 1.4 3.1 2 4.9 2 1.8 0 3.6-.7 4.9-2l2.2 2.2.8-6.4z" />
                    </svg>
                    <div>Subscription</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">20,929</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-lg text-center">🇺🇸</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">27.4%</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left font-medium text-green-600">$12,499.77</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-green-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M11 22.012a1 1 0 01-.707-1.707l4.5-4.5a1 1 0 011.414 0l3.293 3.293 4.793-4.793a1 1 0 111.414 1.414l-5.5 5.5a1 1 0 01-1.414 0L15.5 17.926l-3.793 3.793a1 1 0 01-.707.293z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Machine Learning A-Z</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex shrink-0 -space-x-3 -ml-px">
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image07} width={28} height={28} alt="User 07" />
                    </a>
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image04} width={28} height={28} alt="User 04" />
                    </a>
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image11} width={28} height={28} alt="User 11" />
                    </a>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-2" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M4.3 4.5c1.9-1.9 5.1-1.9 7 0 .7.7 1.2 1.7 1.4 2.7l2-.3c-.2-1.5-.9-2.8-1.9-3.8C10.1.4 5.7.4 2.9 3.1L.7.9 0 7.3l6.4-.7-2.1-2.1zM15.6 8.7l-6.4.7 2.1 2.1c-1.9 1.9-5.1 1.9-7 0-.7-.7-1.2-1.7-1.4-2.7l-2 .3c.2 1.5.9 2.8 1.9 3.8 1.4 1.4 3.1 2 4.9 2 1.8 0 3.6-.7 4.9-2l2.2 2.2.8-6.4z" />
                    </svg>
                    <div>Subscription</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">17,944</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-lg text-center">🇬🇧</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">22.6%</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left font-medium text-green-600">$4,227.09</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-violet-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M24.446 19.335a2.5 2.5 0 00-3.522 3.194c-.845.63-1.87.97-2.924.971a4.979 4.979 0 01-1.113-.135 4.436 4.436 0 01-1.343 1.682 6.91 6.91 0 006.9-1.165 2.5 2.5 0 002-4.547h.002zM20.431 11.938a2.5 2.5 0 10-.4 2.014 5.027 5.027 0 012.723 3.078c.148-.018.297-.028.446-.03a4.5 4.5 0 011.7.334 7.023 7.023 0 00-4.469-5.396zM14.969 20.25a2.49 2.49 0 00-1.932-1.234A4.624 4.624 0 0113 18.5a4.97 4.97 0 011.348-3.391 4.456 4.456 0 01-.788-2.016A6.989 6.989 0 0011 18.5c.003.391.04.781.11 1.166a2.5 2.5 0 103.859.584z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">2024 Web Bootcamp</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex shrink-0 -space-x-3 -ml-px">
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image05} width={28} height={28} alt="User 05" />
                    </a>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-2" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M4.3 4.5c1.9-1.9 5.1-1.9 7 0 .7.7 1.2 1.7 1.4 2.7l2-.3c-.2-1.5-.9-2.8-1.9-3.8C10.1.4 5.7.4 2.9 3.1L.7.9 0 7.3l6.4-.7-2.1-2.1zM15.6 8.7l-6.4.7 2.1 2.1c-1.9 1.9-5.1 1.9-7 0-.7-.7-1.2-1.7-1.4-2.7l-2 .3c.2 1.5.9 2.8 1.9 3.8 1.4 1.4 3.1 2 4.9 2 1.8 0 3.6-.7 4.9-2l2.2 2.2.8-6.4z" />
                    </svg>
                    <div>Subscription</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">16,097</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-lg text-center">🇫🇷</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">22.4%</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left font-medium text-green-600">$2,499.77</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-sky-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M18 26a8 8 0 118-8 8.009 8.009 0 01-8 8zm0-14a6 6 0 100 12 6 6 0 000-12z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Digital Marketing Course</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex shrink-0 -space-x-3 -ml-px">
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image06} width={28} height={28} alt="User 06" />
                    </a>
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image11} width={28} height={28} alt="User 11" />
                    </a>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-2" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M4.3 4.5c1.9-1.9 5.1-1.9 7 0 .7.7 1.2 1.7 1.4 2.7l2-.3c-.2-1.5-.9-2.8-1.9-3.8C10.1.4 5.7.4 2.9 3.1L.7.9 0 7.3l6.4-.7-2.1-2.1zM15.6 8.7l-6.4.7 2.1 2.1c-1.9 1.9-5.1 1.9-7 0-.7-.7-1.2-1.7-1.4-2.7l-2 .3c.2 1.5.9 2.8 1.9 3.8 1.4 1.4 3.1 2 4.9 2 1.8 0 3.6-.7 4.9-2l2.2 2.2.8-6.4z" />
                    </svg>
                    <div>Subscription</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">12,996</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-lg text-center">🇮🇹</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">22.1%</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left font-medium text-green-600">$2,224.09</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-red-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <circle cx="18" cy="18" r="3" />
                        <path d="M13.05 24.363l-.707-.707a8 8 0 010-11.312l.707-.707 1.414 1.414-.707.707a6 6 0 000 8.484l.707.707-1.414 1.414zM22.95 24.363l-1.414-1.414.707-.707a6 6 0 000-8.484l-.707-.707 1.414-1.414.707.707a8 8 0 010 11.312l-.707.707z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Form Builder PRO</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex shrink-0 -space-x-3 -ml-px">
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image09} width={28} height={28} alt="User 09" />
                    </a>
                    <a className="block" href="#0">
                      <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={Image01} width={28} height={28} alt="User 01" />
                    </a>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-2" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M4.3 4.5c1.9-1.9 5.1-1.9 7 0 .7.7 1.2 1.7 1.4 2.7l2-.3c-.2-1.5-.9-2.8-1.9-3.8C10.1.4 5.7.4 2.9 3.1L.7.9 0 7.3l6.4-.7-2.1-2.1zM15.6 8.7l-6.4.7 2.1 2.1c-1.9 1.9-5.1 1.9-7 0-.7-.7-1.2-1.7-1.4-2.7l-2 .3c.2 1.5.9 2.8 1.9 3.8 1.4 1.4 3.1 2 4.9 2 1.8 0 3.6-.7 4.9-2l2.2 2.2.8-6.4z" />
                    </svg>
                    <div>Subscription</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">7,097</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-lg text-center">🇩🇪</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-center">17.4%</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left font-medium text-green-600">$1,949.72</div>
                </td>
              </tr>
            </tbody>
          </table>

        </div>
      </div>
    </div>
  )
}
