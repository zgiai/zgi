'use client'

import { useState } from 'react'
import Banner02 from '@/components/banner-02'

export default function Banners02() {

  const [banner2WarningOpen, setBanner2WarningOpen] = useState<boolean>(true)
  const [banner2ErrorOpen, setBanner2ErrorOpen] = useState<boolean>(true)
  const [banner2SuccessOpen, setBanner2SuccessOpen] = useState<boolean>(true)
  const [banner2InfoOpen, setBanner2InfoOpen] = useState<boolean>(true)

  return (
    <div className="space-y-3">

      <Banner02 type="warning" open={banner2WarningOpen} setOpen={setBanner2WarningOpen}>
        We're currently experiencing an increase in inquiries. There may be a delay in responses from the Support.
      </Banner02>

      <Banner02 type="success" open={banner2SuccessOpen} setOpen={setBanner2SuccessOpen}>
        We're currently experiencing an increase in inquiries. There may be a delay in responses from the Support.
      </Banner02>

      <Banner02 type="error" open={banner2ErrorOpen} setOpen={setBanner2ErrorOpen}>
        We're currently experiencing an increase in inquiries. There may be a delay in responses from the Support.
      </Banner02>

      <Banner02 open={banner2InfoOpen} setOpen={setBanner2InfoOpen}>
        We're currently experiencing an increase in inquiries. There may be a delay in responses from the Support.
      </Banner02>

    </div>
  )
}
