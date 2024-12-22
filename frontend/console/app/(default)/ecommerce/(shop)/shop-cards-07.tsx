import Image from 'next/image'
import AppImage21 from '@/public/images/applications-image-21.jpg'
import AppImage22 from '@/public/images/applications-image-22.jpg'
import AppImage23 from '@/public/images/applications-image-23.jpg'
import AppImage24 from '@/public/images/applications-image-24.jpg'
import AppImage25 from '@/public/images/applications-image-25.jpg'
import AppImage26 from '@/public/images/applications-image-26.jpg'
import AppImage27 from '@/public/images/applications-image-27.jpg'
import AppImage28 from '@/public/images/applications-image-28.jpg'
import AppImage29 from '@/public/images/applications-image-29.jpg'
import AppImage30 from '@/public/images/applications-image-30.jpg'
import AppImage31 from '@/public/images/applications-image-31.jpg'
import AppImage32 from '@/public/images/applications-image-32.jpg'

export default function ShopCards07() {
  return (
    <>
      {/* Card 1 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage21} width={301} height={226} alt="Application 21" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
            {/* Special Offer label */}
            <div className="absolute bottom-0 right-0 mb-4 mr-4">
              <div className="inline-flex items-center text-xs font-medium text-gray-100 dark:text-gray-300 bg-gray-900/60 dark:bg-gray-800/60 rounded-full text-center px-2 py-0.5">
                <svg className="w-3 h-3 shrink-0 fill-current text-yellow-500 mr-1" viewBox="0 0 12 12">
                  <path d="M11.953 4.29a.5.5 0 00-.454-.292H6.14L6.984.62A.5.5 0 006.12.173l-6 7a.5.5 0 00.379.825h5.359l-.844 3.38a.5.5 0 00.864.445l6-7a.5.5 0 00.075-.534z" />
                </svg>
                <span>Special Offer</span>
              </div>
            </div>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-red-500/20 text-red-700 rounded-full text-center px-2 py-0.5">$39.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 2 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage22} width={301} height={226} alt="Application 22" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$69.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 3 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage23} width={301} height={226} alt="Application 23" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
            {/* Special Offer label */}
            <div className="absolute bottom-0 right-0 mb-4 mr-4">
              <div className="inline-flex items-center text-xs font-medium text-gray-100 dark:text-gray-300 bg-gray-900/60 dark:bg-gray-800/60 rounded-full text-center px-2 py-0.5">
                <svg className="w-3 h-3 shrink-0 fill-current text-yellow-500 mr-1" viewBox="0 0 12 12">
                  <path d="M11.953 4.29a.5.5 0 00-.454-.292H6.14L6.984.62A.5.5 0 006.12.173l-6 7a.5.5 0 00.379.825h5.359l-.844 3.38a.5.5 0 00.864.445l6-7a.5.5 0 00.075-.534z" />
                </svg>
                <span>Special Offer</span>
              </div>
            </div>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-red-500/20 text-red-700 rounded-full text-center px-2 py-0.5">$39.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 4 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage24} width={301} height={226} alt="Application 24" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$69.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 5 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage25} width={301} height={226} alt="Application 25" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$69.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 6 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage26} width={301} height={226} alt="Application 26" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
            {/* Special Offer label */}
            <div className="absolute bottom-0 right-0 mb-4 mr-4">
              <div className="inline-flex items-center text-xs font-medium text-gray-100 dark:text-gray-300 bg-gray-900/60 dark:bg-gray-800/60 rounded-full text-center px-2 py-0.5">
                <svg className="w-3 h-3 shrink-0 fill-current text-yellow-500 mr-1" viewBox="0 0 12 12">
                  <path d="M11.953 4.29a.5.5 0 00-.454-.292H6.14L6.984.62A.5.5 0 006.12.173l-6 7a.5.5 0 00.379.825h5.359l-.844 3.38a.5.5 0 00.864.445l6-7a.5.5 0 00.075-.534z" />
                </svg>
                <span>Special Offer</span>
              </div>
            </div>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-red-500/20 text-red-700 rounded-full text-center px-2 py-0.5">$39.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 7 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage27} width={301} height={226} alt="Application 27" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$69.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 8 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage28} width={301} height={226} alt="Application 28" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$69.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 9 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage29} width={301} height={226} alt="Application 29" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$69.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* CTA */}
      <div className="col-span-full md:order-1 xl:order-none">
        <div className="h-full p-6 bg-gray-900 dark:bg-gray-800 rounded-lg text-center xl:text-left xl:flex xl:justify-between xl:items-center">
          <div className="mb-4 xl:mb-0 xl:mr-4">
            <div className="text-xl text-gray-50 font-semibold mb-1">Excepteur sint occaecat cupidatat 🎁</div>
            <div className="m-auto text-sm text-gray-400">Excepteur sint occaecat cupidatat non proidentsunt in culpa qui officia deserunt mollit!</div>
          </div>
          <button className="btn bg-gray-100 text-gray-800 hover:bg-white shrink-0">Reedem Now!</button>
        </div>
      </div>

      {/* Card 10 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage30} width={301} height={226} alt="Application 30" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$69.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 11 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage31} width={301} height={226} alt="Application 31" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-red-500/20 text-red-700 rounded-full text-center px-2 py-0.5">$39.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Card 12 */}
      <div className="col-span-full md:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage32} width={301} height={226} alt="Application 32" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              <header className="mb-2">
                <a href="#0">
                  <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                </a>
                <div className="text-sm">Lorem ipsum dolor sit amet adipiscing elit, sed do eiusmod.</div>
              </header>
            </div>
            {/* Rating and price */}
            <div className="flex flex-wrap justify-between items-center">
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
                <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$69.00</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}