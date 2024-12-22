export const metadata = {
  title: 'Roadmap - Mosaic',
  description: 'Page description',
}

import Image from 'next/image'
import User01 from '@/public/images/user-28-01.jpg'
import User02 from '@/public/images/user-28-02.jpg'
import User03 from '@/public/images/user-28-03.jpg'
import User05 from '@/public/images/user-28-05.jpg'
import User06 from '@/public/images/user-28-06.jpg'
import User08 from '@/public/images/user-28-08.jpg'
import User09 from '@/public/images/user-28-09.jpg'

export default function Roadmap() {
  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">

      {/* Page header */}
      <div className="sm:flex sm:justify-between sm:items-center px-4 sm:px-6 py-8 border-b border-gray-200 dark:border-gray-700/60">

        {/* Left: Title */}
        <div className="mb-4 sm:mb-0">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Roadmap</h1>
        </div>

        {/* Right: Actions */}
        <div className="grid grid-flow-col sm:auto-cols-max justify-start sm:justify-end gap-2">

          {/* Add event button */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">Add Event</button>

        </div>

      </div>

      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">
        <div className="max-w-3xl m-auto">

          {/* Posts */}
          <div className="xl:-translate-x-16">

            {/* Post */}
            <article className="pt-6">
              <div className="xl:flex">
                <div className="w-32 shrink-0">
                  <h2 className="text-xl leading-snug font-bold text-gray-800 dark:text-gray-100 xl:leading-7 mb-4 xl:mb-0">2024 Q1</h2>
                </div>
                <div className="grow pb-6 border-b border-gray-200 dark:border-gray-700/60">
                  <header>
                    <div className="flex flex-nowrap items-center space-x-2 mb-6">
                      {/* Avatars */}
                      <div className="flex shrink-0 -space-x-3 -ml-px">
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User05} width={28} height={28} alt="User 05" />
                        </a>
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User08} width={28} height={28} alt="User 08" />
                        </a>
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User09} width={28} height={28} alt="User 09" />
                        </a>
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User06} width={28} height={28} alt="User 06" />
                        </a>
                      </div>
                      <div className="text-gray-400 dark:text-gray-600">·</div>
                      <div>
                        <div className="text-xs inline-flex font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2.5 py-1">Completed</div>
                      </div>
                    </div>
                  </header>
                  {/* List */}
                  <ul className="-my-2">
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 h-full w-0.5 bg-gray-200 dark:bg-gray-700 self-start ml-2.5 -translate-x-1/2 translate-y-3" aria-hidden="true"></div>
                        <div className="absolute left-0 rounded-full bg-violet-500" aria-hidden="true">
                          <svg className="fill-current text-white" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M14.4 8.4L13 7l-4 4-2-2-1.4 1.4L9 13.8z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Product V1 - Early Access</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 h-full w-0.5 bg-gray-200 dark:bg-gray-700 self-start ml-2.5 -translate-x-1/2 translate-y-3" aria-hidden="true"></div>
                        <div className="absolute left-0 rounded-full bg-violet-500" aria-hidden="true">
                          <svg className="fill-current text-white" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M14.4 8.4L13 7l-4 4-2-2-1.4 1.4L9 13.8z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Web3 Compatibility</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 h-full w-0.5 bg-gray-200 dark:bg-gray-700 self-start ml-2.5 -translate-x-1/2 translate-y-3" aria-hidden="true"></div>
                        <div className="absolute left-0 rounded-full bg-violet-500" aria-hidden="true">
                          <svg className="fill-current text-white" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M14.4 8.4L13 7l-4 4-2-2-1.4 1.4L9 13.8z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Proof-of-Funds Compliance Key</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 rounded-full bg-violet-500" aria-hidden="true">
                          <svg className="fill-current text-white" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M14.4 8.4L13 7l-4 4-2-2-1.4 1.4L9 13.8z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Activity Feed Integration</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                  </ul>
                </div>
              </div>
            </article>

            {/* Post */}
            <article className="pt-6">
              <div className="xl:flex">
                <div className="w-32 shrink-0">
                  <h2 className="text-xl leading-snug font-bold text-gray-800 dark:text-gray-100 xl:leading-7 mb-4 xl:mb-0">2024 Q2</h2>
                </div>
                <div className="grow pb-6 border-b border-gray-200 dark:border-gray-700/60">
                  <header>
                    <div className="flex flex-nowrap items-center space-x-2 mb-6">
                      {/* Avatars */}
                      <div className="flex shrink-0 -space-x-3 -ml-px">
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User03} width={28} height={28} alt="User 03" />
                        </a>
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User05} width={28} height={28} alt="User 05" />
                        </a>
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User01} width={28} height={28} alt="User 01" />
                        </a>
                      </div>
                      <div className="text-gray-400 dark:text-gray-600">·</div>
                      <div>
                        <div className="btn-xs text-xs bg-violet-500/20 text-violet-600 px-2.5 py-1 rounded-full shadow-none">Working on</div>
                      </div>
                    </div>
                  </header>
                  {/* List */}
                  <ul className="-my-2">
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 h-full w-0.5 bg-gray-200 dark:bg-gray-700 self-start ml-2.5 -translate-x-1/2 translate-y-3" aria-hidden="true"></div>
                        <div className="absolute left-0 rounded-full bg-violet-500" aria-hidden="true">
                          <svg className="fill-current text-white" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M14.4 8.4L13 7l-4 4-2-2-1.4 1.4L9 13.8z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Product V1 - Early Access</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 h-full w-0.5 bg-gray-200 dark:bg-gray-700 self-start ml-2.5 -translate-x-1/2 translate-y-3" aria-hidden="true"></div>
                        <div className="absolute left-0 rounded-full bg-violet-500" aria-hidden="true">
                          <svg className="fill-current text-white" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M14.4 8.4L13 7l-4 4-2-2-1.4 1.4L9 13.8z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Web3 Compatibility</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 h-full w-0.5 bg-gray-200 dark:bg-gray-700 self-start ml-2.5 -translate-x-1/2 translate-y-3" aria-hidden="true"></div>
                        <div className="absolute left-0 rounded-full bg-violet-500" aria-hidden="true">
                          <svg className="fill-current text-white" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M14.4 8.4L13 7l-4 4-2-2-1.4 1.4L9 13.8z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Proof-of-Funds Compliance Key</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 rounded-full bg-white dark:bg-gray-900" aria-hidden="true">
                          <svg className="fill-current text-gray-200 dark:text-gray-700" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M10 18a8 8 0 100-16 8 8 0 000 16zm0 2C4.477 20 0 15.523 0 10S4.477 0 10 0s10 4.477 10 10-4.477 10-10 10z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Activity Feed Integration</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                  </ul>
                </div>
              </div>
            </article>

            {/* Post */}
            <article className="pt-6">
              <div className="xl:flex">
                <div className="w-32 shrink-0">
                  <h2 className="text-xl leading-snug font-bold text-gray-800 dark:text-gray-100 xl:leading-7 mb-4 xl:mb-0">2024 Q3</h2>
                </div>
                <div className="grow pb-6 border-b border-gray-200 dark:border-gray-700/60">
                  <header>
                    <div className="flex flex-nowrap items-center space-x-2 mb-6">
                      {/* Avatars */}
                      <div className="flex shrink-0 -space-x-3 -ml-px">
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User06} width={28} height={28} alt="User 03" />
                        </a>
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User05} width={28} height={28} alt="User 05" />
                        </a>
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User09} width={28} height={28} alt="User 01" />
                        </a>
                      </div>
                      <div className="text-gray-400 dark:text-gray-600">·</div>
                      <div>
                        <div className="text-xs inline-flex font-medium bg-yellow-500/20 text-yellow-700 rounded-full text-center px-2.5 py-1">Planned</div>
                      </div>
                    </div>
                  </header>
                  {/* List */}
                  <ul className="-my-2">
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 h-full w-0.5 bg-gray-200 dark:bg-gray-700 self-start ml-2.5 -translate-x-1/2 translate-y-3" aria-hidden="true"></div>
                        <div className="absolute left-0 rounded-full bg-white dark:bg-gray-900" aria-hidden="true">
                          <svg className="fill-current text-gray-200 dark:text-gray-700" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M10 18a8 8 0 100-16 8 8 0 000 16zm0 2C4.477 20 0 15.523 0 10S4.477 0 10 0s10 4.477 10 10-4.477 10-10 10z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Product V1 - Early Access</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 rounded-full bg-white dark:bg-gray-900" aria-hidden="true">
                          <svg className="fill-current text-gray-200 dark:text-gray-700" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M10 18a8 8 0 100-16 8 8 0 000 16zm0 2C4.477 20 0 15.523 0 10S4.477 0 10 0s10 4.477 10 10-4.477 10-10 10z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Web3 Compatibility</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                  </ul>
                </div>
              </div>
            </article>

            {/* Post */}
            <article className="pt-6">
              <div className="xl:flex">
                <div className="w-32 shrink-0">
                  <h2 className="text-xl leading-snug font-bold text-gray-800 dark:text-gray-100 xl:leading-7 mb-4 xl:mb-0">2024 Q4</h2>
                </div>
                <div className="grow pb-6 border-b border-gray-200 dark:border-gray-700/60">
                  <header>
                    <div className="flex flex-nowrap items-center space-x-2 mb-6">
                      {/* Avatars */}
                      <div className="flex shrink-0 -space-x-3 -ml-px">
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User06} width={28} height={28} alt="User 03" />
                        </a>
                        <a className="block" href="#0">
                          <Image className="rounded-full border-2 border-white dark:border-gray-800 box-content" src={User02} width={28} height={28} alt="User 02" />
                        </a>
                      </div>
                      <div className="text-gray-400 dark:text-gray-600">·</div>
                      <div>
                        <div className="text-xs inline-flex font-medium bg-yellow-500/20 text-yellow-700 rounded-full text-center px-2.5 py-1">Planned</div>
                      </div>
                    </div>
                  </header>
                  {/* List */}
                  <ul className="-my-2">
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 h-full w-0.5 bg-gray-200 dark:bg-gray-700 self-start ml-2.5 -translate-x-1/2 translate-y-3" aria-hidden="true"></div>
                        <div className="absolute left-0 rounded-full bg-white dark:bg-gray-900" aria-hidden="true">
                          <svg className="fill-current text-gray-200 dark:text-gray-700" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M10 18a8 8 0 100-16 8 8 0 000 16zm0 2C4.477 20 0 15.523 0 10S4.477 0 10 0s10 4.477 10 10-4.477 10-10 10z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Product V1 - Early Access</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                    {/* List item */}
                    <li className="relative py-2">
                      <div className="flex items-center mb-1">
                        <div className="absolute left-0 rounded-full bg-white dark:bg-gray-900" aria-hidden="true">
                          <svg className="fill-current text-gray-200 dark:text-gray-700" width="20" height="20" viewBox="0 0 20 20">
                            <path d="M10 18a8 8 0 100-16 8 8 0 000 16zm0 2C4.477 20 0 15.523 0 10S4.477 0 10 0s10 4.477 10 10-4.477 10-10 10z" />
                          </svg>
                        </div>
                        <h3 className="text-lg font-bold text-gray-800 dark:text-gray-100 pl-9">Web3 Compatibility</h3>
                      </div>
                      <div className="pl-9">Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</div>
                    </li>
                  </ul>
                </div>
              </div>
            </article>
          </div>

        </div>
      </div>
    </div>
  )
}