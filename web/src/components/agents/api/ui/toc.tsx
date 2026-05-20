'use client';

import React, { useEffect, useRef, useState } from 'react';
import { cn } from '@/lib/utils';

interface TocItem {
  id: string;
  text: string;
  level: number;
}

export interface TocProps {
  rootRef: React.RefObject<HTMLElement>;
  className?: string;
  variant?: 'inline' | 'floating';
}

export function Toc({ rootRef, variant = 'inline', className }: TocProps) {
  const [items, setItems] = useState<TocItem[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);

  // Programmatic scroll lock to avoid IO overriding the clicked highlight
  const isAutoScrolling = useRef(false);
  const releaseTimer = useRef<number | null>(null);
  const scrollRootRef = useRef<HTMLElement | null>(null);
  const HEADER_OFFSET = 80; // apply when scrolling window

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;

    let intersectionObserver: IntersectionObserver | null = null;
    let currentHeadings: HTMLHeadingElement[] = [];

    // Find nearest scroll container for the content
    const getScrollContainer = (el: HTMLElement | null): HTMLElement | null => {
      let cur: HTMLElement | null = el;
      while (cur && cur !== document.body) {
        const style = getComputedStyle(cur);
        const overflowY = style.overflowY || style.overflow;
        const canScroll = /(auto|scroll)/.test(overflowY);
        if (canScroll && cur.scrollHeight > cur.clientHeight) return cur;
        cur = cur.parentElement;
      }
      return null;
    };

    const baseTop = () =>
      scrollRootRef.current ? scrollRootRef.current.getBoundingClientRect().top : 0;

    // Setup or re-setup the intersection observer
    const setupObserver = () => {
      // Disconnect previous observer
      if (intersectionObserver) {
        intersectionObserver.disconnect();
      }

      // Collect headings within provided root
      currentHeadings = Array.from(
        root.querySelectorAll('h1,h2,h3,h4,h5,h6')
      ) as HTMLHeadingElement[];
      const parsed: TocItem[] = currentHeadings
        .filter(h => !!h.id)
        .map(h => {
          const level = Number(h.tagName.substring(1));
          const text = (h.textContent || '').replace(/\s*#\s*$/, '').trim();
          return { id: h.id, text, level };
        });
      setItems(parsed);

      scrollRootRef.current = getScrollContainer(root);

      // IntersectionObserver should use the scroll container as root when present
      intersectionObserver = new IntersectionObserver(
        entries => {
          if (isAutoScrolling.current) return; // ignore during programmatic scroll

          const visible = entries
            .filter(e => e.isIntersecting)
            .sort(
              (a, b) => (a.target as HTMLElement).offsetTop - (b.target as HTMLElement).offsetTop
            );

          if (visible.length > 0) {
            const top = visible[0].target as HTMLHeadingElement;
            setActiveId(top.id);
          } else {
            const refTop = baseTop();
            const nearestAbove = currentHeadings
              .filter(h => h.getBoundingClientRect().top - refTop < 60)
              .sort((a, b) => b.getBoundingClientRect().top - a.getBoundingClientRect().top)[0];
            if (nearestAbove) setActiveId(nearestAbove.id);
          }
        },
        scrollRootRef.current
          ? { root: scrollRootRef.current, rootMargin: '0px 0px -60% 0px', threshold: [0, 1] }
          : { rootMargin: '0px 0px -60% 0px', threshold: [0, 1] }
      );

      currentHeadings.forEach(h => intersectionObserver?.observe(h));
    };

    // Initial setup
    setupObserver();

    // Watch for DOM changes to re-setup observer when headings are added/removed
    const mutationObserver = new MutationObserver(() => {
      // Debounce: only re-setup if heading count changed
      const newHeadings = root.querySelectorAll('h1,h2,h3,h4,h5,h6');
      if (newHeadings.length !== currentHeadings.length) {
        setupObserver();
      }
    });
    mutationObserver.observe(root, { childList: true, subtree: true });

    return () => {
      intersectionObserver?.disconnect();
      mutationObserver.disconnect();
      if (releaseTimer.current) window.clearTimeout(releaseTimer.current);
      isAutoScrolling.current = false;
    };
  }, [rootRef]);

  const indentClass = (level: number) => {
    switch (level) {
      case 1:
        return 'pl-0';
      case 2:
        return 'pl-2';
      case 3:
        return 'pl-4';
      case 4:
        return 'pl-6';
      case 5:
        return 'pl-8';
      default:
        return 'pl-10';
    }
  };

  const handleClick = (e: React.MouseEvent<HTMLAnchorElement>, id: string) => {
    e.preventDefault();
    const el = document.getElementById(id);
    if (!el) return;

    const container = scrollRootRef.current;
    const effectiveOffset = container ? 0 : HEADER_OFFSET;

    // Lock IO updates during programmatic scroll
    isAutoScrolling.current = true;
    setActiveId(id);
    history.replaceState(null, '', `#${id}`);

    if (container) {
      const containerTop = container.getBoundingClientRect().top;
      const targetTopRel = el.getBoundingClientRect().top - containerTop + container.scrollTop;
      const target = Math.max(0, targetTopRel - effectiveOffset);
      container.scrollTo({ top: target, behavior: 'smooth' });
      const distance = Math.abs(container.scrollTop - target);
      const duration = Math.min(1200, Math.max(300, Math.floor(distance * 0.35)));
      if (releaseTimer.current) window.clearTimeout(releaseTimer.current);
      releaseTimer.current = window.setTimeout(() => {
        isAutoScrolling.current = false;
      }, duration);
    } else {
      const targetTop = el.getBoundingClientRect().top + window.scrollY - effectiveOffset;
      const target = Math.max(0, targetTop);
      window.scrollTo({ top: target, behavior: 'smooth' });
      const distance = Math.abs(window.scrollY - target);
      const duration = Math.min(1200, Math.max(300, Math.floor(distance * 0.35)));
      if (releaseTimer.current) window.clearTimeout(releaseTimer.current);
      releaseTimer.current = window.setTimeout(() => {
        isAutoScrolling.current = false;
      }, duration);
    }
  };

  const containerClass =
    variant === 'floating'
      ? 'text-[13px]'
      : 'sticky top-16 max-h-[calc(100vh-8rem)] overflow-auto border-l pl-4 text-[13px]';

  return (
    <aside className={cn(containerClass, className)}>
      <nav>
        <ul className="space-y-1">
          {items.map(item => {
            const isActive = item.id === activeId;
            return (
              <li key={item.id} className={indentClass(item.level)}>
                <a
                  href={`#${item.id}`}
                  onClick={e => handleClick(e, item.id)}
                  className={cn(
                    'block py-1 border-l-2 pl-2 transition-colors',
                    isActive
                      ? 'border-primary text-foreground font-medium'
                      : 'border-transparent text-muted-foreground hover:text-primary'
                  )}
                  title={item.text}
                >
                  {item.text}
                </a>
              </li>
            );
          })}
        </ul>
      </nav>
    </aside>
  );
}

export default Toc;
