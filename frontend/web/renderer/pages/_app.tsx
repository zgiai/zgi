import type { AppProps } from 'next/app'
import localFont from 'next/font/local'
import React from 'react'

import '../styles/globals.css'

const geistSans = localFont({
  src: '/fonts/GeistVF.woff',
  variable: '--font-geist-sans',
  weight: '100 900',
})
const geistMono = localFont({
  src: '/fonts/GeistMonoVF.woff',
  variable: '--font-geist-mono',
  weight: '100 900',
})

function MyApp({ Component, pageProps }: AppProps) {
  return (
    <React.Fragment>
      <div className={`${geistSans.variable} ${geistMono.variable} antialiased`}>
        <Component {...pageProps} />
      </div>
    </React.Fragment>
  )
}

export default MyApp
