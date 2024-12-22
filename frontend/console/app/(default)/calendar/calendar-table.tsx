'use client'

import { useEffect } from 'react'
import { CalendarProperties } from './calendar-properties'

export interface Event {
  eventStart: Date
  eventEnd: Date | null
  eventName: string
  eventColor: string
}

export default function CalendarTable({ events }: { events: Event[] }) {

  const {
    dayNames,
    currentYear,
    currentMonth,
    daysInMonth,
    startingBlankDays,
    endingBlankDays,
    eventColor,
    isToday,
    renderDays,
  } = CalendarProperties()  

  const getEvents = (date: number) => {
    return events.filter(e => new Date(e.eventStart).toDateString() === new Date(currentYear, currentMonth, date).toDateString())
  }  

  useEffect(() => {
    renderDays()
  }, [])

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow overflow-hidden">

      {/* Days of the week */}
      <div className="grid grid-cols-7 gap-px border-b border-gray-200 dark:border-gray-700/60">
        {
          dayNames.map(day => {
            return (
              <div className="px-1 py-3" key={day}>
                <div className="text-gray-500 text-sm font-medium text-center lg:hidden">{day.substring(0, 3)}</div>
                <div className="text-gray-500 dark:text-gray-400 text-sm font-medium text-center hidden lg:block">{day}</div>
              </div>
            )
          })
        }
      </div>

      {/* Day cells */}
      <div className="grid grid-cols-7 gap-px bg-gray-200 dark:bg-gray-700/60">
        {/* Diagonal stripes pattern */}
        <svg className="sr-only">
          <defs>
            <pattern id="stripes" patternUnits="userSpaceOnUse" width="5" height="5" patternTransform="rotate(135)">
              <line className="stroke-current text-gray-200 dark:text-gray-700 opacity-50" x1="0" y="0" x2="0" y2="5" strokeWidth="2" />
            </pattern>
          </defs>
        </svg>
        {/* Empty cells (previous month) */}
        {
          startingBlankDays.map(blankday => {
            return (
              <div className="bg-gray-50 dark:bg-gray-800 h-20 sm:h-28 lg:h-36" key={blankday}>
                <svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%">
                  <rect width="100%" height="100%" fill="url(#stripes)" />
                </svg>
              </div>
            )
          })
        }
        {/* Days of the current month */}
        {
          daysInMonth.map(day => {
            return (
              <div className="relative bg-white dark:bg-gray-800 h-20 sm:h-28 lg:h-36 overflow-hidden" key={day}>
                <div className="h-full flex flex-col justify-between">
                  {/* Events */}
                  <div className="grow flex flex-col relative p-0.5 sm:p-1.5 overflow-hidden">
                    {
                      getEvents(day).map(event => {
                        return (
                          <button className="relative w-full text-left mb-1" key={event.eventName}>
                            <div className={`px-2 py-0.5 rounded-lg overflow-hidden ${eventColor(event.eventColor)}`}>
                              {/* Event name */}
                              <div className="text-xs font-semibold truncate">{event.eventName}</div>
                              {/* Event time */}
                              <div className="text-xs uppercase truncate hidden sm:block">
                                {/* Start date */}
                                {event.eventStart &&
                                  <span>{event.eventStart.toLocaleTimeString([], { hour12: true, hour: 'numeric', minute: 'numeric' })}</span>
                                }
                                {/* End date */}
                                {event.eventEnd &&
                                  <span>
                                    - <span>{event.eventEnd.toLocaleTimeString([], { hour12: true, hour: 'numeric', minute: 'numeric' })}</span>
                                  </span>
                                }
                              </div>
                            </div>
                          </button>
                        )
                      })
                    }
                    <div className="absolute bottom-0 left-0 right-0 h-4 bg-gradient-to-t from-white dark:from-gray-800 to-transparent pointer-events-none" aria-hidden="true"></div>
                  </div>
                  {/* Cell footer */}
                  <div className="flex justify-between items-center p-0.5 sm:p-1.5">
                    {/* More button (if more than 2 events) */}
                    {getEvents(day).length > 2 &&
                      <button className="text-xs text-gray-500 dark:text-gray-300 font-medium whitespace-nowrap text-center sm:py-0.5 px-0.5 sm:px-2 border border-gray-200 dark:border-gray-700/60 rounded-lg">
                        <span className="md:hidden">+</span><span>{getEvents(day).length - 2}</span> <span className="hidden md:inline">more</span>
                      </button>
                    }
                    {/* Day number */}
                    <button className={`inline-flex ml-auto w-6 h-6 items-center justify-center text-xs sm:text-sm dark:text-gray-300 font-medium text-center rounded-full hover:bg-violet-100 dark:hover:bg-gray-600 ${isToday(day) && 'text-violet-500'}`}>{day}</button>
                  </div>
                </div>
              </div>
            )
          })
        }
        {/* Empty cells (next month) */}
        {
          endingBlankDays.map(blankday => {
            return (
              <div className="bg-gray-50 dark:bg-gray-800 h-20 sm:h-28 lg:h-36" key={blankday}>
                <svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%">
                  <rect width="100%" height="100%" fill="url(#stripes)" />
                </svg>
              </div>
            )
          })
        }
      </div>
    </div>
  )
}