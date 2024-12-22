'use client'

import { useState } from 'react'
import Notification from '@/components/notification'

export default function Notifications() {

  const [toast3WarningOpen, setToast3WarningOpen] = useState<boolean>(true)
  const [toast3ErrorOpen, setToast3ErrorOpen] = useState<boolean>(true)
  const [toast3SuccessOpen, setToast3SuccessOpen] = useState<boolean>(true)
  const [toast3InfoOpen, setToast3InfoOpen] = useState<boolean>(true)

  return (
    <div className="space-y-3">

      <Notification type="warning" open={toast3WarningOpen} setOpen={setToast3WarningOpen}>
        <div className="font-medium text-gray-800 dark:text-gray-100 mb-1">Merged Pull Request</div>
        <div>Lorem ipsum dolor sit amet, consectetur adipiscing sed do eiusmod tempor incididunt ut labore et dolore.</div>
      </Notification>

      <Notification type="success" open={toast3SuccessOpen} setOpen={setToast3SuccessOpen}>
        <div className="font-medium text-gray-800 dark:text-gray-100 mb-1">Merged Pull Request</div>
        <div>Lorem ipsum dolor sit amet, consectetur adipiscing sed do eiusmod tempor incididunt ut labore et dolore.</div>
      </Notification>

      <Notification type="error" open={toast3ErrorOpen} setOpen={setToast3ErrorOpen}>
        <div className="font-medium text-gray-800 dark:text-gray-100 mb-1">Merged Pull Request</div>
        <div>Lorem ipsum dolor sit amet, consectetur adipiscing sed do eiusmod tempor incididunt ut labore et dolore.</div>
      </Notification>

      <Notification open={toast3InfoOpen} setOpen={setToast3InfoOpen}>
        <div className="font-medium text-gray-800 dark:text-gray-100 mb-1">Merged Pull Request</div>
        <div>Lorem ipsum dolor sit amet, consectetur adipiscing sed do eiusmod tempor incididunt ut labore et dolore.</div>
      </Notification> 

    </div>
  )
}
