'use client'

import { useState } from 'react'
import Toast from '@/components/toast'

export default function Toasts01() {

  const [toastWarningOpen, setToastWarningOpen] = useState<boolean>(true)
  const [toastErrorOpen, setToastErrorOpen] = useState<boolean>(true)
  const [toastSuccessOpen, setToastSuccessOpen] = useState<boolean>(true)
  const [toastInfoOpen, setToastInfoOpen] = useState<boolean>(true)

  return (
    <div className="space-y-3">

      <Toast type="warning" open={toastWarningOpen} setOpen={setToastWarningOpen}>
        A warning toast.
      </Toast>

      <Toast type="success" open={toastSuccessOpen} setOpen={setToastSuccessOpen}>
        A successful toast.
      </Toast>

      <Toast type="error" open={toastErrorOpen} setOpen={setToastErrorOpen}>
        A dangerous toast.
      </Toast>

      <Toast open={toastInfoOpen} setOpen={setToastInfoOpen}>
        An informational toast.
      </Toast> 

    </div>
  )
}
