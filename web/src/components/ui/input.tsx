'use client';

import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { SearchIcon, EyeIcon, EyeOffIcon } from 'lucide-react';

import { cn } from '@/lib/utils';

const inputVariants = cva(
  'flex h-9 rounded-lg px-3 py-1 text-sm shadow-xs transition-all file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:cursor-not-allowed disabled:opacity-50 placeholder:text-muted-foreground/50 input-depth focus-border',
  {
    variants: {
      variant: {
        outline:
          'border border-border bg-background hover:border-border-strong focus-visible:border-primary',
        filled:
          'border border-transparent bg-muted/30 hover:bg-muted/50 focus-visible:bg-background focus-visible:border-primary focus-visible:shadow-sm',
      },
      error: {
        true: 'border-destructive focus-visible:border-destructive text-destructive placeholder:text-destructive/50',
      },
    },
    defaultVariants: {
      variant: 'outline',
    },
  }
);

interface InputProps extends React.ComponentProps<'input'>, VariantProps<typeof inputVariants> {
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  error?: boolean;
  errorText?: React.ReactNode;
  root?: boolean;
  showCharacterCount?: boolean;
  characterCountClassName?: string;
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  (
    {
      className,
      variant,
      type,
      leftIcon,
      rightIcon,
      error,
      errorText,
      root,
      showCharacterCount,
      characterCountClassName,
      maxLength,
      value,
      defaultValue,
      ...props
    },
    ref
  ) => {
    const isError = error || !!errorText;
    const countValue = value ?? defaultValue ?? '';
    const count = Array.from(String(countValue)).length;

    const inputNode = (
      <input
        ref={ref}
        type={type}
        data-slot="input"
        className={cn(
          inputVariants({ variant, error: isError, className: !root ? className : undefined }),
          (leftIcon || rightIcon) && 'w-full',
          leftIcon && 'pl-10',
          rightIcon && 'pr-10',
          showCharacterCount && maxLength && 'w-full pr-16'
        )}
        maxLength={maxLength}
        value={value}
        defaultValue={defaultValue}
        onWheel={e => {
          if (type === 'number') {
            e.currentTarget.blur();
          }
        }}
        {...props}
      />
    );

    const withIcons =
      leftIcon || rightIcon ? (
        <div className="relative flex items-center w-full">
          {leftIcon && (
            <div className="absolute left-3 flex items-center justify-center inset-y-0 text-muted-foreground pointer-events-none">
              {React.cloneElement(leftIcon as React.ReactElement, { className: 'size-4' })}
            </div>
          )}
          {inputNode}
          {rightIcon && (
            <div className="absolute right-3 flex items-center justify-center inset-y-0 text-muted-foreground">
              {React.cloneElement(rightIcon as React.ReactElement, { className: 'size-4' })}
            </div>
          )}
          {showCharacterCount && maxLength && !rightIcon && (
            <span
              className={cn(
                'pointer-events-none absolute bottom-1.5 right-3 rounded bg-background/80 px-1 text-[11px] text-muted-foreground',
                count > maxLength && 'text-destructive',
                characterCountClassName
              )}
            >
              {count}/{maxLength}
            </span>
          )}
        </div>
      ) : (
        <div className="relative flex w-full items-center">
          {inputNode}
          {showCharacterCount && maxLength && (
            <span
              className={cn(
                'pointer-events-none absolute bottom-1.5 right-3 rounded bg-background/80 px-1 text-[11px] text-muted-foreground',
                count > maxLength && 'text-destructive',
                characterCountClassName
              )}
            >
              {count}/{maxLength}
            </span>
          )}
        </div>
      );

    return (
      <div className={cn('flex flex-col gap-1.5', root && className)}>
        {withIcons}
        {errorText && (
          <p className="text-xs font-medium text-destructive animate-in fade-in slide-in-from-top-1 duration-200">
            {errorText}
          </p>
        )}
      </div>
    );
  }
);
Input.displayName = 'Input';

function SearchInput({ className, ...props }: InputProps) {
  return (
    <Input
      type="search"
      leftIcon={<SearchIcon />}
      className={cn('rounded-full', className)}
      {...props}
    />
  );
}

function PasswordInput({ className, ...props }: InputProps) {
  const [show, setShow] = React.useState(false);

  return (
    <Input
      type={show ? 'text' : 'password'}
      rightIcon={
        <button
          type="button"
          onClick={() => setShow(!show)}
          className="hover:bg-muted/80 hover:text-foreground text-muted-foreground/60 cursor-pointer transition-all outline-none focus-visible:ring-1 focus-visible:ring-ring rounded-md p-1 -mr-1 pointer-events-auto active:scale-90"
        >
          {show ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
        </button>
      }
      className={className}
      {...props}
    />
  );
}

export { Input, SearchInput, PasswordInput };
