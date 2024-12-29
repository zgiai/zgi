'use client'

import { useEffect, useRef, useState } from 'react'
import { useAppProvider } from '@/app/app-provider'
import { useSelectedLayoutSegments } from 'next/navigation'
import { Transition } from '@headlessui/react'
import { getBreakpoint } from '../utils/utils'
import SidebarLinkGroup from './sidebar-link-group'
import SidebarLink from './sidebar-link'
import Logo from './logo'

export default function Sidebar({
  variant = 'default',
}: {
  variant?: 'default' | 'v2'
}) {
  const sidebar = useRef<HTMLDivElement>(null)
  const { sidebarOpen, setSidebarOpen } = useAppProvider()
  const [sidebarExpanded, setSidebarExpanded] = useState<boolean>(false)
  const segments = useSelectedLayoutSegments()
  const [breakpoint, setBreakpoint] = useState<string | undefined>(getBreakpoint())
  const expandOnly = !sidebarExpanded && (breakpoint === 'lg' || breakpoint === 'xl')
  // close on click outside
  useEffect(() => {
    const clickHandler = ({ target }: { target: EventTarget | null }): void => {
      if (!sidebar.current) return
      if (!sidebarOpen || sidebar.current.contains(target as Node)) return
      setSidebarOpen(false)
    }
    document.addEventListener('click', clickHandler)
    return () => document.removeEventListener('click', clickHandler)
  })

  // close if the esc key is pressed
  useEffect(() => {
    const keyHandler = ({ keyCode }: { keyCode: number }): void => {
      if (!sidebarOpen || keyCode !== 27) return
      setSidebarOpen(false)
    }
    document.addEventListener('keydown', keyHandler)
    return () => document.removeEventListener('keydown', keyHandler)
  })

  const handleBreakpoint = () => {
    setBreakpoint(getBreakpoint())
  }

  useEffect(() => {
    window.addEventListener('resize', handleBreakpoint)
    return () => {
      window.removeEventListener('resize', handleBreakpoint)
    }
  }, [breakpoint])

  const projectLinks = [
    {
      type: 'groups',
      title: 'Pages',
      children: [
        {
          type: 'link',
          title: 'Organization',
          path: 'organization',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('organization') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} viewBox="0 0 1026 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="7526" width="16" height="16">
              <path d="M338.163055 376.734778a211.351909 105.675955 0 1 0 422.703818 0 211.351909 105.675955 0 1 0-422.703818 0Z" fill="currentColor" fillOpacity={0.5}></path>
              <path d="M831.669763 566.599243a216.107327 216.107327 0 0 0-30.293774 2.465772l-73.973168-109.022359 5.107671-4.403165 4.050912-3.522532a267.712418 267.712418 0 0 0 0-375.325765 267.712418 267.712418 0 0 0-375.325766 0 267.536292 267.536292 0 0 0 0 375.149638A207.829377 207.829377 0 0 0 389.239766 475.541796l-66.399725 111.312005A228.964568 228.964568 0 0 0 228.964568 566.599243a231.254214 231.254214 0 0 0-228.964568 228.964568 226.674923 226.674923 0 0 0 67.808738 160.979705A228.964568 228.964568 0 0 0 228.964568 1024a233.54386 233.54386 0 0 0 159.922945-67.808738 225.26591 225.26591 0 0 0 0-321.783281c-2.113519-2.113519-4.579291-4.579291-7.22119-6.868937l70.450636-116.948057a271.587203 271.587203 0 0 0 96.869625 17.612659 264.189886 264.189886 0 0 0 120.118335-28.884761l63.757826 94.0516a188.807706 188.807706 0 0 0-92.818713 165.382869 193.73925 193.73925 0 1 0 385.717234 0 190.745098 190.745098 0 0 0-194.091503-192.154111z m-441.725491 228.964568a160.979704 160.979704 0 0 1-47.201926 112.368766A172.427933 172.427933 0 0 1 228.964568 957.600275a158.513932 158.513932 0 0 1-113.425524-48.258686 157.633299 157.633299 0 0 1 0-227.027176A160.099071 160.099071 0 0 1 228.964568 634.055728a155.695906 155.695906 0 0 1 113.073272 48.787065 160.275198 160.275198 0 0 1 47.906432 113.601651z m234.424493-348.906776a200.96044 200.96044 0 0 1-74.853801 14.794633 211.351909 211.351909 0 0 1-76.791194-14.794633A191.801858 191.801858 0 0 1 408.613691 405.091159a205.715858 205.715858 0 0 1-42.622635-63.581699 208.533884 208.533884 0 0 1 0-153.406261 188.983832 188.983832 0 0 1 105.675955-105.675954 211.351909 211.351909 0 0 1 76.615067-14.618507 215.226694 215.226694 0 0 1 76.791193 14.618507A192.330237 192.330237 0 0 1 689.71173 123.288614a189.159959 189.159959 0 0 1 41.918129 64.110079 208.71001 208.71001 0 0 1 14.794634 76.791193 211.351909 211.351909 0 0 1-14.794634 76.791194 187.574819 187.574819 0 0 1-40.861369 62.877193 221.391125 221.391125 0 0 1-66.399725 42.798762z m296.949432 401.92088A129.100791 129.100791 0 0 1 880.632955 875.349157a131.74269 131.74269 0 0 1-48.258686 9.686963 127.515652 127.515652 0 0 1-48.258686-9.686963 125.754386 125.754386 0 0 1-77.671827-115.891297A123.288614 123.288614 0 0 1 832.198142 634.055728a126.987272 126.987272 0 0 1 90.352942 36.282077 125.402133 125.402133 0 0 1 36.282077 89.120055 123.288614 123.288614 0 0 1-37.514964 89.120055z" fill="currentColor"></path>
            </svg>
          ),
          href: '/organization',
          fn: () => {

          }
        },
        {
          type: 'group',
          title: 'Dashboard',
          path: 'dashboard',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('dashboard') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
              <path d="M5.936.278A7.983 7.983 0 0 1 8 0a8 8 0 1 1-8 8c0-.722.104-1.413.278-2.064a1 1 0 1 1 1.932.516A5.99 5.99 0 0 0 2 8a6 6 0 1 0 6-6c-.53 0-1.045.076-1.548.21A1 1 0 1 1 5.936.278Z" />
              <path d="M6.068 7.482A2.003 2.003 0 0 0 8 10a2 2 0 1 0-.518-3.932L3.707 2.293a1 1 0 0 0-1.414 1.414l3.775 3.775Z" />
            </svg>
          ),
          children: [
            { type: 'sublink', title: 'Main', href: '/dashboard' },
            { type: 'sublink', title: 'Analytics', href: '/dashboard/analytics' },
            { type: 'sublink', title: 'Fintech', href: '/dashboard/fintech' },
          ],
        },
        {
          type: 'group',
          title: 'E-Commerce',
          path: 'ecommerce',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('ecommerce') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
              <path d="M9 6.855A3.502 3.502 0 0 0 8 0a3.5 3.5 0 0 0-1 6.855v1.656L5.534 9.65a3.5 3.5 0 1 0 1.229 1.578L8 10.267l1.238.962a3.5 3.5 0 1 0 1.229-1.578L9 8.511V6.855ZM6.5 3.5a1.5 1.5 0 1 1 3 0 1.5 1.5 0 0 1-3 0Zm4.803 8.095c.005-.005.01-.01.013-.016l.012-.016a1.5 1.5 0 1 1-.025.032ZM3.5 11c.474 0 .897.22 1.171.563l.013.016.013.017A1.5 1.5 0 1 1 3.5 11Z" />
            </svg>
          ),
          children: [
            { type: 'sublink', title: 'Customers', href: '/ecommerce/customers' },
            { type: 'sublink', title: 'Orders', href: '/ecommerce/orders' },
            { type: 'sublink', title: 'Invoices', href: '/ecommerce/invoices' },
          ],
        },
        {
          type: 'group',
          title: 'Job Board',
          path: 'jobs',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('jobs') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
              <path d="M6.753 2.659a1 1 0 0 0-1.506-1.317L2.451 4.537l-.744-.744A1 1 0 1 0 .293 5.207l1.5 1.5a1 1 0 0 0 1.46-.048l3.5-4ZM6.753 10.659a1 1 0 1 0-1.506-1.317l-2.796 3.195-.744-.744a1 1 0 0 0-1.414 1.414l1.5 1.5a1 1 0 0 0 1.46-.049l3.5-4ZM8 4.5a1 1 0 0 1 1-1h6a1 1 0 1 1 0 2H9a1 1 0 0 1-1-1ZM9 11.5a1 1 0 1 0 0 2h6a1 1 0 1 0 0-2H9Z" />
            </svg>
          ),
          children: [
            // { type: 'sublink', title: 'Listing', href: '/jobs' },
            // { type: 'sublink', title: 'Job Post', href: '/jobs/post' },
            // { type: 'sublink', title: 'Company Profile', href: '/jobs/company' },
          ],
        },
        {
          type: 'link',
          title: 'Calendar',
          path: 'calendar',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('calendar') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
              <path d="M5 4a1 1 0 0 0 0 2h6a1 1 0 1 0 0-2H5Z" />
              <path d="M4 0a4 4 0 0 0-4 4v8a4 4 0 0 0 4 4h8a4 4 0 0 0 4-4V4a4 4 0 0 0-4-4H4ZM2 4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4Z" />
            </svg>
          ),
          href: '/calendar',
        },
        {
          type: 'link',
          title: 'Campaigns',
          path: 'campaigns',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('campaigns') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
              <path d="M6.649 1.018a1 1 0 0 1 .793 1.171L6.997 4.5h3.464l.517-2.689a1 1 0 1 1 1.964.378L12.498 4.5h2.422a1 1 0 0 1 0 2h-2.807l-.77 4h2.117a1 1 0 1 1 0 2h-2.501l-.517 2.689a1 1 0 1 1-1.964-.378l.444-2.311H5.46l-.517 2.689a1 1 0 1 1-1.964-.378l.444-2.311H1a1 1 0 1 1 0-2h2.807l.77-4H2.46a1 1 0 0 1 0-2h2.5l.518-2.689a1 1 0 0 1 1.17-.793ZM9.307 10.5l.77-4H6.612l-.77 4h3.464Z" />
            </svg>
          ),
          href: '/campaigns',
        },
        {
          type: 'group',
          title: 'Settings',
          path: 'settings',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('settings') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
              <path d="M10.5 1a3.502 3.502 0 0 1 3.355 2.5H15a1 1 0 1 1 0 2h-1.145a3.502 3.502 0 0 1-6.71 0H1a1 1 0 0 1 0-2h6.145A3.502 3.502 0 0 1 10.5 1ZM9 4.5a1.5 1.5 0 1 1 3 0 1.5 1.5 0 0 1-3 0ZM5.5 9a3.502 3.502 0 0 1 3.355 2.5H15a1 1 0 1 1 0 2H8.855a3.502 3.502 0 0 1-6.71 0H1a1 1 0 1 1 0-2h1.145A3.502 3.502 0 0 1 5.5 9ZM4 12.5a1.5 1.5 0 1 0 3 0 1.5 1.5 0 0 0-3 0Z" fillRule="evenodd" />
            </svg>
          ),
          children: [
            { type: 'sublink', title: 'My Account', href: '/settings/account' },
            { type: 'sublink', title: 'My Notifications', href: '/settings/notifications' },
            { type: 'sublink', title: 'Connected Apps', href: '/settings/apps' },
            { type: 'sublink', title: 'Plans', href: '/settings/plans' },
            { type: 'sublink', title: 'Billing & Invoices', href: '/settings/billing' },
            { type: 'sublink', title: 'Give Feedback', href: '/settings/feedback' },
          ],
        },
        {
          type: 'group',
          title: 'Utility',
          path: 'utility',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('utility') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
              <path d="M14.75 2.5a1.25 1.25 0 1 0 0-2.5 1.25 1.25 0 0 0 0 2.5ZM14.75 16a1.25 1.25 0 1 0 0-2.5 1.25 1.25 0 0 0 0 2.5ZM2.5 14.75a1.25 1.25 0 1 1-2.5 0 1.25 1.25 0 0 1 2.5 0ZM1.25 2.5a1.25 1.25 0 1 0 0-2.5 1.25 1.25 0 0 0 0 2.5Z" />
              <path d="M8 2a6 6 0 1 0 0 12A6 6 0 0 0 8 2ZM4 8a4 4 0 1 1 8 0 4 4 0 0 1-8 0Z" />
            </svg>
          ),
          children: [
            { type: 'sublink', title: 'Changelog', href: '/utility/changelog' },
            { type: 'sublink', title: 'Roadmap', href: '/utility/roadmap' },
            { type: 'sublink', title: 'FAQs', href: '/utility/faqs' },
            { type: 'sublink', title: 'Empty State', href: '/utility/empty-state' },
            { type: 'sublink', title: '404', href: '/utility/404' },
          ],
        },
      ],
    },
  ]

  const organizationLinks = [
    {
      type: 'groups',
      title: 'Pages',
      children: [
        {
          type: 'link',
          title: 'Organization',
          path: 'organization',
          icon: (
            <svg className={`shrink-0 fill-current ${segments.includes('organization') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} viewBox="0 0 1026 1024" version="1.1" xmlns="http://www.w3.org/2000/svg" p-id="7526" width="16" height="16">
              <path d="M338.163055 376.734778a211.351909 105.675955 0 1 0 422.703818 0 211.351909 105.675955 0 1 0-422.703818 0Z" fill="currentColor" fillOpacity={0.5}></path>
              <path d="M831.669763 566.599243a216.107327 216.107327 0 0 0-30.293774 2.465772l-73.973168-109.022359 5.107671-4.403165 4.050912-3.522532a267.712418 267.712418 0 0 0 0-375.325765 267.712418 267.712418 0 0 0-375.325766 0 267.536292 267.536292 0 0 0 0 375.149638A207.829377 207.829377 0 0 0 389.239766 475.541796l-66.399725 111.312005A228.964568 228.964568 0 0 0 228.964568 566.599243a231.254214 231.254214 0 0 0-228.964568 228.964568 226.674923 226.674923 0 0 0 67.808738 160.979705A228.964568 228.964568 0 0 0 228.964568 1024a233.54386 233.54386 0 0 0 159.922945-67.808738 225.26591 225.26591 0 0 0 0-321.783281c-2.113519-2.113519-4.579291-4.579291-7.22119-6.868937l70.450636-116.948057a271.587203 271.587203 0 0 0 96.869625 17.612659 264.189886 264.189886 0 0 0 120.118335-28.884761l63.757826 94.0516a188.807706 188.807706 0 0 0-92.818713 165.382869 193.73925 193.73925 0 1 0 385.717234 0 190.745098 190.745098 0 0 0-194.091503-192.154111z m-441.725491 228.964568a160.979704 160.979704 0 0 1-47.201926 112.368766A172.427933 172.427933 0 0 1 228.964568 957.600275a158.513932 158.513932 0 0 1-113.425524-48.258686 157.633299 157.633299 0 0 1 0-227.027176A160.099071 160.099071 0 0 1 228.964568 634.055728a155.695906 155.695906 0 0 1 113.073272 48.787065 160.275198 160.275198 0 0 1 47.906432 113.601651z m234.424493-348.906776a200.96044 200.96044 0 0 1-74.853801 14.794633 211.351909 211.351909 0 0 1-76.791194-14.794633A191.801858 191.801858 0 0 1 408.613691 405.091159a205.715858 205.715858 0 0 1-42.622635-63.581699 208.533884 208.533884 0 0 1 0-153.406261 188.983832 188.983832 0 0 1 105.675955-105.675954 211.351909 211.351909 0 0 1 76.615067-14.618507 215.226694 215.226694 0 0 1 76.791193 14.618507A192.330237 192.330237 0 0 1 689.71173 123.288614a189.159959 189.159959 0 0 1 41.918129 64.110079 208.71001 208.71001 0 0 1 14.794634 76.791193 211.351909 211.351909 0 0 1-14.794634 76.791194 187.574819 187.574819 0 0 1-40.861369 62.877193 221.391125 221.391125 0 0 1-66.399725 42.798762z m296.949432 401.92088A129.100791 129.100791 0 0 1 880.632955 875.349157a131.74269 131.74269 0 0 1-48.258686 9.686963 127.515652 127.515652 0 0 1-48.258686-9.686963 125.754386 125.754386 0 0 1-77.671827-115.891297A123.288614 123.288614 0 0 1 832.198142 634.055728a126.987272 126.987272 0 0 1 90.352942 36.282077 125.402133 125.402133 0 0 1 36.282077 89.120055 123.288614 123.288614 0 0 1-37.514964 89.120055z" fill="currentColor"></path>
            </svg>
          ),
          href: '/organization',
        }
      ],
    }
  ];

  const bottomLinks = {
    type: 'group',
    title: 'More',
    children: [
      {
        type: 'group',
        title: 'Authentication',
        path: 'authentication',
        icon: (
          <svg className={`shrink-0 fill-current text-gray-400 dark:text-gray-500`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
            <path d="M11.442 4.576a1 1 0 1 0-1.634-1.152L4.22 11.35 1.773 8.366A1 1 0 1 0 .227 9.634l3.281 4a1 1 0 0 0 1.59-.058l6.344-9ZM15.817 4.576a1 1 0 1 0-1.634-1.152l-5.609 7.957a1 1 0 0 0-1.347 1.453l.656.8a1 1 0 0 0 1.59-.058l6.344-9Z" />
          </svg>
        ),
        children: [
          { type: 'sublink', title: 'Sign in', href: '/signin' },
          { type: 'sublink', title: 'Sign up', href: '/signup' },
          { type: 'sublink', title: 'Reset Password', href: '/reset-password' },
        ],
      },
      {
        type: 'group',
        title: 'Onboarding',
        path: 'onboarding',
        icon: (
          <svg className={`shrink-0 fill-current text-gray-400 dark:text-gray-500`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
            <path d="M6.668.714a1 1 0 0 1-.673 1.244 6.014 6.014 0 0 0-4.037 4.037 1 1 0 1 1-1.916-.571A8.014 8.014 0 0 1 5.425.041a1 1 0 0 1 1.243.673ZM7.71 4.709a3 3 0 1 0 0 6 3 3 0 0 0 0-6ZM9.995.04a1 1 0 1 0-.57 1.918 6.014 6.014 0 0 1 4.036 4.037 1 1 0 0 0 1.917-.571A8.014 8.014 0 0 0 9.995.041ZM14.705 8.75a1 1 0 0 1 .673 1.244 8.014 8.014 0 0 1-5.383 5.384 1 1 0 0 1-.57-1.917 6.014 6.014 0 0 0 4.036-4.037 1 1 0 0 1 1.244-.673ZM1.958 9.424a1 1 0 0 0-1.916.57 8.014 8.014 0 0 0 5.383 5.384 1 1 0 0 0 .57-1.917 6.014 6.014 0 0 1-4.037-4.037Z" />
          </svg>
        ),
        children: [
          { type: 'sublink', title: 'Step 1', href: '/onboarding-01' },
          { type: 'sublink', title: 'Step 2', href: '/onboarding-02' },
          { type: 'sublink', title: 'Step 3', href: '/onboarding-03' },
          { type: 'sublink', title: 'Step 4', href: '/onboarding-04' },
        ],
      },
      {
        type: 'group',
        title: 'Components',
        path: 'components-library',
        icon: (
          <svg className={`shrink-0 fill-current ${segments.includes('components-library') ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
            <path d="M.06 10.003a1 1 0 0 1 1.948.455c-.019.08.01.152.078.19l5.83 3.333c.053.03.116.03.168 0l5.83-3.333a.163.163 0 0 0 .078-.188 1 1 0 0 1 1.947-.459 2.161 2.161 0 0 1-1.032 2.384l-5.83 3.331a2.168 2.168 0 0 1-2.154 0l-5.83-3.331a2.162 2.162 0 0 1-1.032-2.382Zm7.856-7.981-5.83 3.332a.17.17 0 0 0 0 .295l5.828 3.33c.054.031.118.031.17.002l5.83-3.333a.17.17 0 0 0 0-.294L8.085 2.023a.172.172 0 0 0-.17-.001ZM9.076.285l5.83 3.332c1.458.833 1.458 2.935 0 3.768l-5.83 3.333c-.667.38-1.485.38-2.153-.001l-5.83-3.332c-1.457-.833-1.457-2.935 0-3.767L6.925.285a2.173 2.173 0 0 1 2.15 0Z" />
          </svg>
        ),
        children: [
          { type: 'sublink', title: 'Button', href: '/components-library/button' },
          { type: 'sublink', title: 'Input Form', href: '/components-library/form' },
          { type: 'sublink', title: 'Dropdown', href: '/components-library/dropdown' },
          { type: 'sublink', title: 'Alert & Banner', href: '/components-library/alert' },
          { type: 'sublink', title: 'Modal', href: '/components-library/modal' },
          { type: 'sublink', title: 'Pagination', href: '/components-library/pagination' },
          { type: 'sublink', title: 'Tabs', href: '/components-library/tabs' },
          { type: 'sublink', title: 'Breadcrumb', href: '/components-library/breadcrumb' },
          { type: 'sublink', title: 'Badge', href: '/components-library/badge' },
          { type: 'sublink', title: 'Avatar', href: '/components-library/avatar' },
          { type: 'sublink', title: 'Tooltip', href: '/components-library/tooltip' },
          { type: 'sublink', title: 'Accordion', href: '/components-library/accordion' },
          { type: 'sublink', title: 'Icons', href: '/components-library/icons' },
        ],
      },
    ],
  }

  return (
    <div className={`min-w-fit ${sidebarExpanded ? 'sidebar-expanded' : ''}`}>
      {/* Sidebar backdrop (mobile only) */}
      <Transition
        as="div"
        className="fixed inset-0 bg-gray-900 bg-opacity-30 z-40 lg:hidden lg:z-auto"
        show={sidebarOpen}
        enter="transition-opacity ease-out duration-200"
        enterFrom="opacity-0"
        enterTo="opacity-100"
        leave="transition-opacity ease-out duration-100"
        leaveFrom="opacity-100"
        leaveTo="opacity-0"
        aria-hidden="true"
      />

      {/* Sidebar */}
      <Transition
        show={sidebarOpen}
        unmount={false}
        as="div"
        id="sidebar"
        ref={sidebar}
        className={`flex lg:!flex flex-col absolute z-40 left-0 top-0 lg:static lg:left-auto lg:top-auto lg:translate-x-0 h-[100dvh] overflow-y-scroll lg:overflow-y-auto no-scrollbar w-64 lg:w-20 lg:sidebar-expanded:!w-64 2xl:!w-64 shrink-0 bg-white dark:bg-gray-800 p-4 transition-all duration-200 ease-in-out ${variant === 'v2' ? 'border-r border-gray-200 dark:border-gray-700/60' : 'rounded-r-2xl shadow-sm'}`}
        enterFrom="-translate-x-full"
        enterTo="translate-x-0"
        leaveFrom="translate-x-0"
        leaveTo="-translate-x-full"
      >
        {/* Sidebar header */}
        <div className="flex justify-between mb-10 pr-3 sm:px-2">
          {/* Close button */}
          <button
            className="lg:hidden text-gray-500 hover:text-gray-400"
            onClick={() => setSidebarOpen(!sidebarOpen)}
            aria-controls="sidebar"
            aria-expanded={sidebarOpen}
          >
            <span className="sr-only">Close sidebar</span>
            <svg className="w-6 h-6 fill-current" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
              <path d="M10.7 18.7l1.4-1.4L7.8 13H20v-2H7.8l4.3-4.3-1.4-1.4L4 12z" />
            </svg>
          </button>
          {/* Logo */}
          <Logo />
        </div>
        <div className="space-y-8 flex-1 justify-between flex flex-col">
          {(segments.includes('organization') ? organizationLinks : projectLinks).map((group, index) => (
            <div className="flex-1" key={index}>
              <h3 className="text-xs uppercase text-gray-400 dark:text-gray-500 font-semibold pl-3">
                <span className="hidden lg:block lg:sidebar-expanded:hidden 2xl:hidden text-center w-6" aria-hidden="true">
                  •••
                </span>
                <span className="lg:hidden lg:sidebar-expanded:block 2xl:block">{group.title}</span>
              </h3>
              <LinksItem key={index} group={group} segments={segments} expandOnly={expandOnly} setSidebarExpanded={setSidebarExpanded} />
            </div>
          ))}
          <div>
            <LinksItem group={bottomLinks} segments={segments} expandOnly={expandOnly} setSidebarExpanded={setSidebarExpanded} />
          </div>
        </div>
        {/* Expand / collapse button */}
        <div className="pt-3 hidden lg:inline-flex 2xl:hidden justify-end mt-auto">
          <div className="w-12 pl-4 pr-3 py-2">
            <button className="text-gray-400 hover:text-gray-500 dark:text-gray-500 dark:hover:text-gray-400" onClick={() => setSidebarExpanded(!sidebarExpanded)}>
              <span className="sr-only">Expand / collapse sidebar</span>
              <svg className="shrink-0 fill-current text-gray-400 dark:text-gray-500 sidebar-expanded:rotate-180" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
                <path d="M15 16a1 1 0 0 1-1-1V1a1 1 0 1 1 2 0v14a1 1 0 0 1-1 1ZM8.586 7H1a1 1 0 1 0 0 2h7.586l-2.793 2.793a1 1 0 1 0 1.414 1.414l4.5-4.5A.997.997 0 0 0 12 8.01M11.924 7.617a.997.997 0 0 0-.217-.324l-4.5-4.5a1 1 0 0 0-1.414 1.414L8.586 7M12 7.99a.996.996 0 0 0-.076-.373Z" />
              </svg>
            </button>
          </div>
        </div>
      </Transition>
    </div>
  )
}

function LinksItem({ group, segments, expandOnly, setSidebarExpanded }: { group: any, segments: any, expandOnly: boolean, setSidebarExpanded: any }) {
  return <div>
    <ul className="mt-3">
      {group.children.map((link: any, linkIndex: any) => link.type === 'link' ? (
        <li key={linkIndex} className={`pl-4 pr-3 py-2 rounded-lg mb-0.5 last:mb-0 bg-[linear-gradient(135deg,var(--tw-gradient-stops))] ${segments.includes(link.path.toLowerCase()) && 'from-violet-500/[0.12] dark:from-violet-500/[0.24] to-violet-500/[0.04]'}`}>
          <SidebarLink href={link?.href || '#0'}>
            <div className="flex items-center">
              {link.icon}
              <span className="text-sm font-medium ml-4 lg:opacity-0 lg:sidebar-expanded:opacity-100 2xl:opacity-100 duration-200">{link.title}</span>
            </div>
          </SidebarLink>
        </li>) : (
        <SidebarLinkGroup key={linkIndex} open={segments.includes(link.path.toLowerCase())}>
          {(handleClick, open) => (
            <>
              <a
                href={link?.href || '#0'}
                className={`block text-gray-800 dark:text-gray-100 truncate transition ${segments.includes(link.path.toLowerCase()) ? '' : 'hover:text-gray-900 dark:hover:text-white'}`}
                onClick={(e) => {
                  e.preventDefault();
                  expandOnly ? setSidebarExpanded(true) : handleClick();
                }}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center">
                    {link.icon}
                    <span className="text-sm font-medium ml-4 lg:opacity-0 lg:sidebar-expanded:opacity-100 2xl:opacity-100 duration-200">
                      {link.title}
                    </span>
                  </div>
                  {link.children && (
                    <div className="flex shrink-0 ml-2">
                      <svg className={`w-3 h-3 shrink-0 ml-1 fill-current text-gray-400 dark:text-gray-500 ${open && 'rotate-180'}`} viewBox="0 0 12 12">
                        <path d="M5.9 11.4L.5 6l1.4-1.4 4 4 4-4L11.3 6z" />
                      </svg>
                    </div>
                  )}
                </div>
              </a>
              {link.children && (
                <div className="lg:hidden lg:sidebar-expanded:block 2xl:block">
                  <ul className={`pl-8 mt-1 ${!open && 'hidden'}`}>
                    {link.children.map((sublink: any, sublinkIndex: any) => (
                      <li key={sublinkIndex} className="mb-1 last:mb-0">
                        <SidebarLink href={sublink.href}>
                          <span className="text-sm font-medium lg:opacity-0 lg:sidebar-expanded:opacity-100 2xl:opacity-100 duration-200">
                            {sublink.title}
                          </span>
                        </SidebarLink>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}
        </SidebarLinkGroup>
      ))}
    </ul>
  </div>
}
