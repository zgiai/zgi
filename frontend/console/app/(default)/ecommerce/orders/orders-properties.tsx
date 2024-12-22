'use client'

import { ReactElement, useState } from 'react'

export const OrdersProperties = () => {
  const [descriptionOpen, setDescriptionOpen] = useState<boolean>(false)

  const statusColor = (status: string): string => {
    switch (status) {
      case 'Approved':
        return 'bg-green-500/20 text-green-700'
      case 'Refunded':
        return 'bg-yellow-500/20 text-yellow-700'
      default:
        return 'bg-gray-100 dark:bg-gray-700 text-gray-500 dark:text-gray-400'
    }
  }

  const typeIcon = (type: string): ReactElement => {
    switch (type) {
      case 'Subscription':
        return (
          <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-2" width="16" height="16" viewBox="0 0 16 16">
            <path d="M4.3 4.5c1.9-1.9 5.1-1.9 7 0 .7.7 1.2 1.7 1.4 2.7l2-.3c-.2-1.5-.9-2.8-1.9-3.8C10.1.4 5.7.4 2.9 3.1L.7.9 0 7.3l6.4-.7-2.1-2.1zM15.6 8.7l-6.4.7 2.1 2.1c-1.9 1.9-5.1 1.9-7 0-.7-.7-1.2-1.7-1.4-2.7l-2 .3c.2 1.5.9 2.8 1.9 3.8 1.4 1.4 3.1 2 4.9 2 1.8 0 3.6-.7 4.9-2l2.2 2.2.8-6.4z" />
          </svg>
        )
      default:
        return (
          <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-2" width="16" height="16" viewBox="0 0 16 16">
            <path d="M11.4 0L10 1.4l2 2H8.4c-2.8 0-5 2.2-5 5V12l-2-2L0 11.4l3.7 3.7c.2.2.4.3.7.3.3 0 .5-.1.7-.3l3.7-3.7L7.4 10l-2 2V8.4c0-1.7 1.3-3 3-3H12l-2 2 1.4 1.4 3.7-3.7c.4-.4.4-1 0-1.4L11.4 0z" />
          </svg>
        )
    }
  } 

  return {
    descriptionOpen,
    setDescriptionOpen,
    statusColor,
    typeIcon,
  }
}
