'use client';

import { useEffect, useMemo, useState } from 'react';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useLocale } from '@/hooks/use-locale';
import { useCurrentWorkspace, useIsOrganizationMode } from '@/store/workspace-store';
import type { CreatePromptRequest, PromptSummary, PromptType } from '@/services/types/prompt';

interface PromptFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (data: CreatePromptRequest) => Promise<unknown> | unknown;
  initialPrompt?: PromptSummary | null;
  initialDraft?: Partial<CreatePromptRequest>;
}

const emptyChatPrompt = JSON.stringify(
  [
    { role: 'system', content: 'You are a helpful assistant.' },
    { role: 'user', content: '{{input}}' },
  ],
  null,
  2
);

function normalizePromptLocale(locale?: string): string {
  switch (locale) {
    case 'zh-Hans':
    case 'en-US':
    case 'ja-JP':
      return locale;
    default:
      return 'zh-Hans';
  }
}

export function PromptFormDialog({
  open,
  onOpenChange,
  onSubmit,
  initialPrompt,
  initialDraft,
}: PromptFormDialogProps) {
  const t = useT('prompts');
  const { locale: currentLocale } = useLocale();
  const currentWorkspace = useCurrentWorkspace();
  const isOrganizationMode = useIsOrganizationMode();

  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [description, setDescription] = useState('');
  const [locale, setLocale] = useState('zh-Hans');
  const [category, setCategory] = useState('');
  const [tags, setTags] = useState('');
  const [source, setSource] = useState<'personal' | 'workspace'>('personal');
  const [promptType, setPromptType] = useState<PromptType>('text');
  const [content, setContent] = useState('');
  const [contentError, setContentError] = useState('');
  const [commitMessage, setCommitMessage] = useState('');
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const defaultLocale = useMemo(() => normalizePromptLocale(currentLocale), [currentLocale]);

  const effectiveWorkspaceId = useMemo(
    () => (isOrganizationMode ? selectedWorkspace?.id : currentWorkspace?.id),
    [currentWorkspace?.id, isOrganizationMode, selectedWorkspace?.id]
  );

  useEffect(() => {
    if (!open) return;
    if (initialPrompt) {
      setName(initialPrompt.name);
      setSlug(initialPrompt.slug);
      setDescription(initialPrompt.description ?? '');
      setLocale(initialPrompt.locale);
      setCategory(initialPrompt.category ?? '');
      setTags(initialPrompt.tags.join(', '));
      setSource(initialPrompt.source === 'workspace' ? 'workspace' : 'personal');
      if (initialDraft?.initial_version?.prompt_type) {
        setPromptType(initialDraft.initial_version.prompt_type);
      }
      if (initialDraft?.initial_version?.content) {
        setContent(
          typeof initialDraft.initial_version.content === 'string'
            ? initialDraft.initial_version.content
            : JSON.stringify(initialDraft.initial_version.content, null, 2)
        );
      }
      setCommitMessage(initialDraft?.initial_version?.commit_message ?? '');
      setContentError('');
      setAdvancedOpen(false);
    } else {
      setName(initialDraft?.name ?? '');
      setSlug(initialDraft?.slug ?? '');
      setDescription(initialDraft?.description ?? '');
      setLocale(normalizePromptLocale(initialDraft?.locale ?? defaultLocale));
      setCategory(initialDraft?.category ?? '');
      setTags(initialDraft?.tags?.join(', ') ?? '');
      setSource((initialDraft?.source as 'personal' | 'workspace' | undefined) ?? 'personal');
      setPromptType(initialDraft?.initial_version?.prompt_type ?? 'text');
      setContent(
        initialDraft?.initial_version?.content
          ? typeof initialDraft.initial_version.content === 'string'
            ? initialDraft.initial_version.content
            : JSON.stringify(initialDraft.initial_version.content, null, 2)
          : ''
      );
      setCommitMessage(initialDraft?.initial_version?.commit_message ?? '');
      setContentError('');
      setSelectedWorkspace(undefined);
      setAdvancedOpen(false);
    }
  }, [defaultLocale, initialDraft, initialPrompt, open]);

  const canSubmit = name.trim().length > 0 && !!effectiveWorkspaceId && content.trim().length > 0;

  const handleSubmit = async () => {
    if (!canSubmit || !effectiveWorkspaceId) return;
    setIsSubmitting(true);
    try {
      let parsedContent: CreatePromptRequest['initial_version']['content'] = content;
      if (promptType !== 'text') {
        try {
          parsedContent = JSON.parse(content) as CreatePromptRequest['initial_version']['content'];
        } catch {
          setContentError(t('form.invalidChatJson'));
          return;
        }
      }
      setContentError('');
      await onSubmit({
        workspace_id: effectiveWorkspaceId,
        source,
        name: name.trim(),
        slug: slug.trim() || undefined,
        description: description.trim() || null,
        locale,
        category: category.trim() || null,
        tags: tags
          .split(',')
          .map(item => item.trim())
          .filter(Boolean),
        initial_version: {
          prompt_type: promptType,
          content: parsedContent,
          labels: ['production'],
          commit_message: commitMessage.trim() || null,
        },
      });
      onOpenChange(false);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('form.createTitle')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-5">
          <div className="rounded-lg border bg-muted/20 p-3 text-sm text-muted-foreground">
            {t('form.simpleHint')}
          </div>
          {isOrganizationMode ? (
            <div className="space-y-2">
              <Label>{t('fields.workspace')}</Label>
              <WorkspaceSelector value={selectedWorkspace} onChange={setSelectedWorkspace} autoSelectFirst />
            </div>
          ) : null}
          <div className="space-y-2">
            <Label>{t('fields.name')}</Label>
            <Input value={name} onChange={e => setName(e.target.value)} placeholder={t('placeholders.name')} />
          </div>
          <div className="space-y-2">
            <Label>{t('fields.content')}</Label>
            <Textarea
              value={content}
              onChange={e => {
                setContent(e.target.value);
                setContentError('');
              }}
              placeholder={promptType === 'chat' ? t('placeholders.chatContent') : t('placeholders.textContent')}
              className="min-h-56 font-mono text-xs"
            />
            {contentError ? (
              <div className="text-sm text-destructive">{contentError}</div>
            ) : null}
          </div>
          <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
            <div className="rounded-xl border">
              <CollapsibleTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  className="w-full justify-between rounded-xl px-4 py-3 h-auto"
                >
                  <span>{advancedOpen ? t('actions.lessOptions') : t('actions.moreOptions')}</span>
                  {advancedOpen ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="border-t px-4 py-4 space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label>{t('fields.source')}</Label>
                    <Select value={source} onValueChange={value => setSource(value as 'personal' | 'workspace')}>
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="personal">{t('sources.personal')}</SelectItem>
                        <SelectItem value="workspace">{t('sources.workspace')}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>{t('fields.locale')}</Label>
                    <Select value={locale} onValueChange={setLocale}>
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="zh-Hans">zh-Hans</SelectItem>
                        <SelectItem value="en-US">en-US</SelectItem>
                        <SelectItem value="ja-JP">ja-JP</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>{t('fields.promptType')}</Label>
                    <Select
                      value={promptType}
                      onValueChange={value => {
                        const next = value as PromptType;
                        setPromptType(next);
                        if (!content.trim()) {
                          setContent(next === 'chat' ? emptyChatPrompt : '');
                        }
                      }}
                    >
                      <SelectTrigger><SelectValue /></SelectTrigger>
                      <SelectContent>
                        <SelectItem value="text">text</SelectItem>
                        <SelectItem value="chat">chat</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label>{t('fields.slug')}</Label>
                    <Input value={slug} onChange={e => setSlug(e.target.value)} placeholder={t('placeholders.slug')} />
                  </div>
                  <div className="space-y-2">
                    <Label>{t('fields.category')}</Label>
                    <Input value={category} onChange={e => setCategory(e.target.value)} placeholder={t('placeholders.category')} />
                  </div>
                  <div className="space-y-2">
                    <Label>{t('fields.tags')}</Label>
                    <Input value={tags} onChange={e => setTags(e.target.value)} placeholder={t('placeholders.tags')} />
                  </div>
                </div>
                <div className="space-y-2">
                  <Label>{t('fields.description')}</Label>
                  <Textarea
                    value={description}
                    onChange={e => setDescription(e.target.value)}
                    placeholder={t('placeholders.description')}
                    className="min-h-20"
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('fields.commitMessage')}</Label>
                  <Input
                    value={commitMessage}
                    onChange={e => setCommitMessage(e.target.value)}
                    placeholder={t('placeholders.commitMessage')}
                  />
                </div>
              </CollapsibleContent>
            </div>
          </Collapsible>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isSubmitting}>
            {t('actions.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={!canSubmit || isSubmitting}>
            {t('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default PromptFormDialog;
