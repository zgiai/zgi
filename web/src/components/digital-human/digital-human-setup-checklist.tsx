'use client';

import { useEffect, useMemo, useRef } from 'react';
import { useSearchParams } from 'next/navigation';
import { CheckCircle2, Circle, ListChecks } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  useDigitalHumanDraftBindings,
  useDigitalHumanSetup,
  useDigitalHumanSkillCandidates,
} from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { DigitalHumanSetupItem } from '@/services/types/digital-human';

// Operation names the checklist can explain; others show their id.
const OPERATION_KEYS = {
  'server.system.snapshot': 'snapshot',
  'server.service.status': 'status',
  'server.journal.tail': 'journal',
  'server.service.restart': 'restart',
  'server.service.start': 'start',
  'server.service.stop': 'stop',
  'server.service.reload': 'reload',
} as const;

// Where each kind of item is done: the bindings section that adds it.
const ITEM_TARGETS: Record<DigitalHumanSetupItem['kind'], string> = {
  skill: 'digital-human-bindings-skills',
  connector: 'digital-human-bindings-connectors',
};

/**
 * What a digital human started from a template still needs before it can
 * publish (ADR 0023). Each item opens the place to do it, and turns done as
 * soon as the draft's bindings meet it; the list hides once complete.
 */
export function DigitalHumanSetupChecklist({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const searchParams = useSearchParams();
  const containerRef = useRef<HTMLElement>(null);
  const bindings = useDigitalHumanDraftBindings(id).data?.data;
  // The checklist follows the draft's bindings: any change refetches it.
  const bindingsVersion = useMemo(
    () =>
      bindings
        ? `${bindings.draft_lock_version}:${bindings.items
            .map(item => `${item.binding_type}:${item.resource_id}:${item.binding_revision}`)
            .join(',')}`
        : 'loading',
    [bindings]
  );
  const setup = useDigitalHumanSetup(id, bindingsVersion).data?.data;
  const skillCandidates = useDigitalHumanSkillCandidates().data?.data?.items;
  const skillNames = useMemo(
    () => new Map((skillCandidates ?? []).map(item => [item.resource_id, item.display_name])),
    [skillCandidates]
  );
  const visible = Boolean(setup && !setup.complete && setup.items.length > 0);
  const arrivedForSetup = searchParams.get('setup') === '1';

  useEffect(() => {
    if (visible && arrivedForSetup) {
      containerRef.current?.scrollIntoView({ block: 'start' });
    }
  }, [visible, arrivedForSetup]);

  if (!setup || !visible) return null;

  const operationNames = (ids: readonly string[] = []) =>
    ids
      .map(operation =>
        operation in OPERATION_KEYS
          ? t(`setup.operations.${OPERATION_KEYS[operation as keyof typeof OPERATION_KEYS]}`)
          : operation
      )
      .join(t('setup.listSeparator'));

  const itemTitle = (item: DigitalHumanSetupItem) =>
    item.kind === 'skill'
      ? t('setup.skillTitle', { name: skillNames.get(item.id) || item.id })
      : item.id === 'serverops_ssh'
        ? t('setup.sshTitle')
        : t('setup.connectorTitle', { name: item.id });

  const itemDescription = (item: DigitalHumanSetupItem) => {
    if (item.kind === 'skill') return t('setup.skillDescription');
    const required = t('setup.connectorRequires', {
      operations: operationNames(item.operation_ids),
    });
    const optional = item.optional_operation_ids?.length
      ? ` ${t('setup.connectorOptional', { operations: operationNames(item.optional_operation_ids) })}`
      : '';
    return required + optional;
  };

  const open = (item: DigitalHumanSetupItem) => {
    document.getElementById(ITEM_TARGETS[item.kind])?.scrollIntoView({ block: 'center' });
  };

  const remaining = setup.items.filter(item => !item.met).length;

  return (
    <section
      ref={containerRef}
      id="configuration-setup"
      className="scroll-mt-14 rounded-xl border border-amber-500/40 bg-amber-500/5 p-4"
    >
      <div className="flex items-start gap-3">
        <ListChecks className="mt-0.5 size-5 shrink-0 text-amber-600" />
        <div className="min-w-0">
          <h3 className="text-sm font-semibold">{t('setup.title')}</h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('setup.description', { count: remaining })}
          </p>
        </div>
      </div>
      <ul className="mt-3 space-y-2">
        {setup.items.map(item => (
          <li
            key={`${item.kind}:${item.id}`}
            className={cn(
              'flex items-start gap-3 rounded-lg border bg-background px-3 py-2.5',
              item.met && 'opacity-70'
            )}
          >
            {item.met ? (
              <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-600" aria-hidden />
            ) : (
              <Circle className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
            )}
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">
                {itemTitle(item)}
                <span className="sr-only">{item.met ? t('setup.done') : t('setup.pending')}</span>
              </p>
              <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                {itemDescription(item)}
              </p>
            </div>
            {!item.met ? (
              <Button type="button" size="sm" variant="outline" onClick={() => open(item)}>
                {item.kind === 'skill' ? t('setup.bindSkill') : t('setup.connect')}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
