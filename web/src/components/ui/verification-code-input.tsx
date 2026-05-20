'use client';

import { useState, useRef, useEffect } from 'react';
import { cn } from '@/lib/utils';

interface VerificationCodeInputProps {
  length?: number;
  onChange: (code: string) => void;
  onComplete?: (code: string) => void;
  disabled?: boolean;
  autoFocus?: boolean;
  errorText?: string;
}

/**
 * A user-friendly verification code input with individual digit inputs
 */
export function VerificationCodeInput({
  length = 6,
  onChange,
  onComplete,
  disabled = false,
  autoFocus = true,
  errorText,
}: VerificationCodeInputProps) {
  const [code, setCode] = useState<string[]>(Array(length).fill(''));
  const inputRefs = useRef<Array<HTMLInputElement | null>>([]);

  // Initialize refs array
  useEffect(() => {
    inputRefs.current = inputRefs.current.slice(0, length);
  }, [length]);

  // Focus first input on mount if autoFocus is true
  useEffect(() => {
    if (autoFocus && inputRefs.current[0]) {
      setTimeout(() => {
        inputRefs.current[0]?.focus();
      }, 100);
    }
  }, [autoFocus]);

  const handleChange = (index: number, value: string) => {
    // Allow only digits
    if (value.length > 0 && !/^\d+$/.test(value)) {
      return;
    }

    // Handle pasted verification code
    if (value.length > 1) {
      const pastedCode = value.slice(0, length);
      const codeArray = pastedCode.split('').concat(Array(length - pastedCode.length).fill(''));

      setCode(codeArray);
      onChange(codeArray.join(''));

      if (pastedCode.length === length && onComplete) {
        onComplete(pastedCode);
      }
      return;
    }

    // Update code state
    const newCode = [...code];
    newCode[index] = value;
    setCode(newCode);
    onChange(newCode.join(''));

    // Auto focus next input when current input is filled
    if (value && index < length - 1) {
      inputRefs.current[index + 1]?.focus();
    }

    // Call onComplete when all inputs are filled
    if (newCode.every(c => c !== '') && onComplete) {
      onComplete(newCode.join(''));
    }
  };

  const handleKeyDown = (index: number, e: React.KeyboardEvent<HTMLInputElement>) => {
    // Focus previous input on backspace when current input is empty
    if (e.key === 'Backspace' && !code[index] && index > 0) {
      inputRefs.current[index - 1]?.focus();
    }

    // Navigate between inputs with arrow keys
    if (e.key === 'ArrowLeft' && index > 0) {
      e.preventDefault();
      inputRefs.current[index - 1]?.focus();
    }

    if (e.key === 'ArrowRight' && index < length - 1) {
      e.preventDefault();
      inputRefs.current[index + 1]?.focus();
    }
  };

  return (
    <div className="flex flex-col gap-1.5 w-full">
      <div className="flex space-x-2 justify-center w-full">
        {Array.from({ length }).map((_, index) => (
          <div key={index} className="relative w-12">
            <input
              ref={el => {
                inputRefs.current[index] = el;
              }}
              type="text"
              inputMode="numeric"
              pattern="\d*"
              maxLength={index === 0 ? length : 1}
              value={code[index] || ''}
              onChange={e => handleChange(index, e.target.value)}
              onKeyDown={e => handleKeyDown(index, e)}
              disabled={disabled}
              className={cn(
                'w-full h-14 text-center text-2xl font-semibold border rounded-md transition-all focus:ring-2 focus:ring-primary focus:border-primary',
                errorText ? 'border-destructive' : 'border-border'
              )}
              autoComplete="one-time-code"
              aria-label={`Verification code digit ${index + 1}`}
            />
          </div>
        ))}
      </div>
      {errorText && (
        <p className="text-xs font-medium text-destructive text-center animate-in fade-in slide-in-from-top-1 duration-200">
          {errorText}
        </p>
      )}
    </div>
  );
}
