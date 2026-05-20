import { cn } from '@/lib/utils';

interface ZgiDrawingWordmarkProps {
  className?: string;
  loop?: boolean;
  title?: string;
}

export function ZgiDrawingWordmark({
  className,
  loop = true,
  title = 'ZGI',
}: ZgiDrawingWordmarkProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 184 90"
      role="img"
      aria-label={title}
      className={cn('zgi-logo-draw h-auto w-[150px]', !loop && 'zgi-logo-draw-once', className)}
    >
      <title>{title}</title>
      <path
        className="zgi-logo-draw-blue"
        pathLength={1}
        d="m0.51 89.01v-11.05l43.98-60.6h-44.1v-16.85h68.19v12.31l-44.11 60.6h44.11v15.59h-68.07z"
      />
      <path
        className="zgi-logo-draw-black"
        pathLength={1}
        d="m113.2 88.9c-23.16 0-38.86-15.81-38.86-43.28 0-25.59 15.15-43.15 38.53-43.15 18.72 0 32.8 10.73 35.95 28.07h-17.45c-2.58-8.15-8.44-11.83-17.73-11.83-12.75 0-21.72 9.7-21.72 26.8 0 15.25 7.41 27.68 21.38 27.68 11.39 0 19.64-5.75 19.75-17.02h-18.44v-13.25h34.98v10.73c0 20.52-14.51 35.25-36.39 35.25z"
      />
      <path
        className="zgi-logo-draw-black"
        pathLength={1}
        d="m163.6 31.96h18.83v57.05h-18.83v-57.05z"
      />
      <path
        className="zgi-logo-draw-blue"
        pathLength={1}
        d="m173 21.96c-6.75 0-10.43-5.75-10.43-10.76 0-5.23 4.13-11.2 10.54-11.2 6.64 0 10.22 5.86 10.22 10.76 0 5.45-4.24 11.2-10.33 11.2z"
      />
    </svg>
  );
}
