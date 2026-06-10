import React, { useState } from 'react';
import JsonView from '@uiw/react-json-view';
import { lightTheme } from '@uiw/react-json-view/light';
import type { RuntimeLabel } from './types';
import { compactValue, getReadableRecordEntries, isRecord } from './utils';

export const RuntimeValuePreview: React.FC<{
  value: unknown;
  lines?: number;
  expandable?: boolean;
  maxRecordEntries?: number;
  runtimeLabel: RuntimeLabel;
}> = ({ value, lines = 2, expandable = false, maxRecordEntries = 3, runtimeLabel }) => {
  const [expanded, setExpanded] = useState(false);
  const textValue =
    typeof value === 'string'
      ? value
      : typeof value === 'number' || typeof value === 'boolean'
        ? String(value)
        : null;
  const normalizedTextValue = textValue?.replace(/\s+/g, ' ').trim();
  const canExpand = Boolean(expandable && normalizedTextValue && normalizedTextValue.length > 90);

  if (isRecord(value)) {
    const entries = getReadableRecordEntries(value, runtimeLabel).slice(0, maxRecordEntries);
    return (
      <div className="grid gap-1">
        {entries.map(([key, label, entryValue]) => (
          <div key={key} className="flex min-w-0 gap-1.5">
            <span className="shrink-0 text-muted-foreground/70">{label}:</span>
            <span className="min-w-0 truncate text-foreground/80">
              {compactValue(entryValue, runtimeLabel, expanded ? 500 : 72)}
            </span>
          </div>
        ))}
      </div>
    );
  }

  return (
    <div className="grid gap-1">
      <div
        className="break-words text-foreground/85"
        style={
          expanded
            ? { whiteSpace: 'pre-wrap' }
            : {
                display: '-webkit-box',
                WebkitLineClamp: lines,
                WebkitBoxOrient: 'vertical',
                overflow: 'hidden',
              }
        }
      >
        {expanded && textValue ? textValue : compactValue(value, runtimeLabel)}
      </div>
      {canExpand ? (
        <button
          type="button"
          className="w-fit rounded px-1.5 py-0.5 text-[10px] font-medium text-primary transition-colors hover:bg-primary/10"
          onClick={event => {
            event.stopPropagation();
            setExpanded(prev => !prev);
          }}
        >
          {expanded ? runtimeLabel('collapse') : runtimeLabel('expandAll')}
        </button>
      ) : null}
    </div>
  );
};

export const RuntimeStructuredView: React.FC<{ value: unknown; runtimeLabel: RuntimeLabel }> = ({
  value,
  runtimeLabel,
}) => {
  const [showRaw, setShowRaw] = useState(false);

  if (typeof value === 'string') {
    return (
      <div className="whitespace-pre-wrap break-words px-3 py-2 text-[12px] leading-5 text-foreground/85">
        {value}
      </div>
    );
  }

  const renderReadableValue = () => {
    if (Array.isArray(value)) {
      return (
        <div className="grid gap-1.5 px-2 py-2">
          <div className="text-[11px] text-muted-foreground">
            {runtimeLabel('arrayCount', { count: value.length })}
          </div>
          {value.slice(0, 8).map((entry, index) => (
            <div key={index} className="rounded-md bg-background/70 px-2 py-1.5 text-[12px]">
              <RuntimeValuePreview value={entry} lines={3} runtimeLabel={runtimeLabel} />
            </div>
          ))}
          {value.length > 8 ? (
            <div className="px-1 text-[11px] text-muted-foreground">
              {runtimeLabel('arrayMore', { count: value.length - 8 })}
            </div>
          ) : null}
        </div>
      );
    }

    if (isRecord(value)) {
      const entries = getReadableRecordEntries(value, runtimeLabel);
      if (entries.length === 0) {
        return (
          <div className="px-3 py-2 text-[12px] leading-5 text-muted-foreground">
            {runtimeLabel('technicalFieldsHidden')}
          </div>
        );
      }
      return (
        <div className="grid gap-1.5 px-2 py-2">
          {entries.slice(0, 12).map(([key, label, entryValue]) => (
            <div
              key={key}
              className="grid gap-1 rounded-md bg-background/70 px-2 py-1.5 text-[12px] leading-5"
            >
              <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                {label}
              </div>
              <RuntimeValuePreview value={entryValue} lines={4} runtimeLabel={runtimeLabel} />
            </div>
          ))}
          {entries.length > 12 ? (
            <div className="px-1 text-[11px] text-muted-foreground">
              {runtimeLabel('fieldsMore', { count: entries.length - 12 })}
            </div>
          ) : null}
        </div>
      );
    }

    return (
      <div className="px-3 py-2 text-[12px] leading-5 text-foreground/85">
        {compactValue(value, runtimeLabel, 320)}
      </div>
    );
  };

  return (
    <div className="grid gap-1">
      {renderReadableValue()}
      <button
        type="button"
        className="mx-2 mb-2 w-fit rounded border border-border/50 bg-background px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        onClick={event => {
          event.stopPropagation();
          setShowRaw(prev => !prev);
        }}
      >
        {showRaw ? runtimeLabel('hideRawData') : runtimeLabel('rawData')}
      </button>
      {showRaw ? (
        <JsonView
          value={value ?? {}}
          style={{ ...lightTheme, background: 'transparent' }}
          className="border-t border-border/30 p-1 px-2.5 text-[11px]"
        />
      ) : null}
    </div>
  );
};

