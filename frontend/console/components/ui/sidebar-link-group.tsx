import { useState } from 'react'

interface SidebarLinkGroupProps {
  children: (handleClick: () => void, openGroup: boolean) => React.ReactNode
  open?: boolean
}

export default function SidebarLinkGroup({
  children,
  open = false
}: SidebarLinkGroupProps) {
  const [openGroup, setOpenGroup] = useState<boolean>(open)

  const handleClick = () => {
    setOpenGroup(!openGroup);
  }

  return (
    <li className={`pl-4 pr-3 py-2 rounded-lg mb-0.5 last:mb-0 bg-[linear-gradient(135deg,var(--tw-gradient-stops))] group is-link-group ${open && 'from-violet-500/[0.12] dark:from-violet-500/[0.24] to-violet-500/[0.04]'}`}>
      {children(handleClick, openGroup)}
    </li>
  )
}