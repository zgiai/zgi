import type React from 'react';

import { cn } from '@/lib/utils';

const HIDDEN_TRANSLATE_X = 'calc(100% + var(--workflow-panel-stack-right, 0px) + 24px)';

/**
 * @util getRightPanelMotionStyle
 * @description Composes React Flow panel stack offsets with the temporary right-slide transform.
 */
export function getRightPanelMotionStyle(
  panelStyle: React.CSSProperties,
  temporarilyHidden: boolean
): React.CSSProperties {
  const baseTransform =
    typeof panelStyle.transform === 'string' && panelStyle.transform.length > 0
      ? panelStyle.transform
      : 'translate(0px, 0px)';

  return {
    ...panelStyle,
    transform: `${baseTransform} translateX(${temporarilyHidden ? HIDDEN_TRANSLATE_X : '0px'})`,
  };
}

/**
 * @util getRightPanelMotionClassName
 * @description Shared right-panel transition classes for temporary canvas-operation hiding.
 */
export function getRightPanelMotionClassName(className: string, temporarilyHidden: boolean) {
  return cn(
    className,
    'transition-[transform,opacity] duration-200 ease-out will-change-transform motion-reduce:transition-none',
    temporarilyHidden ? 'pointer-events-none opacity-0' : 'opacity-100'
  );
}
