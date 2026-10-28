'use client';

import { useMemo, useState } from 'react';
import { Check, ClipboardCopy, Plus, Server } from 'lucide-react';
import { toast } from 'sonner';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { ConnectorKeySetupPanel } from '@/components/connectors/setup/connector-key-setup-panel';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Radio, RadioGroup } from '@/components/ui/radio';
import { connectorCatalogItems, useConnectorCatalog } from '@/hooks/connectors/use-connectors';
import { useDigitalHumanConnectorCandidates } from '@/hooks/digital-human/use-digital-humans';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { PersonalConnection } from '@/services/types/connector';
import { connectorApprovalPolicy } from '../digital-human-connector-model';
import {
  createConnectorCandidateSelectable,
  selectServerConnection,
  serverPresetOf,
  type DigitalHumanCreateConnectorSelection,
  type ServerCapabilityPreset,
} from './digital-human-connector-create-model';

const PRESETS: readonly ServerCapabilityPreset[] = ['inspect', 'manage', 'full'];

/** Commands an administrator runs once on the server to prepare a dedicated account. */
const PREPARE_COMMANDS = [
  'sudo useradd -m -s /bin/bash zgi-ops',
  'sudo passwd zgi-ops',
  "echo 'zgi-ops ALL=(root) NOPASSWD: /usr/bin/systemctl' | sudo tee /etc/sudoers.d/zgi-ops",
  'sudo chmod 440 /etc/sudoers.d/zgi-ops',
].join('\n');

/**
 * The server a template needs, as its own step: choose a saved server or add
 * one here, then say what the specialist may do and how it asks. Everything
 * stays a local selection until the digital human is created.
 */
export function DigitalHumanServerStep({
  connectorId,
  selections,
  onChangeSelections,
  disabled = false,
}: {
  connectorId: string;
  selections: readonly DigitalHumanCreateConnectorSelection[];
  onChangeSelections: (selections: DigitalHumanCreateConnectorSelection[]) => void;
  disabled?: boolean;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const candidatesQuery = useDigitalHumanConnectorCandidates({ limit: 512 });
  const catalogQuery = useConnectorCatalog();
  const connector = useMemo(() => {
    const item = connectorCatalogItems(catalogQuery.data?.data).find(
      value => value.id === connectorId
    );
    return item ? localizeConnectorCatalogItem(item, locale) : undefined;
  }, [catalogQuery.data?.data, connectorId, locale]);
  const servers = (candidatesQuery.data?.data.items ?? []).filter(
    candidate => candidate.parent_resource_id === connectorId
  );
  const selected = selections.find(item => item.candidate.parent_resource_id === connectorId);
  const [preset, setPreset] = useState<ServerCapabilityPreset>(
    selected ? serverPresetOf(selected) : 'manage'
  );
  const [readOnlyAuto, setReadOnlyAuto] = useState(
    selected ? connectorApprovalPolicy(selected.binding) === 'read_only_auto' : true
  );
  const [addOpen, setAddOpen] = useState(false);
  const [setupFailed, setSetupFailed] = useState(false);
  const [riskyPreset, setRiskyPreset] = useState(false);

  const apply = (
    candidateId: string,
    nextPreset = preset,
    nextReadOnlyAuto = readOnlyAuto,
    pool = servers
  ) => {
    const candidate = pool.find(value => value.resource_id === candidateId);
    if (!candidate || !createConnectorCandidateSelectable(candidate)) return;
    onChangeSelections([
      ...selectServerConnection(
        selections,
        candidate,
        nextPreset,
        nextReadOnlyAuto ? 'read_only_auto' : 'always_ask'
      ),
    ]);
  };

  const changePreset = (next: ServerCapabilityPreset) => {
    if (next === 'full' && preset !== 'full') {
      setRiskyPreset(true);
      return;
    }
    setPreset(next);
    if (selected) apply(selected.candidate.resource_id, next);
  };

  const ready = async (connection: PersonalConnection) => {
    setSetupFailed(false);
    const result = await candidatesQuery.refetch();
    const pool = (result.data?.data.items ?? []).filter(
      candidate => candidate.parent_resource_id === connectorId
    );
    if (
      !pool.some(
        value => value.resource_id === connection.id && createConnectorCandidateSelectable(value)
      )
    ) {
      setSetupFailed(true);
      return;
    }
    apply(connection.id, preset, readOnlyAuto, pool);
    setAddOpen(false);
  };

  const copyPrepare = async () => {
    try {
      await navigator.clipboard.writeText(PREPARE_COMMANDS);
      toast.success(t('createFlow.server.copied'));
    } catch {
      toast.error(t('createFlow.server.copyFailed'));
    }
  };

  return (
    <section className="space-y-4 rounded-xl border border-primary/30 bg-primary/[0.02] p-4">
      <div className="flex items-start gap-3">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-primary">
          <Server className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="font-medium">{t('createFlow.server.title')}</h2>
            {selected ? (
              <Badge variant="subtle">
                <Check className="mr-1 size-3" />
                {t('createFlow.server.connected', { name: selected.candidate.display_name })}
              </Badge>
            ) : (
              <Badge variant="outline">{t('createFlow.server.required')}</Badge>
            )}
          </div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('createFlow.server.description')}
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <p className="text-xs font-medium">{t('createFlow.server.chooseLabel')}</p>
        {candidatesQuery.isLoading ? (
          <p className="text-xs text-muted-foreground">{t('createFlow.server.loading')}</p>
        ) : null}
        <div className="grid gap-2 sm:grid-cols-2">
          {servers.map(server => {
            const usable = createConnectorCandidateSelectable(server);
            const isSelected = selected?.candidate.resource_id === server.resource_id;
            return (
              <button
                key={server.resource_id}
                type="button"
                disabled={disabled || !usable}
                onClick={() => apply(server.resource_id)}
                className={`rounded-lg border p-3 text-left text-sm transition-colors motion-reduce:transition-none disabled:opacity-50 ${
                  isSelected ? 'border-primary bg-primary/5' : 'hover:bg-muted/30'
                }`}
              >
                <span className="block truncate font-medium">{server.display_name}</span>
                <span className="mt-0.5 block text-xs text-muted-foreground">
                  {isSelected
                    ? t('createFlow.server.inUse')
                    : usable
                      ? t('createFlow.server.use')
                      : t('bindings.connectors.unavailable')}
                </span>
              </button>
            );
          })}
          <button
            type="button"
            disabled={disabled || !connector}
            onClick={() => setAddOpen(true)}
            className="flex items-center gap-2 rounded-lg border border-dashed p-3 text-left text-sm hover:border-primary/40 hover:bg-primary/[0.03] disabled:opacity-50"
          >
            <Plus className="size-4 text-primary" />
            <span>
              <span className="block font-medium">{t('createFlow.server.add')}</span>
              <span className="block text-xs text-muted-foreground">
                {t('createFlow.server.addHint')}
              </span>
            </span>
          </button>
        </div>
      </div>

      <div className="space-y-2">
        <p className="text-xs font-medium">{t('createFlow.server.presetLabel')}</p>
        <RadioGroup
          value={preset}
          onValueChange={value => changePreset(value as ServerCapabilityPreset)}
          className="gap-2"
        >
          {PRESETS.map(value => (
            <Radio
              key={value}
              value={value}
              disabled={disabled}
              label={t(`createFlow.server.presets.${value}.label`)}
              description={t(`createFlow.server.presets.${value}.description`)}
            />
          ))}
        </RadioGroup>
      </div>

      <label className="flex items-start gap-2 text-sm">
        <Checkbox
          checked={readOnlyAuto}
          disabled={disabled}
          onCheckedChange={value => {
            const next = value === true;
            setReadOnlyAuto(next);
            if (selected) apply(selected.candidate.resource_id, preset, next);
          }}
        />
        <span>
          <span className="block font-medium">{t('createFlow.server.readOnlyAuto')}</span>
          <span className="mt-0.5 block text-xs leading-5 text-muted-foreground">
            {t('createFlow.server.readOnlyAutoHint')}
          </span>
        </span>
      </label>

      {addOpen && connector ? (
        <Dialog open onOpenChange={open => (open ? undefined : setAddOpen(false))}>
          <DialogContent size="md">
            <DialogHeader>
              <DialogTitle>{t('createFlow.server.addTitle')}</DialogTitle>
              <DialogDescription>{t('createFlow.server.addDescription')}</DialogDescription>
            </DialogHeader>
            <DialogBody className="space-y-4">
              <details className="rounded-lg border bg-muted/20 p-3 text-xs leading-5">
                <summary className="cursor-pointer font-medium">
                  {t('createFlow.server.prepareTitle')}
                </summary>
                <ul className="mt-2 list-disc space-y-1 pl-4 text-muted-foreground">
                  <li>{t('createFlow.server.prepareAccount')}</li>
                  <li>{t('createFlow.server.prepareNetwork')}</li>
                  <li>{t('createFlow.server.prepareSudo')}</li>
                  <li>{t('createFlow.server.prepareIdentity')}</li>
                </ul>
                <div className="mt-3 rounded-md border bg-background">
                  <div className="flex items-center justify-between border-b px-2 py-1">
                    <span className="text-muted-foreground">
                      {t('createFlow.server.prepareCommands')}
                    </span>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => void copyPrepare()}
                    >
                      <ClipboardCopy className="size-3.5" />
                      {t('createFlow.server.copy')}
                    </Button>
                  </div>
                  <pre className="overflow-x-auto whitespace-pre p-2 font-mono text-[11px]">
                    {PREPARE_COMMANDS}
                  </pre>
                </div>
              </details>
              {setupFailed ? (
                <p role="alert" className="text-sm text-destructive">
                  {t('bindings.connectors.setupNotAdded')}
                </p>
              ) : null}
              <ConnectorKeySetupPanel
                connector={connector}
                embedded
                onReady={connection => void ready(connection)}
              />
            </DialogBody>
          </DialogContent>
        </Dialog>
      ) : null}

      <ConfirmDialog
        open={riskyPreset}
        onOpenChange={open => {
          if (!open) setRiskyPreset(false);
        }}
        variant="danger"
        title={t('createFlow.server.fullRiskTitle')}
        description={t('bindings.connectors.riskyOperationDescription')}
        confirmText={t('bindings.connectors.riskyOperationConfirm')}
        cancelText={t('common.cancel')}
        onConfirm={() => {
          setRiskyPreset(false);
          setPreset('full');
          if (selected) apply(selected.candidate.resource_id, 'full');
        }}
      />
    </section>
  );
}
