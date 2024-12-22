import Image from 'next/image'
import AppImage01 from '@/public/images/applications-image-01.jpg'
import AppImage02 from '@/public/images/applications-image-02.jpg'
import AppImage03 from '@/public/images/applications-image-03.jpg'
import AppImage04 from '@/public/images/applications-image-04.jpg'

export default function ShopCards01() {
  return (
    <>
      {/* Card 1 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <Image className="w-full" src={AppImage01} width={286} height={160} alt="Application 01" />
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-3">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold">HTML, CSS, JavaScript - Build 6 Creative Projects</h3>
              </header>
              {/* Rating and price */}
              <div className="flex flex-wrap justify-between items-center mb-4">
                {/* Rating */}
                <div className="flex items-center space-x-2 mr-2">
                  {/* Stars */}
                  <div className="flex space-x-1">
                    <button>
                      <span className="sr-only">1 star</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">2 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">3 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">4 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">5 stars</span>
                      <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                  </div>
                  {/* Rate */}
                  <div className="inline-flex text-sm font-medium text-yellow-600">4.2</div>
                </div>
                {/* Price */}
                <div>
                  <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$89.00</div>
                </div>
              </div>
              {/* Features list */}
              <ul className="text-sm space-y-2 mb-5 dark:text-gray-300">
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M7 11.674A1 1 0 0 1 7.739 10H8a1 1 0 0 1 1 1v1.699L14.034 8 9 3.302v1.699a1 1 0 0 1-1 1H2V8.27a1 1 0 1 1-2 0V6a2 2 0 0 1 2-2h5V2.153C7 .84 8.563.16 9.523 1.055l6.268 5.849a1.5 1.5 0 0 1 0 2.193l-6.267 5.849C8.565 15.84 7 15.16 7 13.849v-2.175Zm-1.278.878a.498.498 0 0 1 0 .896L4.2 14.2l-.752 1.521a.5.5 0 0 1-.896 0l-.752-1.52-1.521-.753a.498.498 0 0 1 0-.896l1.521-.752.752-1.52c.167-.342.728-.342.896 0L4.2 11.8l1.521.752ZM3.75 3a1.25 1.25 0 1 1 0-2.5 1.25 1.25 0 0 1 0 2.5Z" />
                  </svg>
                  <div>23 hours on-demand video</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M7 7a1 1 0 1 1 0 2H1a1 1 0 1 1 0-2h6Zm8-5a1 1 0 0 1 0 2H1a1 1 0 1 1 0-2h14ZM5 12a1 1 0 0 1 0 2H1a1 1 0 0 1 0-2h4Z" />
                  </svg>
                  <div>37 articles</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4.17 14H3a3 3 0 0 1-3-3V5a3 3 0 0 1 3-3h1.17A3.001 3.001 0 0 1 7 0h6a3 3 0 0 1 3 3v10a3 3 0 0 1-3 3H7a3.001 3.001 0 0 1-2.83-2ZM4 4H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h1V4Zm10 9V3a1 1 0 0 0-1-1H7a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1Z" />
                  </svg>
                  <div>Access on mobile and TV</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4.798 10.509a3.202 3.202 0 1 1 0-6.404 3.202 3.202 0 0 1 0 6.404Zm0-1.83a1.372 1.372 0 1 0 0-2.745 1.372 1.372 0 0 0 0 2.745Zm6.404-.915a3.202 3.202 0 1 1 0-6.404 3.202 3.202 0 0 1 0 6.404Zm0-1.83a1.372 1.372 0 1 0 0-2.744 1.372 1.372 0 0 0 0 2.744ZM9.335 11.11a.915.915 0 0 1-1.07-1.484 5.033 5.033 0 0 1 7.681 2.41.915.915 0 0 1-1.724.609 3.204 3.204 0 0 0-4.887-1.535Zm-7.558 4.28a.915.915 0 0 1-1.725-.61 5.033 5.033 0 0 1 9.49 0 .915.915 0 0 1-1.724.61 3.204 3.204 0 0 0-6.04 0Z" />
                  </svg>
                  <div>8K+ active installations</div>
                </li>
              </ul>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Buy Now</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 2 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <Image className="w-full" src={AppImage02} width={286} height={160} alt="Application 02" />
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-3">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold">HTML, CSS, JavaScript - Build 6 Creative Projects</h3>
              </header>
              {/* Rating and price */}
              <div className="flex flex-wrap justify-between items-center mb-4">
                {/* Rating */}
                <div className="flex items-center space-x-2 mr-2">
                  {/* Stars */}
                  <div className="flex space-x-1">
                    <button>
                      <span className="sr-only">1 star</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">2 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">3 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">4 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">5 stars</span>
                      <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                  </div>
                  {/* Rate */}
                  <div className="inline-flex text-sm font-medium text-yellow-600">3.9</div>
                </div>
                {/* Price */}
                <div>
                  <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$49.00</div>
                </div>
              </div>
              {/* Features list */}
              <ul className="text-sm space-y-2 mb-5 dark:text-gray-300">
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M7 11.674A1 1 0 0 1 7.739 10H8a1 1 0 0 1 1 1v1.699L14.034 8 9 3.302v1.699a1 1 0 0 1-1 1H2V8.27a1 1 0 1 1-2 0V6a2 2 0 0 1 2-2h5V2.153C7 .84 8.563.16 9.523 1.055l6.268 5.849a1.5 1.5 0 0 1 0 2.193l-6.267 5.849C8.565 15.84 7 15.16 7 13.849v-2.175Zm-1.278.878a.498.498 0 0 1 0 .896L4.2 14.2l-.752 1.521a.5.5 0 0 1-.896 0l-.752-1.52-1.521-.753a.498.498 0 0 1 0-.896l1.521-.752.752-1.52c.167-.342.728-.342.896 0L4.2 11.8l1.521.752ZM3.75 3a1.25 1.25 0 1 1 0-2.5 1.25 1.25 0 0 1 0 2.5Z" />
                  </svg>
                  <div>23 hours on-demand video</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M7 7a1 1 0 1 1 0 2H1a1 1 0 1 1 0-2h6Zm8-5a1 1 0 0 1 0 2H1a1 1 0 1 1 0-2h14ZM5 12a1 1 0 0 1 0 2H1a1 1 0 0 1 0-2h4Z" />
                  </svg>
                  <div>37 articles</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4.17 14H3a3 3 0 0 1-3-3V5a3 3 0 0 1 3-3h1.17A3.001 3.001 0 0 1 7 0h6a3 3 0 0 1 3 3v10a3 3 0 0 1-3 3H7a3.001 3.001 0 0 1-2.83-2ZM4 4H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h1V4Zm10 9V3a1 1 0 0 0-1-1H7a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1Z" />
                  </svg>
                  <div>Access on mobile and TV</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4.798 10.509a3.202 3.202 0 1 1 0-6.404 3.202 3.202 0 0 1 0 6.404Zm0-1.83a1.372 1.372 0 1 0 0-2.745 1.372 1.372 0 0 0 0 2.745Zm6.404-.915a3.202 3.202 0 1 1 0-6.404 3.202 3.202 0 0 1 0 6.404Zm0-1.83a1.372 1.372 0 1 0 0-2.744 1.372 1.372 0 0 0 0 2.744ZM9.335 11.11a.915.915 0 0 1-1.07-1.484 5.033 5.033 0 0 1 7.681 2.41.915.915 0 0 1-1.724.609 3.204 3.204 0 0 0-4.887-1.535Zm-7.558 4.28a.915.915 0 0 1-1.725-.61 5.033 5.033 0 0 1 9.49 0 .915.915 0 0 1-1.724.61 3.204 3.204 0 0 0-6.04 0Z" />
                  </svg>
                  <div>8K+ active installations</div>
                </li>
              </ul>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Buy Now</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 3 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <Image className="w-full" src={AppImage03} width={286} height={160} alt="Application 03" />
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-3">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold">HTML, CSS, JavaScript - Build 6 Creative Projects</h3>
              </header>
              {/* Rating and price */}
              <div className="flex flex-wrap justify-between items-center mb-4">
                {/* Rating */}
                <div className="flex items-center space-x-2 mr-2">
                  {/* Stars */}
                  <div className="flex space-x-1">
                    <button>
                      <span className="sr-only">1 star</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">2 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">3 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">4 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">5 stars</span>
                      <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                  </div>
                  {/* Rate */}
                  <div className="inline-flex text-sm font-medium text-yellow-600">4.6</div>
                </div>
                {/* Price */}
                <div>
                  <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$129.00</div>
                </div>
              </div>
              {/* Features list */}
              <ul className="text-sm space-y-2 mb-5 dark:text-gray-300">
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M7 11.674A1 1 0 0 1 7.739 10H8a1 1 0 0 1 1 1v1.699L14.034 8 9 3.302v1.699a1 1 0 0 1-1 1H2V8.27a1 1 0 1 1-2 0V6a2 2 0 0 1 2-2h5V2.153C7 .84 8.563.16 9.523 1.055l6.268 5.849a1.5 1.5 0 0 1 0 2.193l-6.267 5.849C8.565 15.84 7 15.16 7 13.849v-2.175Zm-1.278.878a.498.498 0 0 1 0 .896L4.2 14.2l-.752 1.521a.5.5 0 0 1-.896 0l-.752-1.52-1.521-.753a.498.498 0 0 1 0-.896l1.521-.752.752-1.52c.167-.342.728-.342.896 0L4.2 11.8l1.521.752ZM3.75 3a1.25 1.25 0 1 1 0-2.5 1.25 1.25 0 0 1 0 2.5Z" />
                  </svg>
                  <div>23 hours on-demand video</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M7 7a1 1 0 1 1 0 2H1a1 1 0 1 1 0-2h6Zm8-5a1 1 0 0 1 0 2H1a1 1 0 1 1 0-2h14ZM5 12a1 1 0 0 1 0 2H1a1 1 0 0 1 0-2h4Z" />
                  </svg>
                  <div>37 articles</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4.17 14H3a3 3 0 0 1-3-3V5a3 3 0 0 1 3-3h1.17A3.001 3.001 0 0 1 7 0h6a3 3 0 0 1 3 3v10a3 3 0 0 1-3 3H7a3.001 3.001 0 0 1-2.83-2ZM4 4H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h1V4Zm10 9V3a1 1 0 0 0-1-1H7a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1Z" />
                  </svg>
                  <div>Access on mobile and TV</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4.798 10.509a3.202 3.202 0 1 1 0-6.404 3.202 3.202 0 0 1 0 6.404Zm0-1.83a1.372 1.372 0 1 0 0-2.745 1.372 1.372 0 0 0 0 2.745Zm6.404-.915a3.202 3.202 0 1 1 0-6.404 3.202 3.202 0 0 1 0 6.404Zm0-1.83a1.372 1.372 0 1 0 0-2.744 1.372 1.372 0 0 0 0 2.744ZM9.335 11.11a.915.915 0 0 1-1.07-1.484 5.033 5.033 0 0 1 7.681 2.41.915.915 0 0 1-1.724.609 3.204 3.204 0 0 0-4.887-1.535Zm-7.558 4.28a.915.915 0 0 1-1.725-.61 5.033 5.033 0 0 1 9.49 0 .915.915 0 0 1-1.724.61 3.204 3.204 0 0 0-6.04 0Z" />
                  </svg>
                  <div>8K+ active installations</div>
                </li>
              </ul>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Buy Now</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 4 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <Image className="w-full" src={AppImage04} width={286} height={160} alt="Application 04" />
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-3">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold">HTML, CSS, JavaScript - Build 6 Creative Projects</h3>
              </header>
              {/* Rating and price */}
              <div className="flex flex-wrap justify-between items-center mb-4">
                {/* Rating */}
                <div className="flex items-center space-x-2 mr-2">
                  {/* Stars */}
                  <div className="flex space-x-1">
                    <button>
                      <span className="sr-only">1 star</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">2 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">3 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">4 stars</span>
                      <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                    <button>
                      <span className="sr-only">5 stars</span>
                      <svg className="fill-current text-gray-300 dark:text-gray-600" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                      </svg>
                    </button>
                  </div>
                  {/* Rate */}
                  <div className="inline-flex text-sm font-medium text-yellow-600">4.7</div>
                </div>
                {/* Price */}
                <div>
                  <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$129.00</div>
                </div>
              </div>
              {/* Features list */}
              <ul className="text-sm space-y-2 mb-5 dark:text-gray-300">
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M7 11.674A1 1 0 0 1 7.739 10H8a1 1 0 0 1 1 1v1.699L14.034 8 9 3.302v1.699a1 1 0 0 1-1 1H2V8.27a1 1 0 1 1-2 0V6a2 2 0 0 1 2-2h5V2.153C7 .84 8.563.16 9.523 1.055l6.268 5.849a1.5 1.5 0 0 1 0 2.193l-6.267 5.849C8.565 15.84 7 15.16 7 13.849v-2.175Zm-1.278.878a.498.498 0 0 1 0 .896L4.2 14.2l-.752 1.521a.5.5 0 0 1-.896 0l-.752-1.52-1.521-.753a.498.498 0 0 1 0-.896l1.521-.752.752-1.52c.167-.342.728-.342.896 0L4.2 11.8l1.521.752ZM3.75 3a1.25 1.25 0 1 1 0-2.5 1.25 1.25 0 0 1 0 2.5Z" />
                  </svg>
                  <div>23 hours on-demand video</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M7 7a1 1 0 1 1 0 2H1a1 1 0 1 1 0-2h6Zm8-5a1 1 0 0 1 0 2H1a1 1 0 1 1 0-2h14ZM5 12a1 1 0 0 1 0 2H1a1 1 0 0 1 0-2h4Z" />
                  </svg>
                  <div>37 articles</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4.17 14H3a3 3 0 0 1-3-3V5a3 3 0 0 1 3-3h1.17A3.001 3.001 0 0 1 7 0h6a3 3 0 0 1 3 3v10a3 3 0 0 1-3 3H7a3.001 3.001 0 0 1-2.83-2ZM4 4H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h1V4Zm10 9V3a1 1 0 0 0-1-1H7a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1Z" />
                  </svg>
                  <div>Access on mobile and TV</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4.798 10.509a3.202 3.202 0 1 1 0-6.404 3.202 3.202 0 0 1 0 6.404Zm0-1.83a1.372 1.372 0 1 0 0-2.745 1.372 1.372 0 0 0 0 2.745Zm6.404-.915a3.202 3.202 0 1 1 0-6.404 3.202 3.202 0 0 1 0 6.404Zm0-1.83a1.372 1.372 0 1 0 0-2.744 1.372 1.372 0 0 0 0 2.744ZM9.335 11.11a.915.915 0 0 1-1.07-1.484 5.033 5.033 0 0 1 7.681 2.41.915.915 0 0 1-1.724.609 3.204 3.204 0 0 0-4.887-1.535Zm-7.558 4.28a.915.915 0 0 1-1.725-.61 5.033 5.033 0 0 1 9.49 0 .915.915 0 0 1-1.724.61 3.204 3.204 0 0 0-6.04 0Z" />
                  </svg>
                  <div>8K+ active installations</div>
                </li>
              </ul>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Buy Now</a>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}