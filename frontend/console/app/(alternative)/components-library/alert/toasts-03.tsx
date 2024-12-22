'use client'

import { useState } from 'react'
import Toast03 from '@/components/toast-03'

export default function Toasts03() {

  const [toast3WarningOpen, setToast3WarningOpen] = useState<boolean>(true)
  const [toast3ErrorOpen, setToast3ErrorOpen] = useState<boolean>(true)
  const [toast3SuccessOpen, setToast3SuccessOpen] = useState<boolean>(true)
  const [toast3InfoOpen, setToast3InfoOpen] = useState<boolean>(true)

  return (
    <div className="space-y-3">

      <Toast03 type="warning" open={toast3WarningOpen} setOpen={setToast3WarningOpen}>
        A warning toast.
      </Toast03>

      <Toast03 type="success" open={toast3SuccessOpen} setOpen={setToast3SuccessOpen}>
        A successful toast.
      </Toast03>

      <Toast03 type="error" open={toast3ErrorOpen} setOpen={setToast3ErrorOpen}>
        A dangerous toast.
      </Toast03>

      <Toast03 open={toast3InfoOpen} setOpen={setToast3InfoOpen}>
        An informational toast.
      </Toast03> 

    </div>
  )
}
