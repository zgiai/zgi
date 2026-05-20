import * as React from 'react';
import { cn } from '@/lib/utils';

interface RadioGroupContextValue {
  value?: string;
  onValueChange?: (value: string) => void;
}

const RadioGroupContext = React.createContext<RadioGroupContextValue>({});

interface RadioProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'type' | 'size'> {
  label?: string;
  description?: string;
  variant?: 'default' | 'card';
  size?: 'sm' | 'md' | 'lg';
}

const Radio = React.forwardRef<HTMLInputElement, RadioProps>(
  (
    {
      className,
      label,
      description,
      variant = 'default',
      size = 'sm',
      value,
      checked,
      onChange,
      ...props
    },
    ref
  ) => {
    const radioId = React.useId();
    const context = React.useContext(RadioGroupContext);

    // Use context values if available, otherwise use props
    const isChecked = context.value !== undefined ? context.value === value : checked;
    const handleChange = context.onValueChange
      ? (e: React.ChangeEvent<HTMLInputElement>) => {
          context.onValueChange?.(e.target.value);
        }
      : onChange;

    const baseStyles = cn(
      // Base radio styles
      'relative inline-flex items-center justify-center group cursor-pointer',
      'border border-primary/50 rounded-full',
      'transition-all duration-200 ease-in-out',
      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
      'disabled:opacity-50 disabled:cursor-not-allowed',
      // Size variants
      size === 'sm' && 'w-4 h-4',
      size === 'md' && 'w-5 h-5',
      size === 'lg' && 'w-6 h-6',
      // Variant styles - use conditional classes instead of peer
      variant === 'default' && [
        'bg-background',
        'hover:border-primary/50',
        isChecked && 'border-primary',
        isChecked && 'hover:border-primary/80',
      ],
      variant === 'card' && [
        'bg-card',
        'hover:border-primary/50',
        isChecked && 'border-primary bg-primary',
        isChecked && 'hover:border-primary/80 hover:bg-primary/90',
      ],
      className
    );

    const dotStyles = cn(
      // Dot styles - use conditional classes instead of peer
      'absolute rounded-full bg-primary transition-all duration-200',
      isChecked ? 'scale-100 opacity-100 group-hover:opacity-80' : 'scale-0 opacity-0',
      // Size variants for dot
      size === 'sm' && 'w-2 h-2',
      size === 'md' && 'w-2.5 h-2.5',
      size === 'lg' && 'w-3 h-3'
    );

    const labelStyles = cn(
      'text-sm font-medium leading-none',
      props.disabled && 'cursor-not-allowed opacity-70',
      size === 'sm' && 'text-sm',
      size === 'md' && 'text-base',
      size === 'lg' && 'text-lg'
    );

    const descriptionStyles = cn(
      'text-sm text-muted-foreground',
      size === 'sm' && 'text-xs',
      size === 'md' && 'text-sm',
      size === 'lg' && 'text-base'
    );

    return (
      <div className="flex items-center space-x-1">
        <div className="relative flex items-center justify-center">
          <input
            type="radio"
            id={radioId}
            ref={ref}
            className="sr-only"
            value={value}
            checked={isChecked}
            onChange={handleChange}
            {...props}
          />
          <label htmlFor={radioId} className={baseStyles}>
            <div className={dotStyles} />
          </label>
        </div>
        {(label || description) && (
          <label
            htmlFor={radioId}
            className={cn(
              'flex cursor-pointer items-center',
              props.disabled && 'cursor-not-allowed'
            )}
          >
            <div className="flex flex-col space-y-1">
              {label && <span className={labelStyles}>{label}</span>}
              {description && <span className={descriptionStyles}>{description}</span>}
            </div>
          </label>
        )}
      </div>
    );
  }
);

Radio.displayName = 'Radio';

interface RadioGroupProps {
  children: React.ReactNode;
  value?: string;
  onValueChange?: (value: string) => void;
  className?: string;
  orientation?: 'horizontal' | 'vertical';
}

const RadioGroup = React.forwardRef<HTMLDivElement, RadioGroupProps>(
  ({ children, value, onValueChange, className, orientation = 'vertical' }, ref) => {
    const contextValue = React.useMemo(
      () => ({
        value,
        onValueChange,
      }),
      [value, onValueChange]
    );

    return (
      <RadioGroupContext.Provider value={contextValue}>
        <div
          ref={ref}
          className={cn(
            'flex',
            orientation === 'horizontal' ? 'flex-row space-x-2' : 'flex-col space-y-2',
            className
          )}
        >
          {children}
        </div>
      </RadioGroupContext.Provider>
    );
  }
);

RadioGroup.displayName = 'RadioGroup';

export { Radio, RadioGroup };
