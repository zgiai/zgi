'use client';

import React, { useState, useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Plus, X } from 'lucide-react';

export interface FileExtensionEditorProps {
  extensions: string[];
  onChange: (extensions: string[]) => void;
  placeholder?: string;
  label?: string;
  clearText?: string;
  addText?: string;
}

/**
 * FileExtensionEditor - A chip-based editor for file extensions
 * Supports comma-separated input for batch adding, auto-prefixes with dot
 */
export function FileExtensionEditor({
  extensions,
  onChange,
  placeholder = 'pdf, docx, txt',
  label,
  clearText = 'Clear',
}: FileExtensionEditorProps) {
  const [inputValue, setInputValue] = useState('');

  // Normalize extension (ensure starts with dot, lowercase, alphanumeric only)
  const normalizeExt = useCallback((ext: string): string => {
    // Remove leading dots, then strip all non-alphanumeric characters
    const trimmed = ext
      .trim()
      .toLowerCase()
      .replace(/^\.+/, '')
      .replace(/[^a-z0-9]/g, '');
    if (!trimmed) return '';
    return `.${trimmed}`;
  }, []);

  // Add multiple extensions (supports comma-separated input)
  const addExtensions = useCallback(
    (input: string) => {
      // Split by comma (both English and Chinese) and process each extension
      const parts = input
        .split(/[,，]+/)
        .map(s => s.trim())
        .filter(Boolean);
      const newExtensions: string[] = [];

      for (const part of parts) {
        const normalized = normalizeExt(part);
        if (normalized && !extensions.includes(normalized) && !newExtensions.includes(normalized)) {
          newExtensions.push(normalized);
        }
      }

      if (newExtensions.length > 0) {
        onChange([...extensions, ...newExtensions]);
      }
      setInputValue('');
    },
    [extensions, normalizeExt, onChange]
  );

  // Remove single extension
  const removeExtension = useCallback(
    (ext: string) => {
      onChange(extensions.filter(e => e !== ext));
    },
    [extensions, onChange]
  );

  // Clear all extensions
  const clearAll = useCallback(() => {
    onChange([]);
  }, [onChange]);

  // Handle Enter key to add
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        addExtensions(inputValue);
      }
    },
    [inputValue, addExtensions]
  );

  // Handle add button click
  const handleAddClick = useCallback(() => {
    addExtensions(inputValue);
  }, [inputValue, addExtensions]);

  return (
    <div className="space-y-2">
      {(label || extensions.length > 0) && (
        <div className="flex items-center justify-between">
          {label && <Label>{label}</Label>}
          {extensions.length > 0 && (
            <button
              type="button"
              className="text-xs text-highlight hover:underline"
              onClick={clearAll}
            >
              {clearText}
            </button>
          )}
        </div>
      )}

      {/* Chips display area */}
      {extensions.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {extensions.map(ext => (
            <span
              key={ext}
              className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-accent text-accent-foreground border"
            >
              {ext}
              <button
                type="button"
                className="ml-0.5 opacity-70 hover:opacity-100"
                onClick={() => removeExtension(ext)}
              >
                <X size={12} />
              </button>
            </span>
          ))}
        </div>
      )}

      {/* Input with add button */}
      <div className="flex gap-2">
        <Input
          type="text"
          className="flex-1"
          value={inputValue}
          onChange={e => {
            // Only allow alphanumeric, comma (for batch), and spaces (for readability)
            const filtered = e.target.value.replace(/[^a-zA-Z0-9,，\s.]/g, '');
            setInputValue(filtered);
          }}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
        />
        <Button type="button" isIcon onClick={handleAddClick} disabled={!inputValue.trim()}>
          <Plus size={22} />
        </Button>
      </div>
    </div>
  );
}

export default FileExtensionEditor;
