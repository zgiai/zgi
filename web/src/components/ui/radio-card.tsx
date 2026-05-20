import * as React from 'react';
import { cn } from '@/lib/utils';
import type { Radio } from './radio';

interface RadioCardProps extends Omit<React.ComponentProps<typeof Radio>, 'variant' | 'size'> {
  icon?: React.ReactNode;
  warpClassName?: string;
  title?: string;
  subtitle?: string;
  description?: string;
  disabled?: boolean;
  checked?: boolean;
  hiddenRadio?: boolean;
}

const RadioCard = React.forwardRef<HTMLInputElement, RadioCardProps>(
  (
    {
      className,
      warpClassName,
      icon,
      title,
      subtitle,
      description,
      disabled = false,
      checked = false,
      hiddenRadio = false,
      ...props
    },
    ref
  ) => {
    const radioId = React.useId();

    const cardStyles = cn(
      'relative flex flex-col p-2 border rounded-lg cursor-pointer transition-all duration-200',
      'hover:border-primary group',
      'focus-within:ring-2 focus-within:ring-ring focus-within:ring-offset-2',
      checked && 'border-primary shadow-sm',
      'peer-disabled:opacity-50 peer-disabled:cursor-not-allowed peer-disabled:hover:border-border peer-disabled:hover:bg-transparent',
      className
    );

    return (
      <div className={cn('relative', warpClassName)}>
        <input
          type="radio"
          id={radioId}
          ref={ref}
          className="peer sr-only"
          disabled={disabled}
          {...props}
        />
        <label
          htmlFor={radioId}
          className={cn('block cursor-pointer h-full', disabled && 'cursor-not-allowed')}
        >
          <div className={cardStyles}>
            {/* Radio indicator */}
            {hiddenRadio ? null : (
              <div className="absolute top-1/2 -translate-y-1/2 left-2 flex items-center justify-center">
                <div
                  className={cn(
                    'w-3.5 h-3.5 border rounded-full transition-all duration-200 group-hover:border-primary',
                    'flex items-center justify-center',
                    checked ? 'border-primary' : 'border-border'
                  )}
                >
                  <div
                    className={cn(
                      'w-1.5 h-1.5 bg-primary rounded-full transition-all duration-200',
                      checked ? 'scale-100 opacity-100' : 'scale-0 opacity-0'
                    )}
                  />
                </div>
              </div>
            )}

            {/* Content */}
            <div className={cn('flex items-start space-x-3 pl-5', hiddenRadio && 'pl-0')}>
              <div className="w-full space-y-1">
                <div className="flex items-center space-x-2">
                  {icon && (
                    <div
                      className={cn(
                        'flex-shrink-0 w-6 h-6',
                        checked ? 'text-primary' : 'text-muted-foreground'
                      )}
                    >
                      {icon}
                    </div>
                  )}
                  {title && (
                    <div
                      className={cn(
                        'font-medium transition-all duration-200',
                        checked ? 'text-primary' : 'text-muted-foreground group-hover:text-primary'
                      )}
                    >
                      {title}
                    </div>
                  )}
                </div>

                {subtitle && <div className="text-sm text-muted-foreground">{subtitle}</div>}

                {description && <div className="text-xs text-muted-foreground">{description}</div>}
              </div>
            </div>
          </div>
        </label>
      </div>
    );
  }
);

RadioCard.displayName = 'RadioCard';

interface RadioCardGroupProps {
  children: React.ReactNode;
  value?: string;
  onValueChange?: (value: string) => void;
  className?: string;
  orientation?: 'horizontal' | 'vertical';
}

const RadioCardGroup = React.forwardRef<HTMLDivElement, RadioCardGroupProps>(
  ({ children, value, onValueChange, className }, ref) => {
    const groupName = React.useId();

    return (
      <div ref={ref} className={cn('grid gap-3 grid-cols-1', className)}>
        {React.Children.map(children, child => {
          if (React.isValidElement(child)) {
            return React.cloneElement(child, {
              checked: value === child.props.value,
              value: child.props.value,
              name: groupName,
              onChange: (e: React.ChangeEvent<HTMLInputElement>) => {
                onValueChange?.(e.target.value);
              },
            } as React.ComponentProps<typeof RadioCard>);
          }
          return child;
        })}
      </div>
    );
  }
);

RadioCardGroup.displayName = 'RadioCardGroup';

export { RadioCard, RadioCardGroup };
