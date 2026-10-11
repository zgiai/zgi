'use client';

import {
  useDeferredValue,
  useEffect,
  useMemo,
  useState,
  type ComponentType,
  type ReactNode,
} from 'react';
import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  BookOpen,
  Check,
  Database,
  Plus,
  PlugZap,
  Puzzle,
  RefreshCw,
  SearchX,
  Table2,
  Trash2,
  Workflow,
} from 'lucide-react';
import { toast } from 'sonner';
import {
  AgentWorkflowTypeBadge,
  AgentWorkflowTypeIcon,
} from '@/components/agents/agent-runtime/workflow-type-display';
import { ResourceBindingCard } from '@/components/common/resource-binding';
import { DigitalHumanConnectorSection } from '@/components/digital-human/digital-human-connector-section';
import { ZgiMarketToolsSection } from '@/components/zgi-market/zgi-market-tools-section';
import { digitalHumanBindingInput } from '@/components/digital-human/digital-human-connector-model';
import { useDigitalHumanDraftStore } from '@/components/digital-human/digital-human-draft-store';
import {
  DigitalHumanSkillBindingCard,
  DigitalHumanSkillDialogBody,
  DigitalHumanSkillSourceSelect,
  filterDigitalHumanSkills,
  type DigitalHumanSkillSourceFilter,
} from '@/components/digital-human/digital-human-skill-picker';
import {
  ResourceSelectionCardIcon,
  ResourceSelectionDialog,
  ResourceSelectionEmptyState,
  ResourceSelectionGrid,
  ResourceSelectionSkeleton,
} from '@/components/common/resource-selection-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import {
  useDigitalHumanBindingCandidates,
  useDigitalHumanDraftBindings,
  useDigitalHumanMutations,
} from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  DigitalHumanBindingType,
  DigitalHumanResourceBinding,
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
} from '@/services/types/digital-human';
import { formatDate } from '@/utils/format';

type BindingSectionKey = 'skills' | 'knowledge' | 'databases' | 'workflows';
type AccessMode = DigitalHumanResourceBindingInput['access_mode'];
type DatabasePickerStep = 'databases' | 'tables';

interface BindingSectionDefinition {
  key: BindingSectionKey;
  title: string;
  empty: string;
  add: string;
  pickerTitle: string;
  types: DigitalHumanBindingType[];
  icon: ComponentType<{ className?: string }>;
}

export function DigitalHumanResourceBindings({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const permissions = useAccountPermissions();
  const query = useDigitalHumanDraftBindings(id);
  const mutations = useDigitalHumanMutations(id);
  const [items, setItems] = useState<DigitalHumanResourceBindingInput[]>([]);
  const [pendingItems, setPendingItems] = useState<DigitalHumanResourceBindingInput[]>([]);
  const [activeSection, setActiveSection] = useState<BindingSectionKey | null>(null);
  const [type, setType] = useState<DigitalHumanBindingType>('skill');
  const [parentId, setParentId] = useState('');
  const [databaseStep, setDatabaseStep] = useState<DatabasePickerStep>('databases');
  const [selectedDatabaseIds, setSelectedDatabaseIds] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const [skillSourceFilter, setSkillSourceFilter] =
    useState<DigitalHumanSkillSourceFilter>('all');
  const deferredSearch = useDeferredValue(search.trim());
  const versionPreview = useDigitalHumanDraftStore(state => state.versionPreviews[id]);
  const canUpdate = permissions.hasPermission('digital_human.update');
  const canConfigure = canUpdate && !versionPreview;
  const displayedItems = versionPreview?.bindings?.map(toBindingInput) ?? items;
  const isPreviewLoading = Boolean(versionPreview && !versionPreview.bindings);

  const sections: BindingSectionDefinition[] = useMemo(
    () => [
      {
        key: 'skills',
        title: t('bindings.sections.skills.title'),
        empty: t('bindings.sections.skills.empty'),
        add: t('bindings.sections.skills.add'),
        pickerTitle: t('bindings.sections.skills.pickerTitle'),
        types: ['skill'],
        icon: Puzzle,
      },
      {
        key: 'knowledge',
        title: t('bindings.sections.knowledge.title'),
        empty: t('bindings.sections.knowledge.empty'),
        add: t('bindings.sections.knowledge.add'),
        pickerTitle: t('bindings.sections.knowledge.pickerTitle'),
        types: ['knowledge_dataset'],
        icon: BookOpen,
      },
      {
        key: 'databases',
        title: t('bindings.sections.databases.title'),
        empty: t('bindings.sections.databases.empty'),
        add: t('bindings.sections.databases.add'),
        pickerTitle: t('bindings.sections.databases.pickerTitle'),
        types: ['database', 'database_table'],
        icon: Database,
      },
      {
        key: 'workflows',
        title: t('bindings.sections.workflows.title'),
        empty: t('bindings.sections.workflows.empty'),
        add: t('bindings.sections.workflows.add'),
        pickerTitle: t('bindings.sections.workflows.pickerTitle'),
        types: ['workflow'],
        icon: Workflow,
      },
    ],
    [t]
  );
  const activeDefinition = sections.find(section => section.key === activeSection);
  const dialogOpen = Boolean(activeDefinition);

  const databaseCandidatesQuery = useDigitalHumanBindingCandidates(
    id,
    { type: 'database', limit: 100 },
    dialogOpen && activeDefinition?.key === 'databases'
  );
  const candidatesQuery = useDigitalHumanBindingCandidates(
    id,
    {
      type,
      query: type === 'skill' ? undefined : deferredSearch || undefined,
      parent_resource_id: type === 'database_table' ? parentId || undefined : undefined,
      limit: 100,
    },
    dialogOpen && !(activeDefinition?.key === 'databases' && databaseStep === 'databases')
  );
  const candidates = useMemo(() => {
    const availableCandidates =
      activeDefinition?.key === 'databases' && databaseStep === 'databases'
        ? (databaseCandidatesQuery.data?.data?.items ?? [])
        : (candidatesQuery.data?.data?.items ?? []);
    const normalizedSearch = deferredSearch.toLocaleLowerCase();
    if (type !== 'skill') {
      if (!normalizedSearch) return availableCandidates;
      return availableCandidates.filter(candidate =>
        [candidate.display_name, candidate.description, candidate.resource_id].some(value =>
          value?.toLocaleLowerCase().includes(normalizedSearch)
        )
      );
    }

    return filterDigitalHumanSkills(availableCandidates, {
      search: deferredSearch,
      source: skillSourceFilter,
      locale,
    });
  }, [
    activeDefinition?.key,
    candidatesQuery.data?.data?.items,
    databaseCandidatesQuery.data?.data?.items,
    databaseStep,
    deferredSearch,
    locale,
    skillSourceFilter,
    type,
  ]);

  useEffect(() => {
    const serverItems = query.data?.data?.items;
    if (!serverItems) return;
    setItems(serverItems.map(toBindingInput));
  }, [query.data?.data?.items]);

  useEffect(() => {
    if (!versionPreview) return;
    setActiveSection(null);
    setSearch('');
    setSkillSourceFilter('all');
    setParentId('');
    setDatabaseStep('databases');
    setSelectedDatabaseIds([]);
  }, [versionPreview]);

  const openPicker = (section: BindingSectionDefinition) => {
    setPendingItems(items.map(item => ({ ...item, metadata: { ...item.metadata } })));
    setType(section.types[0]);
    setSearch('');
    setSkillSourceFilter('all');
    setParentId('');
    setDatabaseStep('databases');
    setSelectedDatabaseIds(
      section.key === 'databases'
        ? Array.from(
            new Set(
              items
                .filter(item => section.types.includes(item.type))
                .map(item =>
                  item.type === 'database'
                    ? item.resource_id
                    : item.parent_resource_id || item.resource_id
                )
            )
          )
        : []
    );
    setActiveSection(section.key);
  };

  const closePicker = () => {
    setActiveSection(null);
    setSearch('');
    setSkillSourceFilter('all');
    setParentId('');
    setDatabaseStep('databases');
    setSelectedDatabaseIds([]);
  };

  const persist = async (nextItems: DigitalHumanResourceBindingInput[]) => {
    const lockVersion = query.data?.data?.draft_lock_version;
    if (lockVersion == null) return false;
    try {
      await mutations.replaceBindings.mutateAsync({
        digitalHumanId: id,
        expectedDraftLockVersion: lockVersion,
        expectedDraftVersionId: query.data?.data?.draft_version_id,
        items: nextItems,
      });
      return true;
    } catch {
      return false;
    }
  };

  const remove = async (binding: DigitalHumanResourceBindingInput) => {
    const nextItems = items.filter(item => bindingKey(item) !== bindingKey(binding));
    if (!(await persist(nextItems))) return;
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

  const toggleDatabaseCandidate = (candidate: DigitalHumanResourceCandidate, checked: boolean) => {
    setSelectedDatabaseIds(current =>
      checked
        ? Array.from(new Set([...current, candidate.resource_id]))
        : current.filter(id => id !== candidate.resource_id)
    );
    if (!checked) {
      setPendingItems(current =>
        current.filter(
          item =>
            !(
              (item.type === 'database' && item.resource_id === candidate.resource_id) ||
              (item.type === 'database_table' && item.parent_resource_id === candidate.resource_id)
            )
        )
      );
    }
  };

  const openDatabaseTables = () => {
    const nextParent = selectedDatabaseIds.includes(parentId)
      ? parentId
      : selectedDatabaseIds[0] || '';
    setParentId(nextParent);
    setType('database_table');
    setSearch('');
    setDatabaseStep('tables');
  };

  const backToDatabases = () => {
    setType('database');
    setParentId('');
    setSearch('');
    setDatabaseStep('databases');
  };

  const saveSelection = async () => {
    if (activeDefinition?.key === 'databases') {
      const incompleteDatabase = selectedDatabaseIds.some(
        databaseId =>
          !pendingItems.some(
            item =>
              (item.type === 'database' && item.resource_id === databaseId) ||
              (item.type === 'database_table' && item.parent_resource_id === databaseId)
          )
      );
      if (incompleteDatabase) {
        toast.error(t('bindings.databases.tableRequired'));
        return;
      }
    }
    if (!(await persist(pendingItems))) return;
    closePicker();
  };

  const pendingSectionCount = activeDefinition
    ? activeDefinition.key === 'databases'
      ? selectedDatabaseIds.length
      : pendingItems.filter(item => activeDefinition.types.includes(item.type)).length
    : 0;
  const selectionChanged = !bindingsEqual(items, pendingItems);
  const activeCandidateQuery =
    activeDefinition?.key === 'databases' && databaseStep === 'databases'
      ? databaseCandidatesQuery
      : candidatesQuery;
  const databaseSelectionIncomplete =
    activeDefinition?.key === 'databases' &&
    databaseStep === 'tables' &&
    selectedDatabaseIds.some(
      databaseId =>
        !pendingItems.some(
          item =>
            (item.type === 'database' && item.resource_id === databaseId) ||
            (item.type === 'database_table' && item.parent_resource_id === databaseId)
        )
    );

  return (
    <div className="min-w-0">
      <div className="grid grid-cols-1 items-stretch gap-4">
        {sections.map(section => {
          const sectionItems = displayedItems.filter(item => section.types.includes(item.type));
          const SectionIcon = section.icon;
          return (
            <section
              key={section.key}
              id={`digital-human-bindings-${section.key}`}
              className="flex h-full min-h-[280px] max-h-[520px] min-w-0 scroll-mt-16 flex-col overflow-hidden rounded-xl border bg-card shadow-sm"
            >
              <div className="flex items-center justify-between gap-3 border-b bg-muted/15 px-3 py-3">
                <div className="flex min-w-0 items-center gap-2.5">
                  <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-primary">
                    <SectionIcon className="size-4" />
                  </span>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="font-medium">{section.title}</h3>
                      <Badge variant="subtle">
                        {t('bindings.selectedCount', { count: sectionItems.length })}
                      </Badge>
                    </div>
                  </div>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  isIcon
                  className="size-8 shrink-0"
                  aria-label={section.add}
                  disabled={!canConfigure || mutations.replaceBindings.isPending}
                  onClick={() => openPicker(section)}
                >
                  <Plus className="size-4" />
                </Button>
              </div>
              <div className="min-h-0 flex-1 overflow-y-auto p-3">
                {query.isLoading || isPreviewLoading ? (
                  <div className="space-y-2">
                    <div className="h-20 animate-pulse rounded-lg bg-muted" />
                    <div className="h-20 animate-pulse rounded-lg bg-muted" />
                  </div>
                ) : sectionItems.length === 0 ? (
                  <button
                    type="button"
                    className="flex min-h-full w-full flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 px-5 py-6 text-center transition-colors hover:border-primary/40 hover:bg-primary/[0.03] disabled:pointer-events-none"
                    disabled={!canConfigure || mutations.replaceBindings.isPending}
                    onClick={() => openPicker(section)}
                  >
                    <Plus className="mb-2 size-5 text-muted-foreground" />
                    <span className="text-sm font-medium text-foreground">{section.add}</span>
                    <span className="mt-1 text-xs leading-5 text-muted-foreground">
                      {section.empty}
                    </span>
                  </button>
                ) : section.key === 'databases' ? (
                  <DatabaseBindingCards
                    items={sectionItems}
                    disabled={!canConfigure || mutations.replaceBindings.isPending}
                    onRemove={item => void remove(item)}
                  />
                ) : (
                  <div className="grid grid-cols-1 gap-3" data-resource-binding-grid>
                    {sectionItems.map(item => (
                      <BindingCard
                        key={bindingKey(item)}
                        item={item}
                        locale={locale}
                        accessLabel={t(`bindings.accessModes.${item.access_mode}`)}
                        removeLabel={t('bindings.removeNamed', {
                          name: item.display_name || item.resource_id,
                        })}
                        onRemove={() => void remove(item)}
                        disabled={!canConfigure || mutations.replaceBindings.isPending}
                      />
                    ))}
                  </div>
                )}
              </div>
            </section>
          );
        })}
        <DigitalHumanConnectorSection
          id={id}
          preview={versionPreview?.bindings}
          readOnly={!canConfigure}
        />
        <ZgiMarketToolsSection subjectType="digital_human" subjectId={id} readOnly={!canConfigure} />
      </div>

      {activeDefinition ? (
        <ResourceSelectionDialog
          open
          title={
            activeDefinition.key === 'databases' && databaseStep === 'tables'
              ? t('bindings.databases.tableDialogTitle')
              : activeDefinition.pickerTitle
          }
          description={
            activeDefinition.key === 'skills'
              ? t('bindings.skills.dialogDescription')
              : activeDefinition.key === 'knowledge'
                ? t('bindings.knowledge.dialogDescription')
                : activeDefinition.key === 'databases'
                  ? t(
                      databaseStep === 'databases'
                        ? 'bindings.databases.dialogDescription'
                        : 'bindings.databases.tableDialogDescription'
                    )
                  : t('bindings.workflows.dialogDescription')
          }
          selectedCountLabel={t('bindings.selectedCount', { count: pendingSectionCount })}
          search={search}
          searchPlaceholder={
            activeDefinition.key === 'skills'
              ? t('bindings.skills.searchPlaceholder')
              : activeDefinition.key === 'knowledge'
                ? t('bindings.knowledge.searchPlaceholder')
                : activeDefinition.key === 'databases'
                  ? t(
                      databaseStep === 'databases'
                        ? 'bindings.databases.searchDatabase'
                        : 'bindings.databases.searchTable'
                    )
                  : t('bindings.workflows.searchPlaceholder')
          }
          isSearching={
            type !== 'skill' && activeCandidateQuery.isFetching && !activeCandidateQuery.isLoading
          }
          toolbar={
            activeDefinition.key === 'skills' ? (
              <DigitalHumanSkillSourceSelect
                value={skillSourceFilter}
                onChange={setSkillSourceFilter}
              />
            ) : undefined
          }
          footer={
            <>
              {activeDefinition.key === 'databases' && databaseStep === 'tables' ? (
                <Button variant="ghost" className="mr-auto" onClick={backToDatabases}>
                  <ArrowLeft className="size-4" />
                  {t('bindings.databases.back')}
                </Button>
              ) : null}
              <Button
                variant={activeDefinition.key === 'skills' ? 'ghost' : 'outline'}
                onClick={closePicker}
              >
                {t('common.cancel')}
              </Button>
              <Button
                onClick={() => {
                  if (activeDefinition.key === 'databases' && databaseStep === 'databases') {
                    if (selectedDatabaseIds.length > 0) openDatabaseTables();
                    else void saveSelection();
                    return;
                  }
                  void saveSelection();
                }}
                disabled={
                  mutations.replaceBindings.isPending ||
                  (activeDefinition.key === 'databases' && databaseStep === 'databases'
                    ? selectedDatabaseIds.length === 0 && !selectionChanged
                    : !selectionChanged || Boolean(databaseSelectionIncomplete))
                }
              >
                {mutations.replaceBindings.isPending
                  ? t('bindings.savingSelection')
                  : activeDefinition.key === 'databases' && databaseStep === 'databases'
                    ? selectedDatabaseIds.length > 0
                      ? t('bindings.databases.next')
                      : t('bindings.saveSelection')
                    : activeDefinition.key === 'skills'
                      ? t('bindings.skills.save')
                      : t('bindings.saveSelection')}
                {activeDefinition.key === 'databases' &&
                databaseStep === 'databases' &&
                selectedDatabaseIds.length > 0 ? (
                  <ArrowRight className="size-4" />
                ) : null}
              </Button>
            </>
          }
          onOpenChange={open => {
            if (!open) closePicker();
          }}
          onChangeSearch={setSearch}
        >
          <ResourceCandidateContent
            section={activeDefinition.key}
            databaseStep={databaseStep}
            type={type}
            parentId={parentId}
            search={deferredSearch}
            candidates={candidates}
            selectedItems={pendingItems}
            selectedDatabaseIds={selectedDatabaseIds}
            databaseCandidates={databaseCandidatesQuery.data?.data?.items ?? []}
            loading={activeCandidateQuery.isLoading}
            error={activeCandidateQuery.isError}
            emptyTitle={t('bindings.noCandidates')}
            emptyDescription={
              deferredSearch ? t('bindings.noSearchResults') : t('bindings.noCandidatesDescription')
            }
            selectDatabaseTitle={t('bindings.selectDatabaseTitle')}
            selectDatabaseDescription={t('bindings.selectDatabaseDescription')}
            noDescriptionLabel={t('bindings.noDescription')}
            skillNoMatchLabel={t('bindings.skills.noMatch')}
            locale={locale}
            retryLabel={t('bindings.retry')}
            onToggle={toggleCandidate}
            onChangeAccess={changeCandidateAccess}
            onToggleDatabase={toggleDatabaseCandidate}
            onChangeDatabase={setParentId}
            onRetry={() => {
              void activeCandidateQuery.refetch();
            }}
          />
        </ResourceSelectionDialog>
      ) : null}
    </div>
  );
}

function BindingCard({
  item,
  locale,
  accessLabel,
  removeLabel,
  onRemove,
  disabled,
}: {
  item: DigitalHumanResourceBindingInput;
  locale: string;
  accessLabel: string;
  removeLabel: string;
  onRemove: () => void;
  disabled: boolean;
}) {
  // A bound skill uses the shared card, which the creation form renders too.
  if (item.type === 'skill') {
    return (
      <DigitalHumanSkillBindingCard
        item={item}
        locale={locale}
        removeLabel={removeLabel}
        onRemove={onRemove}
        disabled={disabled}
      />
    );
  }
  const Icon = bindingIcon(item.type);
  const workflowType = metadataString(item.metadata, 'agent_type');
  const title = item.display_name || item.resource_id;
  const description = metadataString(item.metadata, 'description');
  const status =
    item.type === 'workflow' ? (
      <AgentWorkflowTypeBadge agentType={workflowType} />
    ) : item.type === 'knowledge_dataset' ? undefined : (
      <Badge variant="subtle">{accessLabel}</Badge>
    );
  return (
    <ResourceBindingCard
      icon={
        item.type === 'workflow' ? (
          <AgentWorkflowTypeIcon agentType={workflowType} />
        ) : (
          <Icon className="size-4" />
        )
      }
      title={title}
      description={description}
      status={status}
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
    />
  );
}

function DatabaseBindingCards({
  items,
  disabled,
  onRemove,
}: {
  items: DigitalHumanResourceBindingInput[];
  disabled: boolean;
  onRemove: (item: DigitalHumanResourceBindingInput) => void;
}) {
  const t = useT('digitalHumans');
  const groups = useMemo(() => groupDatabaseBindings(items), [items]);
  return (
    <div
      className="grid grid-cols-1 gap-3 xl:grid-cols-2 2xl:grid-cols-3"
      data-resource-binding-grid
    >
      {groups.map(group => {
        const databaseBinding = group.database;
        const databaseName =
          databaseBinding?.display_name ||
          group.tables
            .map(
              table =>
                metadataString(table.metadata, 'database_name') ||
                metadataString(table.metadata, 'parent_display_name')
            )
            .find(Boolean) ||
          t('bindings.databaseFallback');
        return (
          <ResourceBindingCard
            key={group.id}
            icon={<Database className="size-4" />}
            title={databaseName}
            description={
              group.tables.length > 0
                ? t('bindings.databaseTableCount', { count: group.tables.length })
                : t('bindings.databaseDirectDescription')
            }
            status={
              databaseBinding ? (
                <Badge variant="subtle">
                  {t(`bindings.accessModes.${databaseBinding.access_mode}`)}
                </Badge>
              ) : undefined
            }
            action={
              databaseBinding ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  isIcon
                  className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                  aria-label={t('bindings.removeNamed', { name: databaseName })}
                  title={t('bindings.removeNamed', { name: databaseName })}
                  disabled={disabled}
                  onClick={() => onRemove(databaseBinding)}
                >
                  <Trash2 className="size-4" />
                </Button>
              ) : undefined
            }
          >
            {group.tables.length > 0 ? (
              <div className="space-y-2">
                {group.tables.map(table => {
                  const tableName = table.display_name || t('bindings.tableFallback');
                  return (
                    <div
                      key={bindingKey(table)}
                      className="flex min-h-10 items-center gap-2 rounded-md bg-muted/30 px-3 py-2"
                    >
                      <Table2 className="size-3.5 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate text-xs font-medium">
                        {tableName}
                      </span>
                      <Badge variant="subtle">
                        {t(`bindings.accessModes.${table.access_mode}`)}
                      </Badge>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        isIcon
                        className="size-7 shrink-0 text-muted-foreground hover:text-destructive"
                        aria-label={t('bindings.removeNamed', { name: tableName })}
                        title={t('bindings.removeNamed', { name: tableName })}
                        disabled={disabled}
                        onClick={() => onRemove(table)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  );
                })}
              </div>
            ) : null}
          </ResourceBindingCard>
        );
      })}
    </div>
  );
}

function groupDatabaseBindings(items: DigitalHumanResourceBindingInput[]) {
  const groups = new Map<
    string,
    {
      id: string;
      database?: DigitalHumanResourceBindingInput;
      tables: DigitalHumanResourceBindingInput[];
    }
  >();
  for (const item of items) {
    const id =
      item.type === 'database' ? item.resource_id : item.parent_resource_id || item.resource_id;
    const group = groups.get(id) ?? { id, tables: [] };
    if (item.type === 'database') group.database = item;
    else group.tables.push(item);
    groups.set(id, group);
  }
  return Array.from(groups.values());
}

function ResourceCandidateContent({
  section,
  databaseStep,
  type,
  parentId,
  search,
  candidates,
  selectedItems,
  selectedDatabaseIds,
  databaseCandidates,
  loading,
  error,
  emptyTitle,
  emptyDescription,
  selectDatabaseTitle,
  selectDatabaseDescription,
  noDescriptionLabel,
  skillNoMatchLabel,
  locale,
  retryLabel,
  onToggle,
  onChangeAccess,
  onToggleDatabase,
  onChangeDatabase,
  onRetry,
}: {
  section: BindingSectionKey;
  databaseStep: DatabasePickerStep;
  type: DigitalHumanBindingType;
  parentId: string;
  search: string;
  candidates: DigitalHumanResourceCandidate[];
  selectedItems: DigitalHumanResourceBindingInput[];
  selectedDatabaseIds: string[];
  databaseCandidates: DigitalHumanResourceCandidate[];
  loading: boolean;
  error: boolean;
  emptyTitle: string;
  emptyDescription: string;
  selectDatabaseTitle: string;
  selectDatabaseDescription: string;
  noDescriptionLabel: string;
  skillNoMatchLabel: string;
  locale: string;
  retryLabel: string;
  onToggle: (candidate: DigitalHumanResourceCandidate, checked: boolean) => void;
  onChangeAccess: (candidate: DigitalHumanResourceCandidate, mode: AccessMode) => void;
  onToggleDatabase: (candidate: DigitalHumanResourceCandidate, checked: boolean) => void;
  onChangeDatabase: (id: string) => void;
  onRetry: () => void;
}) {
  const t = useT('digitalHumans');
  // Skills own their body so the creation dialog renders exactly the same one.
  if (section === 'skills') {
    return (
      <DigitalHumanSkillDialogBody
        candidates={candidates}
        selectedIds={selectedItems.map(item => item.resource_id)}
        onToggle={candidate => onToggle(candidate, !selectedItems.some(item => item.resource_id === candidate.resource_id))}
        locale={locale}
        loading={loading}
        error={error}
        search={Boolean(search)}
        emptyTitle={emptyTitle}
        emptyDescription={emptyDescription}
        skillNoMatchLabel={skillNoMatchLabel}
        retryLabel={retryLabel}
        onRetry={onRetry}
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
            <RefreshCw className="size-4" />
            {retryLabel}
          </Button>
        }
      />
    );
  }
  if (candidates.length === 0 && !(section === 'databases' && databaseStep === 'tables')) {
    return (
      <ResourceSelectionEmptyState
        icon={
          search ? <SearchX /> : type === 'skill' ? <Puzzle /> : <BindingTypeIcon type={type} />
        }
        title={type === 'skill' && search ? skillNoMatchLabel : emptyTitle}
        description={emptyDescription}
        variant={search ? 'search' : 'resource'}
      />
    );
  }

  if (section === 'databases' && databaseStep === 'databases') {
    return (
      <ResourceSelectionGrid>
        {candidates.map(candidate => {
          const selected = selectedDatabaseIds.includes(candidate.resource_id);
          return (
            <ResourceOptionButton
              key={candidate.resource_id}
              icon={<Database />}
              title={candidate.display_name}
              description={candidate.description || noDescriptionLabel}
              selected={selected}
              disabled={!candidate.available}
              onClick={() => onToggleDatabase(candidate, !selected)}
            />
          );
        })}
      </ResourceSelectionGrid>
    );
  }

  if (section === 'databases') {
    return (
      <div className="grid h-full min-h-[460px] gap-4 lg:grid-cols-[240px_minmax(0,1fr)]">
        <aside className="flex min-h-0 flex-col overflow-hidden rounded-lg border bg-muted/15">
          <div className="border-b px-3.5 py-3">
            <div className="text-sm font-semibold">{t('bindings.databases.selectedDatabases')}</div>
            <div className="mt-1 text-xs leading-5 text-muted-foreground">
              {t('bindings.databases.selectTablesHint')}
            </div>
          </div>
          <div className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
            {databaseCandidates
              .filter(database => selectedDatabaseIds.includes(database.resource_id))
              .map(database => {
                const selectedCount = selectedItems.filter(
                  item =>
                    item.type === 'database_table' &&
                    item.parent_resource_id === database.resource_id
                ).length;
                return (
                  <button
                    key={database.resource_id}
                    type="button"
                    aria-pressed={parentId === database.resource_id}
                    className={cn(
                      'flex w-full items-center gap-2.5 rounded-md border border-transparent px-2.5 py-2 text-left transition-colors hover:bg-muted/60',
                      parentId === database.resource_id && 'border-primary/25 bg-primary/5'
                    )}
                    onClick={() => onChangeDatabase(database.resource_id)}
                  >
                    <ResourceSelectionCardIcon className="size-7 text-primary [&>svg]:size-3.5">
                      <Database />
                    </ResourceSelectionCardIcon>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-xs font-medium">
                        {database.display_name}
                      </span>
                      <span className="mt-0.5 block text-[11px] text-muted-foreground">
                        {selectedCount > 0
                          ? t('bindings.databases.selectedTablesCount', { count: selectedCount })
                          : t('bindings.databases.pendingTableSelection')}
                      </span>
                    </span>
                    {selectedCount > 0 ? <Check className="size-3.5 text-primary" /> : null}
                  </button>
                );
              })}
          </div>
        </aside>
        <section className="min-h-[420px] min-w-0 rounded-lg border bg-background p-4">
          {!parentId ? (
            <ResourceSelectionEmptyState
              icon={<Database />}
              title={selectDatabaseTitle}
              description={selectDatabaseDescription}
              className="min-h-[380px]"
            />
          ) : candidates.length === 0 ? (
            <ResourceSelectionEmptyState
              icon={search ? <SearchX /> : <Table2 />}
              title={emptyTitle}
              description={emptyDescription}
              className="min-h-[380px]"
            />
          ) : (
            <ResourceSelectionGrid>
              {candidates.map(candidate => {
                const selected = selectedItems.find(
                  item => resourceKey(item) === resourceKey(candidate)
                );
                return (
                  <div
                    key={resourceKey(candidate)}
                    className={cn(
                      'flex min-h-32 flex-col rounded-lg border bg-background p-3.5 shadow-sm transition-colors hover:border-primary/40 hover:bg-muted/20',
                      selected && 'border-primary bg-primary/5 hover:bg-primary/10'
                    )}
                  >
                    <button
                      type="button"
                      aria-pressed={Boolean(selected)}
                      className="flex w-full flex-1 flex-col text-left"
                      onClick={() => onToggle(candidate, !selected)}
                    >
                      <span className="flex min-h-8 items-center gap-3">
                        <ResourceSelectionCardIcon className="text-primary">
                          <Table2 />
                        </ResourceSelectionCardIcon>
                        <span className="min-w-0 flex-1 truncate text-sm font-semibold">
                          {candidate.display_name}
                        </span>
                        <SelectionMark selected={Boolean(selected)} />
                      </span>
                      <span className="mt-2.5 line-clamp-2 text-xs leading-5 text-muted-foreground">
                        {candidate.description || noDescriptionLabel}
                      </span>
                    </button>
                    {selected ? (
                      <div className="mt-auto flex items-center gap-2 pt-3 text-xs text-muted-foreground">
                        <Badge variant="subtle">
                          {selected.access_mode === 'write'
                            ? t('bindings.databases.writeEnabled')
                            : t('bindings.databases.readOnly')}
                        </Badge>
                        <Switch
                          checked={selected.access_mode === 'write'}
                          onCheckedChange={checked =>
                            onChangeAccess(candidate, checked ? 'write' : 'read')
                          }
                          aria-label={t('bindings.databases.allowWriteForTable', {
                            name: candidate.display_name,
                          })}
                        />
                        <span>{t('bindings.databases.allowWrite')}</span>
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </ResourceSelectionGrid>
          )}
        </section>
      </div>
    );
  }

  return (
    <ResourceSelectionGrid>
      {candidates.map(candidate => {
        const selected = selectedItems.some(item => resourceKey(item) === resourceKey(candidate));
        const isWorkflow = section === 'workflows';
        const workflowType = metadataString(candidate.metadata, 'agent_type');
        const updatedAt = metadataString(candidate.metadata, 'updated_at');
        return (
          <ResourceOptionButton
            key={resourceKey(candidate)}
            icon={isWorkflow ? <AgentWorkflowTypeIcon agentType={workflowType} /> : <Database />}
            title={candidate.display_name}
            titleAccessory={
              isWorkflow ? <AgentWorkflowTypeBadge agentType={workflowType} /> : undefined
            }
            description={candidate.description || noDescriptionLabel}
            footer={
              isWorkflow && updatedAt
                ? t('bindings.workflows.updatedAt', { time: formatDate(updatedAt) })
                : undefined
            }
            selected={selected}
            disabled={!candidate.available}
            onClick={() => onToggle(candidate, !selected)}
          />
        );
      })}
    </ResourceSelectionGrid>
  );
}

function ResourceOptionButton({
  icon,
  title,
  titleAccessory,
  description,
  footer,
  selected,
  disabled,
  onClick,
}: {
  icon: ReactNode;
  title: string;
  titleAccessory?: ReactNode;
  description: string;
  footer?: string;
  selected: boolean;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      disabled={disabled}
      className={cn(
        'flex min-h-32 cursor-pointer flex-col rounded-lg border bg-background p-3.5 text-left shadow-sm transition-colors hover:border-primary/40 hover:bg-muted/20',
        selected && 'border-primary bg-primary/5 hover:bg-primary/10',
        disabled && 'cursor-not-allowed opacity-60'
      )}
      onClick={onClick}
    >
      <span className="flex min-h-8 items-center gap-3">
        <ResourceSelectionCardIcon className="text-primary">{icon}</ResourceSelectionCardIcon>
        <span className="flex min-h-8 min-w-0 flex-1 items-center gap-2">
          <span className="min-w-0 flex-1 truncate text-sm font-semibold">{title}</span>
          {titleAccessory}
        </span>
        <SelectionMark selected={selected} />
      </span>
      <span className="mt-2.5 line-clamp-3 text-xs leading-5 text-muted-foreground">
        {description}
      </span>
      {footer ? (
        <span className="mt-auto pt-2.5 text-[11px] text-muted-foreground">{footer}</span>
      ) : null}
    </button>
  );
}

function SelectionMark({ selected }: { selected: boolean }) {
  return (
    <span
      className={cn(
        'flex size-5 shrink-0 items-center justify-center rounded-full border',
        selected ? 'border-primary bg-primary text-primary-foreground' : 'bg-background'
      )}
    >
      {selected ? <Check className="size-3.5" /> : null}
    </span>
  );
}

function BindingTypeIcon({ type }: { type: DigitalHumanBindingType }) {
  const Icon = bindingIcon(type);
  return <Icon />;
}

function bindingIcon(type: DigitalHumanBindingType) {
  if (type === 'skill') return Puzzle;
  if (type === 'knowledge_dataset') return BookOpen;
  if (type === 'database') return Database;
  if (type === 'database_table') return Table2;
  if (type === 'workflow') return Workflow;
  return PlugZap;
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
    metadata: {
      ...candidate.metadata,
      ...(candidate.description ? { description: candidate.description } : {}),
    },
  };
}

function toBindingInput(item: DigitalHumanResourceBinding): DigitalHumanResourceBindingInput {
  // Carries the row id so a replacement keeps binding identity instead of
  // re-authorizing it as a brand new resource.
  return digitalHumanBindingInput(item);
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
