'use client'

import { useState } from 'react'

export default function NotificationsPanel() {

  const [comments, setComments] = useState<boolean>(true)
  const [messages, setMessages] = useState<boolean>(true)
  const [mentions, setMentions] = useState<boolean>(false)

  return (
    <div className="grow">

      {/* Panel body */}
      <div className="p-6 space-y-6">
        <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-5">My Notifications</h2>

        {/* General */}
        <section>
          <h3 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-1">General</h3>
          <ul>
            <li className="flex justify-between items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div>
                <div className="text-gray-800 dark:text-gray-100 font-semibold">Comments and replies</div>
                <div className="text-sm">Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit.</div>
              </div>
              {/* Right */}
              <div className="flex items-center ml-4">
                <div className="text-sm text-gray-400 dark:text-gray-500 italic mr-2">{comments ? 'On' : 'Off'}</div>
                <div className="form-switch">
                  <input type="checkbox" id="comments" className="sr-only" checked={comments} onChange={() => setComments(!comments)} />
                  <label className="bg-gray-400 dark:bg-gray-700" htmlFor="comments">
                    <span className="bg-white shadow-sm" aria-hidden="true"></span>
                    <span className="sr-only">Enable smart sync</span>
                  </label>
                </div>
              </div>
            </li>
            <li className="flex justify-between items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div>
                <div className="text-gray-800 dark:text-gray-100 font-semibold">Messages</div>
                <div className="text-sm">Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit.</div>
              </div>
              {/* Right */}
              <div className="flex items-center ml-4">
                <div className="text-sm text-gray-400 dark:text-gray-500 italic mr-2">{messages ? 'On' : 'Off'}</div>
                <div className="form-switch">
                  <input type="checkbox" id="messages" className="sr-only" checked={messages} onChange={() => setMessages(!messages)} />
                  <label className="bg-gray-400 dark:bg-gray-700" htmlFor="messages">
                    <span className="bg-white shadow-sm" aria-hidden="true"></span>
                    <span className="sr-only">Enable smart sync</span>
                  </label>
                </div>
              </div>
            </li>
            <li className="flex justify-between items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div>
                <div className="text-gray-800 dark:text-gray-100 font-semibold">Mentions</div>
                <div className="text-sm">Excepteur sint occaecat cupidatat non in culpa qui officia deserunt mollit.</div>
              </div>
              {/* Right */}
              <div className="flex items-center ml-4">
                <div className="text-sm text-gray-400 dark:text-gray-500 italic mr-2">{mentions ? 'On' : 'Off'}</div>
                <div className="form-switch">
                  <input type="checkbox" id="mentions" className="sr-only" checked={mentions} onChange={() => setMentions(!mentions)} />
                  <label className="bg-gray-400 dark:bg-gray-700" htmlFor="mentions">
                    <span className="bg-white shadow-sm" aria-hidden="true"></span>
                    <span className="sr-only">Enable smart sync</span>
                  </label>
                </div>
              </div>
            </li>
          </ul>
        </section>

        {/* Shares */}
        <section>
          <h3 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-1">Shares</h3>
          <ul>
            <li className="flex justify-between items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div>
                <div className="text-gray-800 dark:text-gray-100 font-semibold">Shares of my content</div>
                <div className="text-sm">Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit.</div>
              </div>
              {/* Right */}
              <div className="flex items-center ml-4">
                <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm">Manage</button>
              </div>
            </li>
            <li className="flex justify-between items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div>
                <div className="text-gray-800 dark:text-gray-100 font-semibold">Team invites</div>
                <div className="text-sm">Excepteur sint occaecat cupidatat non in culpa qui officia deserunt mollit.</div>
              </div>
              {/* Right */}
              <div className="flex items-center ml-4">
                <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm">Manage</button>
              </div>
            </li>
            <li className="flex justify-between items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div>
                <div className="text-gray-800 dark:text-gray-100 font-semibold">Smart connection</div>
                <div className="text-sm">Excepteur sint occaecat cupidatat non in culpa qui officia deserunt mollit.</div>
              </div>
              {/* Right */}
              <div className="flex items-center ml-4">
                <div className="text-sm text-gray-400 dark:text-gray-500 italic mr-2 hidden md:block">Active</div>
                <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm text-red-500">Disable</button>
              </div>
            </li>
          </ul>
        </section>
      </div>

      {/* Panel footer */}
      <footer>
        <div className="flex flex-col px-6 py-5 border-t border-gray-200 dark:border-gray-700/60">
          <div className="flex self-end">
            <button className="btn dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300">Cancel</button>
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white ml-3">Save Changes</button>
          </div>
        </div>
      </footer>

    </div>
  )
}