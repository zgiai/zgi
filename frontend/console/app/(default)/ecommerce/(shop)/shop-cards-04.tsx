import Image from 'next/image'
import AppImage13 from '@/public/images/applications-image-13.jpg'
import AppImage14 from '@/public/images/applications-image-14.jpg'
import AppImage15 from '@/public/images/applications-image-15.jpg'
import AppImage16 from '@/public/images/applications-image-16.jpg'

export default function ShopCards04() {
  return (
    <>
      {/* Card 1 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage13} width={286} height={160} alt="Application 13" />
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
              {/* Header */}
              <header className="mb-4">
                <a className="text-xs text-green-500 hover:text-green-600 font-semibold uppercase tracking-widest mb-1" href="#0">Funding</a>
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">SwiftNight Home Plus</h3>
                <div className="text-sm">Lorem ipsum is placeholder text commonly used.</div>
              </header>
              {/* Info */}
              <div className="mb-5">
                <div className="flex justify-between text-sm space-x-2 mb-2">
                  <div><span className="font-semibold text-gray-800 dark:text-gray-100">$789,784</span> <span className="italic">raised</span></div>
                  <div className="text-gray-500 dark:text-gray-400">64%</div>
                </div>
                {/* Bar */}
                <div className="relative w-full h-2 rounded-full bg-gray-200 dark:bg-gray-700 mb-2">
                  <div className="absolute left-0 top-0 bottom-0 bg-green-500 rounded-full" style={{width: '55%'}}></div>
                </div>
                <div className="font-medium text-xs text-gray-500 dark:text-gray-400">30 Days Left</div>
              </div>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Back It</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 2 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage14} width={286} height={160} alt="Application 14" />
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
              {/* Header */}
              <header className="mb-4">
                <a className="text-xs text-green-500 hover:text-green-600 font-semibold uppercase tracking-widest mb-1" href="#0">Funding</a>
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">SwiftNight Home Plus</h3>
                <div className="text-sm">Lorem ipsum is placeholder text commonly used.</div>
              </header>
              {/* Info */}
              <div className="mb-5">
                <div className="flex justify-between text-sm space-x-2 mb-2">
                  <div><span className="font-semibold text-gray-800 dark:text-gray-100">$789,784</span> <span className="italic">raised</span></div>
                  <div className="text-gray-500 dark:text-gray-400">64%</div>
                </div>
                {/* Bar */}
                <div className="relative w-full h-2 rounded-full bg-gray-200 dark:bg-gray-700 mb-2">
                  <div className="absolute left-0 top-0 bottom-0 bg-green-500 rounded-full" style={{width: '55%'}}></div>
                </div>
                <div className="font-medium text-xs text-gray-500 dark:text-gray-400">30 Days Left</div>
              </div>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Back It</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 3 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage15} width={286} height={160} alt="Application 15" />
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
              {/* Header */}
              <header className="mb-4">
                <a className="text-xs text-green-500 hover:text-green-600 font-semibold uppercase tracking-widest mb-1" href="#0">Funding</a>
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">SwiftNight Home Plus</h3>
                <div className="text-sm">Lorem ipsum is placeholder text commonly used.</div>
              </header>
              {/* Info */}
              <div className="mb-5">
                <div className="flex justify-between text-sm space-x-2 mb-2">
                  <div><span className="font-semibold text-gray-800 dark:text-gray-100">$789,784</span> <span className="italic">raised</span></div>
                  <div className="text-gray-500 dark:text-gray-400">64%</div>
                </div>
                {/* Bar */}
                <div className="relative w-full h-2 rounded-full bg-gray-200 dark:bg-gray-700 mb-2">
                  <div className="absolute left-0 top-0 bottom-0 bg-green-500 rounded-full" style={{width: '55%'}}></div>
                </div>
                <div className="font-medium text-xs text-gray-500 dark:text-gray-400">30 Days Left</div>
              </div>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Back It</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 4 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage16} width={286} height={160} alt="Application 16" />
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
              {/* Header */}
              <header className="mb-4">
                <a className="text-xs text-green-500 hover:text-green-600 font-semibold uppercase tracking-widest mb-1" href="#0">Funding</a>
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold mb-1">SwiftNight Home Plus</h3>
                <div className="text-sm">Lorem ipsum is placeholder text commonly used.</div>
              </header>
              {/* Info */}
              <div className="mb-5">
                <div className="flex justify-between text-sm space-x-2 mb-2">
                  <div><span className="font-semibold text-gray-800 dark:text-gray-100">$789,784</span> <span className="italic">raised</span></div>
                  <div className="text-gray-500 dark:text-gray-400">64%</div>
                </div>
                {/* Bar */}
                <div className="relative w-full h-2 rounded-full bg-gray-200 dark:bg-gray-700 mb-2">
                  <div className="absolute left-0 top-0 bottom-0 bg-green-500 rounded-full" style={{width: '55%'}}></div>
                </div>
                <div className="font-medium text-xs text-gray-500 dark:text-gray-400">30 Days Left</div>
              </div>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Back It</a>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}