import { ReactElement } from 'react'

interface Toast03Props {
  children: React.ReactNode
  className?: string
  type?: 'warning' | 'error' | 'success' | ''
  open: boolean
  setOpen: (open: boolean) => void
}

export default function Toast03({
  children,
  className = '',
  type = '',
  open,
  setOpen
}: Toast03Props) {

  const typeIcon = (type: string): ReactElement => {
    switch (type) {
      case 'warning':
        return (
          <svg className="shrink-0 fill-current text-yellow-500 mt-[3px] mr-3" width="16" height="16" viewBox="0 0 16 16">
            <path d="M8 0C3.6 0 0 3.6 0 8s3.6 8 8 8 8-3.6 8-8-3.6-8-8-8zm0 12c-.6 0-1-.4-1-1s.4-1 1-1 1 .4 1 1-.4 1-1 1zm1-3H7V4h2v5z" />
          </svg>
        )
      case 'error':
        return (
          <svg className="shrink-0 fill-current text-red-500 mt-[3px] mr-3" width="16" height="16" viewBox="0 0 16 16">
            <path d="M8 0C3.6 0 0 3.6 0 8s3.6 8 8 8 8-3.6 8-8-3.6-8-8-8zm3.5 10.1l-1.4 1.4L8 9.4l-2.1 2.1-1.4-1.4L6.6 8 4.5 5.9l1.4-1.4L8 6.6l2.1-2.1 1.4 1.4L9.4 8l2.1 2.1z" />
          </svg>
        )
      case 'success':
        return (
          <svg className="shrink-0 fill-current text-green-500 mt-[3px] mr-3" width="16" height="16" viewBox="0 0 16 16">
            <path d="M8 0C3.6 0 0 3.6 0 8s3.6 8 8 8 8-3.6 8-8-3.6-8-8-8zM7 11.4L3.6 8 5 6.6l2 2 4-4L12.4 6 7 11.4z" />
          </svg>
        )
      default:
        return (
          <svg className="shrink-0 fill-current text-violet-500 mt-[3px] mr-3" width="16" height="16" viewBox="0 0 16 16">
            <path d="M8 0C3.6 0 0 3.6 0 8s3.6 8 8 8 8-3.6 8-8-3.6-8-8-8zm1 12H7V7h2v5zM8 6c-.6 0-1-.4-1-1s.4-1 1-1 1 .4 1 1-.4 1-1 1z" />
          </svg>
        )
    }
  }

  return (
    <>
      {open &&
        <div className={className} role="alert">
          <div className="inline-flex min-w-[20rem] px-4 py-2 rounded-lg text-sm bg-white dark:bg-gray-800 shadow-sm border border-gray-200 dark:border-gray-700/60 text-gray-600 dark:text-gray-100">
            <div className="flex w-full justify-between items-start">
              <div className="flex">
                {typeIcon(type)}
                <div>
                  {children}
                </div>
              </div>
              <button className="dark:text-gray-400 opacity-60 hover:opacity-70 ml-3 mt-[3px]" onClick={() => setOpen(false)}>
                <div className="sr-only">Close</div>
                <svg className="fill-current" width="16" height="16" viewBox="0 0 16 16">
                  <path d="M7.95 6.536l4.242-4.243a1 1 0 111.415 1.414L9.364 7.95l4.243 4.242a1 1 0 11-1.415 1.415L7.95 9.364l-4.243 4.243a1 1 0 01-1.414-1.415L6.536 7.95 2.293 3.707a1 1 0 011.414-1.414L7.95 6.536z" />
                </svg>
              </button>
            </div>
          </div>
        </div>
      }
    </>
  )
}