import type { ToastClassnames } from 'sonner';
import { cn } from '@/lib/utils';

export const workflowBillingToastClassNames = {
  toast:
    '!items-center !gap-3 !rounded-md !border !border-amber-200 !bg-amber-50 !pr-3 !text-amber-950 !shadow-[0_8px_28px_rgba(15,23,42,0.10)]',
  content: '!min-w-0 !flex-1 !gap-1 !py-0',
  title: '!text-[14px] !font-semibold !leading-5',
  description: '!text-[12px] !leading-5 !text-amber-900/85',
  icon: '!hidden !h-0 !w-0 !m-0 !p-0',
} satisfies ToastClassnames;

interface WorkflowBillingToastActionProps {
  label: string;
  onClick: () => void;
  className?: string;
}

/**
 * @component WorkflowBillingToastAction
 * @category Feature
 * @status Stable
 * @description Renders the emphasized CTA used inside billing-related toast notifications.
 * @usage Pass as the `action` node in a Sonner toast for billing errors.
 * @example
 * <WorkflowBillingToastAction label="Go to billing" onClick={() => {}} />
 */
export function WorkflowBillingToastAction({
  label,
  onClick,
  className,
}: WorkflowBillingToastActionProps) {
  return (
    <button
      type="button"
      className={cn(
        'self-center inline-flex h-8 shrink-0 items-center justify-center whitespace-nowrap rounded-[4px] border border-amber-300 bg-white px-3 text-[12px] font-semibold text-amber-950 shadow-none transition-colors hover:border-amber-400 hover:bg-amber-100 focus:outline-none focus:ring-2 focus:ring-amber-500/20',
        className
      )}
      onClick={onClick}
    >
      {label}
    </button>
  );
}
