import Image from 'next/image'
import UserImage from '@/public/images/user-64-14.jpg'
import FintechIcon01 from '@/public/images/company-icon-06.svg'
import FintechIcon02 from '@/public/images/company-icon-02.svg'
import FintechIcon03 from '@/public/images/company-icon-03.svg'

export default function FintechIntro() {
  return(
    <div className="flex flex-col col-span-full bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <div className="px-5 py-6">
        <div className="md:flex md:justify-between md:items-center">
          {/* Left side */}
          <div className="flex items-center mb-4 md:mb-0">
            {/* Avatar */}
            <div className="mr-4">
              <Image className="inline-flex rounded-full" src={UserImage} width={64} height={64} alt="User" />
            </div>
            {/* User info */}
            <div>
              <div className="mb-2">
                Hey <strong className="font-medium text-gray-800 dark:text-gray-100">Mary</strong> 👋, this is your current balance:
              </div>
              <div className="text-3xl font-bold text-green-500">$47,347.09</div>
            </div>
          </div>
          {/* Right side */}
          <ul className="shrink-0 flex flex-wrap justify-end md:justify-start -space-x-3 -ml-px">
            <li>
              <a className="block" href="#0">
                <Image className="w-9 h-9 rounded-full" src={FintechIcon01} width={36} height={36} alt="Account 01" />
              </a>
            </li>
            <li>
              <a className="block" href="#0">
                <Image className="w-9 h-9 rounded-full" src={FintechIcon02} width={36} height={36} alt="Account 02" />
              </a>
            </li>
            <li>
              <a className="block" href="#0">
                <Image className="w-9 h-9 rounded-full" src={FintechIcon03} width={36} height={36} alt="Account 03" />
              </a>
            </li>
            <li>
              <button className="flex justify-center items-center w-9 h-9 rounded-full bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-violet-500 shadow-sm transition">
                <span className="sr-only">Add new account</span>
                <svg className="fill-current" width="16" height="16" viewBox="0 0 16 16">
                  <path d="M15 7H9V1c0-.6-.4-1-1-1S7 .4 7 1v6H1c-.6 0-1 .4-1 1s.4 1 1 1h6v6c0 .6.4 1 1 1s1-.4 1-1V9h6c.6 0 1-.4 1-1s-.4-1-1-1z" />
                </svg>
              </button>
            </li>
          </ul>
        </div>
      </div>
    </div>
  )
}
