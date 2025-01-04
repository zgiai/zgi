export default function FintechCard05() {
  return(
    <div className="col-span-full xl:col-span-6 bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Recent Expenses</h2>
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
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-violet-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M24.446 19.335a2.5 2.5 0 00-3.522 3.194c-.845.63-1.87.97-2.924.971a4.979 4.979 0 01-1.113-.135 4.436 4.436 0 01-1.343 1.682 6.91 6.91 0 006.9-1.165 2.5 2.5 0 002-4.547h.002zM20.431 11.938a2.5 2.5 0 10-.4 2.014 5.027 5.027 0 012.723 3.078c.148-.018.297-.028.446-.03a4.5 4.5 0 011.7.334 7.023 7.023 0 00-4.469-5.396zM14.969 20.25a2.49 2.49 0 00-1.932-1.234A4.624 4.624 0 0113 18.5a4.97 4.97 0 011.348-3.391 4.456 4.456 0 01-.788-2.016A6.989 6.989 0 0011 18.5c.003.391.04.781.11 1.166a2.5 2.5 0 103.859.584z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Form Builder CP</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>Revolut</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-gray-800 dark:text-gray-100 text-right">-$1,299.22</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-gray-900">
                      <svg className="w-9 h-9 fill-current text-gray-100" viewBox="0 0 36 36">
                        <path d="M12.014 25.752v-4.998c.196.42.495.749.896.987s.877.357 1.428.357a3.08 3.08 0 0 0 1.694-.483c.504-.322.9-.789 1.19-1.4.29-.611.434-1.328.434-2.149s-.145-1.535-.434-2.142c-.29-.607-.686-1.071-1.19-1.393a3.08 3.08 0 0 0-1.694-.483c-.55 0-1.027.119-1.428.357a2.21 2.21 0 0 0-.896.987v-1.246H9.256v11.606h2.758Zm1.414-6.062a1.34 1.34 0 0 1-1.022-.434c-.27-.29-.406-.686-.406-1.19 0-.495.135-.887.406-1.176a1.34 1.34 0 0 1 1.022-.434c.43 0 .777.142 1.043.427.266.285.399.679.399 1.183 0 .513-.133.912-.399 1.197-.266.285-.614.427-1.043.427Zm10.108 2.408c.924 0 1.771-.217 2.541-.651a4.8 4.8 0 0 0 1.827-1.813c.448-.775.672-1.647.672-2.618 0-.97-.222-1.843-.665-2.618a4.75 4.75 0 0 0-1.82-1.813 5.113 5.113 0 0 0-2.555-.651c-.924 0-1.773.217-2.548.651a4.786 4.786 0 0 0-1.834 1.813c-.448.775-.672 1.647-.672 2.618 0 .97.224 1.843.672 2.618a4.786 4.786 0 0 0 1.834 1.813 5.125 5.125 0 0 0 2.548.651Zm0-2.562c-.7 0-1.25-.229-1.652-.686-.401-.457-.602-1.069-.602-1.834 0-.784.2-1.402.602-1.855.401-.453.952-.679 1.652-.679s1.248.226 1.645.679c.397.453.595 1.071.595 1.855 0 .765-.2 1.377-.602 1.834-.401.457-.947.686-1.638.686Z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">PublicOne Inc.</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>Qonto</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-gray-800 dark:text-gray-100 text-right">-$272.88</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-green-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M15 17h-3v-5a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v5h-3v-2.5a.5.5 0 0 0-.5-.5h-5a.5.5 0 0 0-.5.5V17Zm10 2a1 1 0 0 1 1 1v4a1 1 0 0 1-2 0v-1H12v1a1 1 0 0 1-2 0v-4a1 1 0 0 1 1-1h14Z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Imperial Hotel ****</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>Revolut</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-gray-800 dark:text-gray-100 text-right">-$999.44</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-sky-500">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="m26.3 16.1-2.6-.9-1.8-4.6c-.1-.4-.5-.6-.9-.6h-9c-.6 0-1 .4-1 1v12c0 .6.4 1 1 1h3.3c.6 1.2 1.8 2 3.2 2 1.4 0 2.6-.8 3.2-2.1.1 0 .2.1.3.1h4c.6 0 1-.4 1-1v-6c0-.4-.3-.8-.7-.9ZM25 22h-3c-.2-1.7-1.7-3-3.5-3s-3.2 1.3-3.4 3H13V12h7.3l1.7 4.4c.1.3.3.5.6.6l2.4.7V22Z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Uber</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>N26</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-gray-800 dark:text-gray-100 text-right">-$1,029.77</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap">
                  <div className="flex items-center">
                    <div className="shrink-0 rounded-full mr-2 sm:mr-3 bg-[#EA4335]">
                      <svg className="w-9 h-9 fill-current text-white" viewBox="0 0 36 36">
                        <path d="M18 17v2.4h4.1c-.2 1-1.2 3-4 3-2.4 0-4.3-2-4.3-4.4 0-2.4 2-4.4 4.3-4.4 1.4 0 2.3.6 2.8 1.1l1.9-1.8C21.6 11.7 20 11 18.1 11c-3.9 0-7 3.1-7 7s3.1 7 7 7c4 0 6.7-2.8 6.7-6.8 0-.5 0-.8-.1-1.2H18Z" />
                      </svg>
                    </div>
                    <div className="font-medium text-gray-800 dark:text-gray-100">Google Limited UK</div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>N26</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div>22/01/2024</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-medium text-gray-800 dark:text-gray-100 text-right">-$1,921.26</div>
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
