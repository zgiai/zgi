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
        <style jsx global>{`
        .custom-thin-scrollbar {
          scrollbar-width: thin;
          scrollbar-color: #E2E8F0 transparent;
        }

        .custom-thin-scrollbar::-webkit-scrollbar {
          width: 3px; 
        }

        .custom-thin-scrollbar::-webkit-scrollbar-track {
          background: transparent;
          margin: 4px 0;
        }

        .custom-thin-scrollbar::-webkit-scrollbar-thumb {
          background-color: #E2E8F0;
          border-radius: 1.5px;
          transition: background-color 0.2s ease;
        }

        .custom-thin-scrollbar::-webkit-scrollbar-thumb:hover {
          background-color: #CBD5E1;
        }

        .custom-thin-scrollbar {
          scrollbar-width: thin;
          transition: scrollbar-color 0.2s ease;
        }

        .custom-thin-scrollbar:not(:hover)::-webkit-scrollbar-thumb {
          background-color: transparent;
        }

        .custom-thin-scrollbar:not(:hover) {
          scrollbar-color: transparent transparent;
        }

        /* Firefox specific styles */
        @supports (scrollbar-width: thin) {
          .custom-thin-scrollbar {
            scrollbar-width: thin;
            scrollbar-color: #E2E8F0 transparent;
          }
        }
      `}</style>
      </div>
    </React.Fragment>
  )
}

export default MyApp
