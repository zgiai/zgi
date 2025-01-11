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
  links = []
}: {
  variant?: 'default' | 'v2'
  links?: any[]
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

  // useEffect(() => {
  //   console.log(segments)
  // }, [segments])

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

  const bottomLinks = {
    type: 'group',
    title: 'More',
    children: [
      {
        type: 'group',
        title: 'Authentication',
        path: 'authentication',
        icon: (
          <svg className={`shrink-0 fill-current`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
            <path d="M11.442 4.576a1 1 0 1 0-1.634-1.152L4.22 11.35 1.773 8.366A1 1 0 1 0 .227 9.634l3.281 4a1 1 0 0 0 1.59-.058l6.344-9ZM15.817 4.576a1 1 0 1 0-1.634-1.152l-5.609 7.957a1 1 0 0 0-1.347 1.453l.656.8a1 1 0 0 0 1.59-.058l6.344-9Z" />
          </svg>
        ),
        children: [
          { type: 'sublink', title: 'Sign in', href: '/signin' },
          { type: 'sublink', title: 'Sign up', href: '/signup' },
          // { type: 'sublink', title: 'Reset Password', href: '/reset-password' },
        ],
      },
      // {
      //   type: 'group',
      //   title: 'Onboarding',
      //   path: 'onboarding',
      //   icon: (
      //     <svg className={`shrink-0 fill-current`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
      //       <path d="M6.668.714a1 1 0 0 1-.673 1.244 6.014 6.014 0 0 0-4.037 4.037 1 1 0 1 1-1.916-.571A8.014 8.014 0 0 1 5.425.041a1 1 0 0 1 1.243.673ZM7.71 4.709a3 3 0 1 0 0 6 3 3 0 0 0 0-6ZM9.995.04a1 1 0 1 0-.57 1.918 6.014 6.014 0 0 1 4.036 4.037 1 1 0 0 0 1.917-.571A8.014 8.014 0 0 0 9.995.041ZM14.705 8.75a1 1 0 0 1 .673 1.244 8.014 8.014 0 0 1-5.383 5.384 1 1 0 0 1-.57-1.917 6.014 6.014 0 0 0 4.036-4.037 1 1 0 0 1 1.244-.673ZM1.958 9.424a1 1 0 0 0-1.916.57 8.014 8.014 0 0 0 5.383 5.384 1 1 0 0 0 .57-1.917 6.014 6.014 0 0 1-4.037-4.037Z" />
      //     </svg>
      //   ),
      //   children: [
      //     { type: 'sublink', title: 'Step 1', href: '/onboarding-01' },
      //     { type: 'sublink', title: 'Step 2', href: '/onboarding-02' },
      //     { type: 'sublink', title: 'Step 3', href: '/onboarding-03' },
      //     { type: 'sublink', title: 'Step 4', href: '/onboarding-04' },
      //   ],
      // },
      {
        type: 'group',
        title: 'Components',
        path: 'components-library',
        icon: (
          <svg className={`shrink-0 fill-current`} xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">
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
          <div className="flex-1 gap-4 flex flex-col">
            {links.map((group, index) => (
              <div className="flex flex-col" key={index}>
                <h3 className="text-xs uppercase text-gray-400 dark:text-gray-500 font-semibold pl-3">
                  <span className="hidden lg:block lg:sidebar-expanded:hidden 2xl:hidden text-center w-6" aria-hidden="true">
                    •••
                  </span>
                  <span className="lg:hidden lg:sidebar-expanded:block 2xl:block">{group.title}</span>
                </h3>
                <LinksItem key={index} group={group} segments={segments} expandOnly={expandOnly} setSidebarExpanded={setSidebarExpanded} />
              </div>
            ))}
          </div>
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
              <span className={`${segments.includes(link.path.toLowerCase()) ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`}>
                {link.icon}
              </span>
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
                    <span className={`${segments.includes(link.path.toLowerCase()) ? 'text-violet-500' : 'text-gray-400 dark:text-gray-500'}`}>
                      {link.icon}
                    </span>
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
