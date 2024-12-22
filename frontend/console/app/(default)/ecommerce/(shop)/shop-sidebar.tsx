export default function ShopSidebar() {
  return (
    <div>
      <div className="bg-white dark:bg-gray-800 shadow-sm rounded-xl p-5 min-w-[15rem]">
        <div className="grid md:grid-cols-2 xl:grid-cols-1 gap-6">
          {/* Group 1 */}
          <div>
            <div className="text-sm text-gray-800 dark:text-gray-100 font-semibold mb-3">Discover</div>
            <ul className="text-sm font-medium space-y-2">
              <li>
                <a className="text-violet-500" href="#0">View All</a>
              </li>
              <li>
                <a className="text-gray-600 dark:text-gray-300 hover:text-gray-700 dark:hover:text-gray-200" href="#0">Apps / Software</a>
              </li>
              <li>
                <a className="text-gray-600 dark:text-gray-300 hover:text-gray-700 dark:hover:text-gray-200" href="#0">Design / Tech Products</a>
              </li>
              <li>
                <a className="text-gray-600 dark:text-gray-300 hover:text-gray-700 dark:hover:text-gray-200" href="#0">Books & Writing</a>
              </li>
              <li>
                <a className="text-gray-600 dark:text-gray-300 hover:text-gray-700 dark:hover:text-gray-200" href="#0">Education</a>
              </li>
              <li>
                <a className="text-gray-600 dark:text-gray-300 hover:text-gray-700 dark:hover:text-gray-200" href="#0">Drawing / Painting</a>
              </li>
            </ul>
          </div>
          {/* Price Range */}
          <div>
            <div className="text-sm text-gray-800 dark:text-gray-100 font-semibold mb-3">Price Range</div>
            <label className="sr-only">Price</label>
            <select className="form-select w-full">
              <option>Less than $20</option>
              <option>$20 - $40</option>
              <option>$40 - $80</option>
              <option>More than $80</option>
            </select>
          </div>
          {/* Group 3 */}
          <div>
            <div className="text-sm text-gray-800 dark:text-gray-100 font-semibold mb-3">Multi Select</div>
            <ul className="space-y-2">
              <li>
                <label className="flex items-center">
                  <input type="checkbox" className="form-checkbox" />
                  <span className="text-sm text-gray-600 dark:text-gray-300 font-medium ml-2">Apps / Software</span>
                </label>
              </li>
              <li>
                <label className="flex items-center">
                  <input type="checkbox" className="form-checkbox" />
                  <span className="text-sm text-gray-600 dark:text-gray-300 font-medium ml-2">Education</span>
                </label>
              </li>
              <li>
                <label className="flex items-center">
                  <input type="checkbox" className="form-checkbox" />
                  <span className="text-sm text-gray-600 dark:text-gray-300 font-medium ml-2">Books & Writing</span>
                </label>
              </li>
              <li>
                <label className="flex items-center">
                  <input type="checkbox" className="form-checkbox" />
                  <span className="text-sm text-gray-600 dark:text-gray-300 font-medium ml-2">Drawing / Painting</span>
                </label>
              </li>
            </ul>
          </div>
          {/* Group 4 */}
          <div>
            <div className="text-sm text-gray-800 dark:text-gray-100 font-semibold mb-3">Sort By Rating</div>
            <ul className="space-y-2">
              <li>
                {/* Rating button */}
                <button className="flex items-center space-x-2 mr-2">
                  {/* Stars */}
                  <div className="flex space-x-1">
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                  </div>
                  <div className="inline-flex text-sm text-gray-500 dark:text-gray-400 italic"><span className="sr-only">4 Stars</span> And up</div>
                </button>
              </li>
              <li>
                {/* Rating button */}
                <button className="flex items-center space-x-2 mr-2">
                  {/* Stars */}
                  <div className="flex space-x-1">
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                  </div>
                  <div className="inline-flex text-sm text-gray-500 dark:text-gray-400 italic"><span className="sr-only">3 Stars</span> And up</div>
                </button>
              </li>
              <li>
                {/* Rating button */}
                <button className="flex items-center space-x-2 mr-2">
                  {/* Stars */}
                  <div className="flex space-x-1">
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                  </div>
                  <div className="inline-flex text-sm text-gray-500 dark:text-gray-400 italic"><span className="sr-only">2 Stars</span> And up</div>
                </button>
              </li>
              <li>
                {/* Rating button */}
                <button className="flex items-center space-x-2 mr-2">
                  {/* Stars */}
                  <div className="flex space-x-1">
                    <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                    <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                    </svg>
                  </div>
                  <div className="inline-flex text-sm text-gray-500 dark:text-gray-400 italic"><span className="sr-only">1 Stars</span> And up</div>
                </button>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  )
}