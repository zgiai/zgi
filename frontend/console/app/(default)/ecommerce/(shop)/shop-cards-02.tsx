import Image from 'next/image'
import AppImage05 from '@/public/images/applications-image-05.jpg'
import AppImage06 from '@/public/images/applications-image-06.jpg'
import AppImage07 from '@/public/images/applications-image-07.jpg'
import AppImage08 from '@/public/images/applications-image-08.jpg'

export default function ShopCards02() {
  return (
    <>
      {/* Card 1 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage05} width={286} height={160} alt="Application 05" />
            {/* Popular label */}
            <div className="absolute top-0 right-0 mt-4 mr-4">
              <div className="inline-flex items-center text-xs font-medium text-gray-100 dark:text-gray-300 bg-gray-900/60 dark:bg-gray-800/60 rounded-full text-center px-2 py-0.5">
                <svg className="w-3 h-3 shrink-0 fill-current text-yellow-500 mr-1" viewBox="0 0 12 12">
                  <path d="M11.953 4.29a.5.5 0 00-.454-.292H6.14L6.984.62A.5.5 0 006.12.173l-6 7a.5.5 0 00.379.825h5.359l-.844 3.38a.5.5 0 00.864.445l6-7a.5.5 0 00.075-.534z" />
                </svg>
                <span>Popular</span>
              </div>
            </div>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-2">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                <div className="text-sm">Master Digital Marketing Strategy, Social Media Marketing, Analytics & More!</div>
              </header>
              {/* Rating and Price */}
              <div className="flex flex-wrap items-center justify-between mb-5">
                {/* Rating */}
                <div className="flex items-center space-x-2 mr-2">
                  <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                  </svg>
                  {/* Rate */}
                  <div className="text-sm font-medium whitespace-nowrap">
                    <span>4.7</span> <span className="text-gray-400 dark:text-gray-500">(478)</span>
                  </div>
                </div>
                {/* Price */}
                <div className="flex items-center space-x-2">
                  <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$89.00</div>
                </div>
              </div>
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
          <div className="relative">
            <Image className="w-full" src={AppImage06} width={286} height={160} alt="Application 06" />
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-2">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                <div className="text-sm">Master Digital Marketing Strategy, Social Media Marketing, Analytics & More!</div>
              </header>
              {/* Rating and Price */}
              <div className="flex flex-wrap items-center justify-between mb-5">
                {/* Rating */}
                <div className="flex items-center space-x-2 mr-2">
                  <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                  </svg>
                  {/* Rate */}
                  <div className="text-sm font-medium whitespace-nowrap">
                    <span>4.7</span> <span className="text-gray-400 dark:text-gray-500">(478)</span>
                  </div>
                </div>
                {/* Price */}
                <div className="flex items-center space-x-2">
                  <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$89.00</div>
                </div>
              </div>
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
          <div className="relative">
            <Image className="w-full" src={AppImage07} width={286} height={160} alt="Application 07" />
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-2">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                <div className="text-sm">Master Digital Marketing Strategy, Social Media Marketing, Analytics & More!</div>
              </header>
              {/* Rating and Price */}
              <div className="flex flex-wrap items-center justify-between mb-5">
                {/* Rating */}
                <div className="flex items-center space-x-2 mr-2">
                  <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                  </svg>
                  {/* Rate */}
                  <div className="text-sm font-medium whitespace-nowrap">
                    <span>4.7</span> <span className="text-gray-400 dark:text-gray-500">(478)</span>
                  </div>
                </div>
                {/* Price */}
                <div className="flex items-center space-x-2">
                  <div className="inline-flex text-sm font-medium bg-red-500/20 text-red-700 rounded-full text-center px-2 py-0.5">$39.00</div>
                  <div className="inline-flex text-sm font-medium text-gray-400 dark:text-gray-500 line-through">$199.00</div>
                </div>
              </div>
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
          <div className="relative">
            <Image className="w-full" src={AppImage08} width={286} height={160} alt="Application 08" />
          </div> {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-2">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">Form Builder CP</h3>
                <div className="text-sm">Master Digital Marketing Strategy, Social Media Marketing, Analytics & More!</div>
              </header>
              {/* Rating and Price */}
              <div className="flex flex-wrap items-center justify-between mb-5">
                {/* Rating */}
                <div className="flex items-center space-x-2 mr-2">
                  <svg className="fill-current text-yellow-500" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M10 5.934L8 0 6 5.934H0l4.89 3.954L2.968 16 8 12.223 13.032 16 11.11 9.888 16 5.934z" />
                  </svg>
                  {/* Rate */}
                  <div className="text-sm font-medium whitespace-nowrap">
                    <span>4.7</span> <span className="text-gray-400 dark:text-gray-500">(478)</span>
                  </div>
                </div>
                {/* Price */}
                <div className="flex items-center space-x-2">
                  <div className="inline-flex text-sm font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2 py-0.5">$89.00</div>
                </div>
              </div>
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