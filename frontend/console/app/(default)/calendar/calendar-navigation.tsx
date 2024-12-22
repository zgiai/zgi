'use client'

import { useEffect } from 'react'
import { CalendarProperties } from './calendar-properties'

export interface Event {
  eventStart: Date
  eventEnd: Date | null
  eventName: string
  eventColor: string
}

export default function CalendarNavigation() {

  const {
    currentMonth,
    setCurrentMonth,
    renderDays,
  } = CalendarProperties()  

  return (
    <>
      {/* Previous month button */}
      <button
        className="btn px-2.5 bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-500 hover:text-gray-600 dark:text-gray-400 dark:hover:text-gray-300 disabled:border-gray-200 dark:disabled:border-gray-700 disabled:bg-gray-100 dark:disabled:bg-gray-800 disabled:text-gray-400 dark:disabled:text-gray-600 disabled:cursor-not-allowed"
        disabled={currentMonth === 0}
        onClick={() => { setCurrentMonth(currentMonth - 1); renderDays(); }}
      >
        <span className="sr-only">Previous month</span><wbr />
        <svg className="fill-current text-gray-400 dark:text-gray-500" width="16" height="16" viewBox="0 0 16 16">
          <path d="M9.4 13.4l1.4-1.4-4-4 4-4-1.4-1.4L4 8z" />
        </svg>
      </button>

      {/* Next month button */}
      <button
        className="btn px-2.5 bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-500 hover:text-gray-600 dark:text-gray-400 dark:hover:text-gray-300 disabled:border-gray-200 dark:disabled:border-gray-700 disabled:bg-gray-100 dark:disabled:bg-gray-800 disabled:text-gray-400 dark:disabled:text-gray-600 disabled:cursor-not-allowed"
        disabled={currentMonth === 11}
        onClick={() => { setCurrentMonth(currentMonth + 1); renderDays(); }}
      >
        <span className="sr-only">Next month</span><wbr />
        <svg className="fill-current text-gray-400 dark:text-gray-500" width="16" height="16" viewBox="0 0 16 16">
          <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
        </svg>
      </button>    
    </>
  )
}