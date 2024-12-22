'use client'

import { useState } from 'react'
import Banner from '@/components/banner'

export default function Banners01() {

  const [bannerWarningOpen, setBannerWarningOpen] = useState<boolean>(true)
  const [bannerErrorOpen, setBannerErrorOpen] = useState<boolean>(true)
  const [bannerSuccessOpen, setBannerSuccessOpen] = useState<boolean>(true)
  const [bannerInfoOpen, setBannerInfoOpen] = useState<boolean>(true)

  return (
    <div className="space-y-3">

      <Banner type="warning" open={bannerWarningOpen} setOpen={setBannerWarningOpen}>
        We're currently experiencing an increase in inquiries. There may be a delay in responses from the Support.
      </Banner>

      <Banner type="success" open={bannerSuccessOpen} setOpen={setBannerSuccessOpen}>
        We're currently experiencing an increase in inquiries. There may be a delay in responses from the Support.
      </Banner>

      <Banner type="error" open={bannerErrorOpen} setOpen={setBannerErrorOpen}>
        We're currently experiencing an increase in inquiries. There may be a delay in responses from the Support.
      </Banner>

      <Banner open={bannerInfoOpen} setOpen={setBannerInfoOpen}>
        We're currently experiencing an increase in inquiries. There may be a delay in responses from the Support.
      </Banner>

    </div>
  )
}
