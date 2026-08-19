'use client';

import { useDeferredValue, useEffect, useMemo, useState, type ComponentType } from 'react';
import {
  AlertCircle,
  BookOpen,
  Database,
  Plug,
  SearchX,
  Table2,
  Trash2,
  Workflow,
  Wrench,
} from 'lucide-react';
import { toast } from 'sonner';
import { ResourceBindingCard, ResourceBindingSection } from '@/components/common/resource-binding';
import {
  ResourceSelectionCardIcon,
  ResourceSelectionDialog,
  ResourceSelectionEmptyState,
  ResourceSelectionGrid,
  ResourceSelectionSkeleton,
} from '@/components/common/resource-selection-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Separator } from '@/components/ui/separator';
import {
  useDigitalHumanBindingCandidates,
  useDigitalHumanDraftBindings,
  useDigitalHumanMutations,
} from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  DigitalHumanBindingType,
  DigitalHumanResourceBinding,
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
} from '@/services/types/digital-human';

type BindingSectionKey = 'skills' | 'knowledge' | 'databases' | 'workflows' | 'integrations';
type AccessMode = DigitalHumanResourceBindingInput['access_mode'];

interface BindingSectionDefinition {
  key: BindingSectionKey;
  title: string;
  help: string;
  empty: string;
  add: string;
  pickerTitle: string;
  types: DigitalHumanBindingType[];
  icon: ComponentType<{ className?: string }>;
}

export function DigitalHumanResourceBindings({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const query = useDigitalHumanDraftBindings(id);
  const mutations = useDigitalHumanMutations(id);
  const [items, setItems] = useState<DigitalHumanResourceBindingInput[]>([]);
  const [pendingItems, setPendingItems] = useState<DigitalHumanResourceBindingInput[]>([]);
  const [activeSection, setActiveSection] = useState<BindingSectionKey | null>(null);
  const [type, setType] = useState<DigitalHumanBindingType>('skill');
  const [parentId, setParentId] = useState('');
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search.trim());
  const [openSections, setOpenSections] = useState<Record<BindingSectionKey, boolean>>({
    skills: true,
    knowledge: true,
    databases: true,
    workflows: true,
    integrations: true,
  });
  const canUpdate = permissions.hasPermission('digital_human.update');

  const sections: BindingSectionDefinition[] = useMemo(
    () => [
      {
        key: 'skills',
        title: t('bindings.sections.skills.title'),
        help: t('bindings.sections.skills.help'),
        empty: t('bindings.sections.skills.empty'),
        add: t('bindings.sections.skills.add'),
        pickerTitle: t('bindings.sections.skills.pickerTitle'),
        types: ['skill'],
        icon: Wrench,
      },
      {
        key: 'knowledge',
        title: t('bindings.sections.knowledge.title'),
        help: t('bindings.sections.knowledge.help'),
        empty: t('bindings.sections.knowledge.empty'),
        add: t('bindings.sections.knowledge.add'),
        pickerTitle: t('bindings.sections.knowledge.pickerTitle'),
        types: ['knowledge_dataset'],
        icon: BookOpen,
      },
      {
        key: 'databases',
        title: t('bindings.sections.databases.title'),
        help: t('bindings.sections.databases.help'),
        empty: t('bindings.sections.databases.empty'),
        add: t('bindings.sections.databases.add'),
        pickerTitle: t('bindings.sections.databases.pickerTitle'),
        types: ['database', 'database_table'],
        icon: Database,
      },
      {
        key: 'workflows',
        title: t('bindings.sections.workflows.title'),
        help: t('bindings.sections.workflows.help'),
        empty: t('bindings.sections.workflows.empty'),
        add: t('bindings.sections.workflows.add'),
        pickerTitle: t('bindings.sections.workflows.pickerTitle'),
        types: ['workflow'],
        icon: Workflow,
      },
      {
        key: 'integrations',
        title: t('bindings.sections.integrations.title'),
        help: t('bindings.sections.integrations.help'),
        empty: t('bindings.sections.integrations.empty'),
        add: t('bindings.sections.integrations.add'),
        pickerTitle: t('bindings.sections.integrations.pickerTitle'),
        types: ['integration_connection'],
        icon: Plug,
      },
    ],
    [t]
  );
  const activeDefinition = sections.find(section => section.key === activeSection);
  const dialogOpen = Boolean(activeDefinition);

  const parentCandidatesQuery = useDigitalHumanBindingCandidates(
    id,
    { type: 'database', limit: 100 },
    dialogOpen && type === 'database_table'
  );
  const candidatesQuery = useDigitalHumanBindingCandidates(
    id,
    {
      type,
      query: deferredSearch || undefined,
      parent_resource_id: type === 'database_table' ? parentId || undefined : undefined,
      limit: 100,
    },
    dialogOpen
  );
  const candidates = candidatesQuery.data?.data?.items ?? [];

  useEffect(() => {
    const serverItems = query.data?.data?.items;
    if (!serverItems) return;
    setItems(serverItems.map(toBindingInput));
  }, [query.data?.data?.items]);

  useEffect(() => {
    setSearch('');
    setParentId('');
  }, [type]);

  const openPicker = (section: BindingSectionDefinition) => {
    setPendingItems(items.map(item => ({ ...item, metadata: { ...item.metadata } })));
    setType(section.types[0]);
    setSearch('');
    setParentId('');
    setActiveSection(section.key);
  };

  const closePicker = () => {
    setActiveSection(null);
    setSearch('');
    setParentId('');
  };

  const persist = async (nextItems: DigitalHumanResourceBindingInput[]) => {
    const lockVersion = query.data?.data?.draft_lock_version;
    if (lockVersion == null) return false;
    try {
      await mutations.replaceBindings.mutateAsync({
        digitalHumanId: id,
        expectedDraftLockVersion: lockVersion,
        items: nextItems,
      });
      setItems(nextItems.map(item => ({ ...item, metadata: { ...item.metadata } })));
      return true;
    } catch {
      return false;
    }
  };

  const remove = async (binding: DigitalHumanResourceBindingInput) => {
    const nextItems = items.filter(item => bindingKey(item) !== bindingKey(binding));
    if (!(await persist(nextItems))) return;
    toast.success(t('bindings.saved'));
  };

  const toggleCandidate = (candidate: DigitalHumanResourceCandidate, checked: boolean) => {
    setPendingItems(current => {
      const key = resourceKey(candidate);
      const existing = current.find(item => resourceKey(item) === key);
      const withoutCandidate = current.filter(item => resourceKey(item) !== key);
      if (!checked) return withoutCandidate;
      return [
        ...withoutCandidate,
        candidateToBinding(
          candidate,
          existing?.access_mode ?? candidate.access_modes[0] ?? defaultAccessMode(candidate.type)
        ),
      ];
    });
  };

  const changeCandidateAccess = (candidate: DigitalHumanResourceCandidate, mode: AccessMode) => {
    setPendingItems(current => [
      ...current.filter(item => resourceKey(item) !== resourceKey(candidate)),
      candidateToBinding(candidate, mode),
    ]);
  };

  const saveSelection = async () => {
    if (!(await persist(pendingItems))) return;
    closePicker();
    toast.success(t('bindings.saved'));
  };

  const pendingSectionCount = activeDefinition
    ? pendingItems.filter(item => activeDefinition.types.includes(item.type)).length
    : 0;
  const selectionChanged = !bindingsEqual(items, pendingItems);

  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle>{t('bindings.title')}</CardTitle>
        <CardDescription>{t('bindings.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {sections.map((section, index) => {
          const sectionItems = items.filter(item => section.types.includes(item.type));
          return (
            <div key={section.key}>
              {index > 0 ? <Separator className="mb-5 h-px" /> : null}
              <ResourceBindingSection
                id={`digital-human-bindings-${section.key}`}
                title={section.title}
                open={openSections[section.key]}
                count={sectionItems.length}
                countLabel={t('bindings.selectedCount', { count: sectionItems.length })}
                addLabel={section.add}
                helpText={section.help}
                emptyText={section.empty}
                isLoading={query.isLoading}
                onToggle={() =>
                  setOpenSections(current => ({
                    ...current,
                    [section.key]: !current[section.key],
                  }))
                }
                onAdd={() => openPicker(section)}
                readOnly={!canUpdate || mutations.replaceBindings.isPending}
              >
                <div className="space-y-2">
                  {sectionItems.map(item => (
                    <BindingCard
                      key={bindingKey(item)}
                      item={item}
                      typeLabel={bindingTypeLabel(item.type, t)}
                      accessLabel={t(`bindings.accessModes.${item.access_mode}`)}
                      removeLabel={t('bindings.removeNamed', {
                        name: item.display_name || item.resource_id,
                      })}
                      technicalDetailsLabel={t('common.technicalDetails')}
                      onRemove={() => void remove(item)}
                      disabled={!canUpdate || mutations.replaceBindings.isPending}
                    />
                  ))}
                </div>
              </ResourceBindingSection>
            </div>
          );
        })}
      </CardContent>

      {activeDefinition ? (
        <ResourceSelectionDialog
          open
          title={activeDefinition.pickerTitle}
          description={t('bindings.pickerDescription')}
          selectedCountLabel={t('bindings.selectedCount', { count: pendingSectionCount })}
          search={search}
          searchPlaceholder={t('bindings.searchPlaceholder')}
          isSearching={candidatesQuery.isFetching && !candidatesQuery.isLoading}
          toolbar={
            activeDefinition.key === 'databases' ? (
              <DatabasePickerToolbar
                type={type}
                parentId={parentId}
                parentCandidates={parentCandidatesQuery.data?.data?.items ?? []}
                typeLabel={t('fields.resourceType')}
                parentLabel={t('fields.parentResource')}
                databaseLabel={t('bindings.types.database')}
                tableLabel={t('bindings.types.database_table')}
                selectDatabaseLabel={t('bindings.selectDatabase')}
                onChangeType={setType}
                onChangeParent={setParentId}
              />
            ) : undefined
          }
          footer={
            <>
              <Button variant="outline" onClick={closePicker}>
                {t('common.cancel')}
              </Button>
              <Button
                onClick={() => void saveSelection()}
                disabled={!selectionChanged || mutations.replaceBindings.isPending}
              >
                {mutations.replaceBindings.isPending
                  ? t('bindings.savingSelection')
                  : t('bindings.saveSelection')}
              </Button>
            </>
          }
          onOpenChange={open => {
            if (!open) closePicker();
          }}
          onChangeSearch={setSearch}
        >
          <CandidateContent
            type={type}
            parentId={parentId}
            search={deferredSearch}
            candidates={candidates}
            selectedItems={pendingItems}
            loading={candidatesQuery.isLoading}
            error={candidatesQuery.isError}
            emptyTitle={t('bindings.noCandidates')}
            emptyDescription={
              deferredSearch ? t('bindings.noSearchResults') : t('bindings.noCandidatesDescription')
            }
            selectDatabaseTitle={t('bindings.selectDatabaseTitle')}
            selectDatabaseDescription={t('bindings.selectDatabaseDescription')}
            unavailableLabel={t('bindings.unavailable')}
            readyLabel={t('bindings.ready')}
            noDescriptionLabel={t('bindings.noDescription')}
            retryLabel={t('bindings.retry')}
            accessLabels={{
              read: t('bindings.accessModes.read'),
              write: t('bindings.accessModes.write'),
              execute: t('bindings.accessModes.execute'),
            }}
            onToggle={toggleCandidate}
            onChangeAccess={changeCandidateAccess}
            onRetry={() => void candidatesQuery.refetch()}
          />
        </ResourceSelectionDialog>
      ) : null}
    </Card>
  );
}

function BindingCard({
  item,
  typeLabel,
  accessLabel,
  removeLabel,
  technicalDetailsLabel,
  onRemove,
  disabled,
}: {
  item: DigitalHumanResourceBindingInput;
  typeLabel: string;
  accessLabel: string;
  removeLabel: string;
  technicalDetailsLabel: string;
  onRemove: () => void;
  disabled: boolean;
}) {
  const Icon = bindingIcon(item.type);
  const description = metadataString(item.metadata, 'description');
  return (
    <ResourceBindingCard
      icon={<Icon className="size-4" />}
      title={item.display_name || item.resource_id}
      description={description}
      status={<Badge variant="subtle">{accessLabel}</Badge>}
      action={
        <Button
          type="button"
          variant="ghost"
          size="sm"
          isIcon
          className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
          aria-label={removeLabel}
          title={removeLabel}
          disabled={disabled}
          onClick={onRemove}
        >
          <Trash2 className="size-4" />
        </Button>
      }
    >
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>{typeLabel}</span>
        {item.parent_resource_id ? <span>· {shortID(item.parent_resource_id)}</span> : null}
      </div>
      <details className="mt-2 text-xs text-muted-foreground">
        <summary className="cursor-pointer">{technicalDetailsLabel}</summary>
        <code className="mt-1 block break-all">{item.resource_revision}</code>
      </details>
    </ResourceBindingCard>
  );
}

function DatabasePickerToolbar({
  type,
  parentId,
  parentCandidates,
  typeLabel,
  parentLabel,
  databaseLabel,
  tableLabel,
  selectDatabaseLabel,
  onChangeType,
  onChangeParent,
}: {
  type: DigitalHumanBindingType;
  parentId: string;
  parentCandidates: DigitalHumanResourceCandidate[];
  typeLabel: string;
  parentLabel: string;
  databaseLabel: string;
  tableLabel: string;
  selectDatabaseLabel: string;
  onChangeType: (type: DigitalHumanBindingType) => void;
  onChangeParent: (id: string) => void;
}) {
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-2 sm:flex-row sm:justify-end">
      <label className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
        <span className="shrink-0">{typeLabel}</span>
        <select
          className="h-9 min-w-36 rounded-md border bg-background px-3 text-sm text-foreground"
          value={type}
          onChange={event => onChangeType(event.target.value as DigitalHumanBindingType)}
        >
          <option value="database">{databaseLabel}</option>
          <option value="database_table">{tableLabel}</option>
        </select>
      </label>
      {type === 'database_table' ? (
        <label className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <span className="shrink-0">{parentLabel}</span>
          <select
            className="h-9 min-w-44 max-w-64 rounded-md border bg-background px-3 text-sm text-foreground"
            value={parentId}
            onChange={event => onChangeParent(event.target.value)}
          >
            <option value="">{selectDatabaseLabel}</option>
            {parentCandidates
              .filter(candidate => candidate.available)
              .map(candidate => (
                <option key={candidate.resource_id} value={candidate.resource_id}>
                  {candidate.display_name}
                </option>
              ))}
          </select>
        </label>
      ) : null}
    </div>
  );
}

function CandidateContent({
  type,
  parentId,
  search,
  candidates,
  selectedItems,
  loading,
  error,
  emptyTitle,
  emptyDescription,
  selectDatabaseTitle,
  selectDatabaseDescription,
  unavailableLabel,
  readyLabel,
  noDescriptionLabel,
  retryLabel,
  accessLabels,
  onToggle,
  onChangeAccess,
  onRetry,
}: {
  type: DigitalHumanBindingType;
  parentId: string;
  search: string;
  candidates: DigitalHumanResourceCandidate[];
  selectedItems: DigitalHumanResourceBindingInput[];
  loading: boolean;
  error: boolean;
  emptyTitle: string;
  emptyDescription: string;
  selectDatabaseTitle: string;
  selectDatabaseDescription: string;
  unavailableLabel: string;
  readyLabel: string;
  noDescriptionLabel: string;
  retryLabel: string;
  accessLabels: Record<AccessMode, string>;
  onToggle: (candidate: DigitalHumanResourceCandidate, checked: boolean) => void;
  onChangeAccess: (candidate: DigitalHumanResourceCandidate, mode: AccessMode) => void;
  onRetry: () => void;
}) {
  if (type === 'database_table' && !parentId) {
    return (
      <ResourceSelectionEmptyState
        icon={<Database />}
        title={selectDatabaseTitle}
        description={selectDatabaseDescription}
      />
    );
  }
  if (loading) return <ResourceSelectionSkeleton />;
  if (error) {
    return (
      <ResourceSelectionEmptyState
        icon={<AlertCircle />}
        title={emptyTitle}
        description={emptyDescription}
        action={
          <Button variant="outline" size="sm" onClick={onRetry}>
            {retryLabel}
          </Button>
        }
      />
    );
  }
  if (candidates.length === 0) {
    return (
      <ResourceSelectionEmptyState
        icon={search ? <SearchX /> : <BindingTypeIcon type={type} />}
        title={emptyTitle}
        description={emptyDescription}
        variant={search ? 'search' : 'resource'}
      />
    );
  }

  return (
    <ResourceSelectionGrid>
      {candidates.map(candidate => {
        const selected = selectedItems.find(item => resourceKey(item) === resourceKey(candidate));
        const Icon = bindingIcon(candidate.type);
        return (
          <div
            key={resourceKey(candidate)}
            role="checkbox"
            aria-checked={Boolean(selected)}
            aria-disabled={!candidate.available}
            tabIndex={candidate.available ? 0 : -1}
            className={cn(
              'flex min-h-36 cursor-pointer flex-col rounded-lg border bg-background p-4 text-left transition-colors',
              selected && 'border-primary bg-primary/5 ring-1 ring-primary/20',
              candidate.available ? 'hover:border-primary/40' : 'cursor-not-allowed opacity-60'
            )}
            onClick={() => candidate.available && onToggle(candidate, !selected)}
            onKeyDown={event => {
              if (!candidate.available || (event.key !== 'Enter' && event.key !== ' ')) return;
              event.preventDefault();
              onToggle(candidate, !selected);
            }}
          >
            <div className="flex items-start gap-3">
              <ResourceSelectionCardIcon>
                <Icon />
              </ResourceSelectionCardIcon>
              <div className="min-w-0 flex-1">
                <div className="line-clamp-2 text-sm font-medium">{candidate.display_name}</div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {candidate.available ? readyLabel : unavailableLabel}
                </div>
              </div>
              <Checkbox
                checked={Boolean(selected)}
                disabled={!candidate.available}
                aria-label={candidate.display_name}
                onClick={event => event.stopPropagation()}
                onCheckedChange={checked => onToggle(candidate, checked === true)}
              />
            </div>
            <p className="mt-3 line-clamp-2 text-xs leading-5 text-muted-foreground">
              {candidate.description || noDescriptionLabel}
            </p>
            {selected && candidate.access_modes.length > 1 ? (
              <label
                className="mt-auto flex items-center justify-between gap-2 border-t pt-3 text-xs text-muted-foreground"
                onClick={event => event.stopPropagation()}
              >
                <span>{accessLabels[selected.access_mode]}</span>
                <select
                  className="h-8 rounded-md border bg-background px-2 text-xs text-foreground"
                  value={selected.access_mode}
                  aria-label={`${candidate.display_name}: ${accessLabels[selected.access_mode]}`}
                  onChange={event => onChangeAccess(candidate, event.target.value as AccessMode)}
                >
                  {candidate.access_modes.map(mode => (
                    <option key={mode} value={mode}>
                      {accessLabels[mode]}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
          </div>
        );
      })}
    </ResourceSelectionGrid>
  );
}

function BindingTypeIcon({ type }: { type: DigitalHumanBindingType }) {
  const Icon = bindingIcon(type);
  return <Icon />;
}

function bindingIcon(type: DigitalHumanBindingType) {
  if (type === 'skill') return Wrench;
  if (type === 'knowledge_dataset') return BookOpen;
  if (type === 'database') return Database;
  if (type === 'database_table') return Table2;
  if (type === 'workflow') return Workflow;
  return Plug;
}

function bindingTypeLabel(
  type: DigitalHumanBindingType,
  t: ReturnType<typeof useT<'digitalHumans'>>
) {
  return t(`bindings.types.${type}`);
}

function defaultAccessMode(type: DigitalHumanBindingType): AccessMode {
  if (type === 'knowledge_dataset' || type === 'database' || type === 'database_table') {
    return 'read';
  }
  return 'execute';
}

function candidateToBinding(
  candidate: DigitalHumanResourceCandidate,
  accessMode: AccessMode
): DigitalHumanResourceBindingInput {
  return {
    type: candidate.type,
    resource_id: candidate.resource_id,
    parent_resource_id: candidate.parent_resource_id,
    display_name: candidate.display_name,
    access_mode: accessMode,
    resource_revision: candidate.resource_revision,
    metadata: { ...candidate.metadata },
  };
}

function toBindingInput(item: DigitalHumanResourceBinding): DigitalHumanResourceBindingInput {
  return {
    type: item.binding_type,
    resource_id: item.resource_id,
    parent_resource_id: item.parent_resource_id,
    display_name: item.display_name,
    access_mode: item.access_mode,
    resource_revision: item.resource_revision,
    credential_ref: item.credential_ref,
    metadata: { ...(item.metadata ?? {}) },
  };
}

function resourceKey(item: {
  type: DigitalHumanBindingType;
  resource_id: string;
  parent_resource_id?: string;
}) {
  return `${item.type}:${item.parent_resource_id ?? ''}:${item.resource_id}`;
}

function bindingKey(item: DigitalHumanResourceBindingInput) {
  return `${resourceKey(item)}:${item.access_mode}`;
}

function bindingsEqual(
  left: DigitalHumanResourceBindingInput[],
  right: DigitalHumanResourceBindingInput[]
) {
  const normalize = (items: DigitalHumanResourceBindingInput[]) =>
    items
      .map(item =>
        [
          bindingKey(item),
          item.access_mode,
          item.resource_revision,
          item.credential_ref ?? '',
          JSON.stringify(item.metadata ?? {}),
        ].join('|')
      )
      .sort();
  return JSON.stringify(normalize(left)) === JSON.stringify(normalize(right));
}

function metadataString(metadata: Record<string, unknown>, key: string) {
  const value = metadata[key];
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

function shortID(value: string) {
  if (value.length <= 13) return value;
  return `${value.slice(0, 8)}…${value.slice(-4)}`;
}
