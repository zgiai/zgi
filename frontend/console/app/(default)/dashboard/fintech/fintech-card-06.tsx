import Image from 'next/image'
import UserImage from '@/public/images/user-36-05.jpg'

export default function FintechCard06() {
  return(
    <div className="col-span-full xl:col-span-6 bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Recent Earnings</h2>
      </header>
      <div className="p-3">
        {/* Table */}
        <div className="overflow-x-auto">
          <table className="table-auto w-full dark:text-gray-300">
            {/* Table header */}
            <thead className="text-xs uppercase text-gray-400 dark:text-gray-500 bg-gray-50 dark:bg-gray-700 dark:bg-opacity-50 rounded-sm">
              <tr>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Counterparty</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Account</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Date</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-right">Amount</div>
                </th>
              </tr>
            </thead>
            {/* Table body */}
            <tbody className="text-sm divide-y divide-gray-100 dark:divide-gray-700/60">
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-sky-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M18 26a8 8 0 1 1 8-8 8.009 8.009 0 0 1-8 8Zm0-14a6 6 0 1 0 0 12 6 6 0 0 0 0-12Z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Acme LTD UK</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>Revolut</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-green-600 text-right">+$1,299.22</div>
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
                    <div className="font-medium text-gray-800 dark:text-gray-100">Web.com</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>Qonto</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-green-600 text-right">+$1,200.88</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-[#24292E]">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M18 10.2c-4.4 0-8 3.6-8 8 0 3.5 2.3 6.5 5.5 7.6.4.1.5-.2.5-.4V24c-2.2.5-2.7-1-2.7-1-.4-.9-.9-1.2-.9-1.2-.7-.5.1-.5.1-.5.8.1 1.2.8 1.2.8.7 1.3 1.9.9 2.3.7.1-.5.3-.9.5-1.1-1.8-.2-3.6-.9-3.6-4 0-.9.3-1.6.8-2.1-.1-.2-.4-1 .1-2.1 0 0 .7-.2 2.2.8.6-.2 1.3-.3 2-.3s1.4.1 2 .3c1.5-1 2.2-.8 2.2-.8.4 1.1.2 1.9.1 2.1.5.6.8 1.3.8 2.1 0 3.1-1.9 3.7-3.7 3.9.3.4.6.9.6 1.6v2.2c0 .2.1.5.6.4 3.2-1.1 5.5-4.1 5.5-7.6-.1-4.4-3.7-8-8.1-8Z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Github Inc.</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>N26</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-green-600 text-right">+$499.99</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3">
                      <Image className="w-9 h-9 rounded-full" src={UserImage} width={36} height={36} alt="User 05" />
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Aprilynne Pills</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>Revolut</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-green-600 text-right">+$2,179.36</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-red-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M18 21a3 3 0 1 1 0-6 3 3 0 0 1 0 6Zm-4.95 3.363-.707-.707a8 8 0 0 1 0-11.312l.707-.707 1.414 1.414-.707.707a6 6 0 0 0 0 8.484l.707.707-1.414 1.414Zm9.9 0-1.414-1.414.707-.707a6 6 0 0 0 0-8.484l-.707-.707 1.414-1.414.707.707a8 8 0 0 1 0 11.312l-.707.707Z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Form Builder PRO</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>Revolut</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-green-600 text-right">+$249.88</div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div className="text-center border-t border-gray-100 dark:border-gray-700/60 px-2">
          <a className="block text-sm font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400 pt-4 pb-1" href="#0">
            View All -&gt;
          </a>
        </div>
      </div>
    </div>
  )
}
