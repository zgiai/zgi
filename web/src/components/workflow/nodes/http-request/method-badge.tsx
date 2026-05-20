import React from 'react';
import { cn } from '@/lib/utils';

/** Color mapping for HTTP methods */
const METHOD_COLORS: Record<string, string> = {
  GET: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  POST: 'bg-blue-50 text-blue-700 border-blue-200',
  PUT: 'bg-amber-50 text-amber-700 border-amber-200',
  DELETE: 'bg-red-50 text-red-700 border-red-200',
  PATCH: 'bg-violet-50 text-violet-700 border-violet-200',
  HEAD: 'bg-slate-50 text-slate-700 border-slate-200',
};

interface MethodBadgeProps {
  method: string;
  className?: string;
}

/**
 * Badge component for displaying HTTP methods with color coding
 */
const MethodBadge: React.FC<MethodBadgeProps> = ({ method, className }) => {
  const color = METHOD_COLORS[method] || METHOD_COLORS.GET;
  return (
    <div className={cn('items-center flex h-5', className)}>
      <div className={cn('rounded border px-2 text-sm font-medium', color)}>{method}</div>
    </div>
  );
};

export default MethodBadge;
