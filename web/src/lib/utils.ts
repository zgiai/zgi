import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function debounce<TThis, TArgs extends unknown[]>(
  fn: (this: TThis, ...args: TArgs) => unknown,
  delay: number
): (this: TThis, ...args: TArgs) => void {
  let timer: ReturnType<typeof setTimeout> | null = null;
  return function (this: TThis, ...args: TArgs) {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      fn.apply(this, args);
      timer = null;
    }, delay);
  };
}
