'use client';

import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { Panel } from '@xyflow/react';
import { usePanelStackItem } from '../../hooks';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Slider } from '@/components/ui/slider';
import { Input } from '@/components/ui/input';
import { useWorkflowStore } from '../../store';
import type { WorkflowFeatures } from '../../store/type';
import { Settings2, SlidersHorizontal, X } from 'lucide-react';
import FileUploadSettingsDialog from './file-upload-dialog';
import OpeningStatementDialog, {
  type OpeningStatementDialogValue,
} from './opening-statement-dialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  clampOpeningSlogan,
  getOpeningGuideEditorValue,
} from '@/utils/webapp/opening-statement';
import { getRightPanelMotionClassName, getRightPanelMotionStyle } from '../right-panel-motion';
import { useWorkflowEditor } from '../../hooks/use-workflow-editor';
import { useGenerateWorkflowSuggestedQuestions } from '@/hooks/workflow/use-workflow';
import { useLocale } from '@/hooks/use-locale';
import { toast } from 'sonner';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';
import { buildOpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';

const ITEM_ROW_CLASS =
  'flex items-center justify-between rounded-lg border border-border/70 bg-background px-3 py-3 gap-3 transition-colors hover:bg-muted/20';
const TWO_ACTION_ITEM_ROW_CLASS =
  'flex items-start justify-between rounded-lg border border-border/70 bg-background px-3 py-3 gap-3 transition-colors hover:bg-muted/20';
const ITEM_TEXT_CLASS = 'min-w-0 grow space-y-1';
const ITEM_LABEL_CLASS = 'truncate';
const ITEM_DESC_CLASS = 'text-xs text-muted-foreground line-clamp-3 overflow-ellipsis';
const ITEM_CONTROL_COLUMN_CLASS = 'flex shrink-0 flex-col items-center gap-2';
const SECTION_CARD_CLASS = 'rounded-xl border border-border/70 bg-card p-3.5';

function dedupeSuggestedQuestions(questions: string[]): string[] {
  const seen = new Set<string>();

  return questions
    .map(question => question.trim())
    .filter(question => {
      if (!question) return false;
      const key = question.toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

function normalizeSuggestedQuestionsForEditor(questions: string[] = []): string[] {
  return questions
    .filter(question => typeof question === 'string')
    .slice(0, SUGGESTED_QUESTIONS_LIMIT);
}

type FeaturesForm = Pick<
  WorkflowFeatures,
  | 'opening_statement_type'
  | 'opening_guide_version'
  | 'opening_slogan'
  | 'opening_statement'
  | 'opening_statement_enabled'
  | 'suggested_questions'
  | 'retriever_resource'
  | 'file_upload'
  | 'suggested_questions_after_answer'
  | 'text_to_speech'
  | 'speech_to_text'
  | 'sensitive_word_avoidance'
  | 'conversation_history'
  | 'webapp_workflow_config'
>;

interface FeaturesPanelProps {
  open: boolean;
  temporarilyHidden?: boolean;
  onClose: () => void;
  agentName?: string;
  agentIconType?: string;
  agentIcon?: string;
  agentIconUrl?: string;
}

export default function FeaturesPanel({
  open,
  temporarilyHidden = false,
  onClose,
  agentName,
  agentIconType,
  agentIcon,
  agentIconUrl,
}: FeaturesPanelProps) {
  const t = useT('agents');
  const tCommon = useT('common');
  const { agentId } = useWorkflowEditor();
  const { locale } = useLocale();

  const { panelStyle } = usePanelStackItem({
    id: 'features',
    position: 'top-right',
    order: 0,
    visible: open,
    width: 400,
    gap: 8,
  });

  const storeWorkflowData = useWorkflowStore.use.workflowData();
  const updateWorkflowFeatures = useWorkflowStore.use.updateWorkflowFeatures();

  const [form, setForm] = useState<FeaturesForm>({
    opening_statement_type: storeWorkflowData?.features?.opening_statement_type ?? 'slogan',
    opening_guide_version: storeWorkflowData?.features?.opening_guide_version,
    opening_slogan: storeWorkflowData?.features?.opening_slogan ?? '',
    opening_statement: storeWorkflowData?.features?.opening_statement ?? '',
    opening_statement_enabled: storeWorkflowData?.features?.opening_statement_enabled ?? false,
    suggested_questions: normalizeSuggestedQuestionsForEditor(
      storeWorkflowData?.features?.suggested_questions ?? []
    ),
    retriever_resource: storeWorkflowData?.features?.retriever_resource,
    file_upload: storeWorkflowData?.features?.file_upload,
    suggested_questions_after_answer: storeWorkflowData?.features?.suggested_questions_after_answer,
    text_to_speech: storeWorkflowData?.features?.text_to_speech,
    speech_to_text: storeWorkflowData?.features?.speech_to_text,
    sensitive_word_avoidance: storeWorkflowData?.features?.sensitive_word_avoidance,
    conversation_history: storeWorkflowData?.features?.conversation_history,
    webapp_workflow_config: storeWorkflowData?.features?.webapp_workflow_config,
  });
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false);
  const [openingDialogOpen, setOpeningDialogOpen] = useState(false);
  const generateSuggestedQuestions = useGenerateWorkflowSuggestedQuestions();

  const [shake, setShake] = useState(false);
  useEffect(() => {
    const win = window as Window & {
      __workflowFeaturesPanelOpen?: boolean;
      __workflowFeaturesPanelShake?: () => void;
    };
    win.__workflowFeaturesPanelOpen = open;
    win.__workflowFeaturesPanelShake = () => {
      setShake(true);
      window.setTimeout(() => setShake(false), 600);
    };
    return () => {
      win.__workflowFeaturesPanelOpen = false;
      win.__workflowFeaturesPanelShake = undefined as unknown as () => void;
    };
  }, [open]);

  const buildFormFromStore = useCallback((): FeaturesForm => {
    return {
      opening_statement_type: storeWorkflowData?.features?.opening_statement_type ?? 'slogan',
      opening_guide_version: storeWorkflowData?.features?.opening_guide_version,
      opening_slogan: storeWorkflowData?.features?.opening_slogan ?? '',
      opening_statement: storeWorkflowData?.features?.opening_statement ?? '',
      opening_statement_enabled: storeWorkflowData?.features?.opening_statement_enabled ?? false,
      suggested_questions: normalizeSuggestedQuestionsForEditor(
        storeWorkflowData?.features?.suggested_questions ?? []
      ),
      retriever_resource: storeWorkflowData?.features?.retriever_resource,
      file_upload: storeWorkflowData?.features?.file_upload,
      suggested_questions_after_answer:
        storeWorkflowData?.features?.suggested_questions_after_answer,
      text_to_speech: storeWorkflowData?.features?.text_to_speech,
      speech_to_text: storeWorkflowData?.features?.speech_to_text,
      sensitive_word_avoidance: storeWorkflowData?.features?.sensitive_word_avoidance,
      conversation_history: storeWorkflowData?.features?.conversation_history,
      webapp_workflow_config: storeWorkflowData?.features?.webapp_workflow_config,
    } as FeaturesForm;
  }, [storeWorkflowData.features]);

  const hydratedRef = useRef(false);

  useEffect(() => {
    if (!open) {
      hydratedRef.current = false;
      return;
    }
    if (hydratedRef.current) return;
    const nextForm = buildFormFromStore();
    setForm(nextForm);
    hydratedRef.current = true;
  }, [open, buildFormFromStore]);

  const handleToggle = useCallback(
    (
      key:
        | 'retriever_resource'
        | 'file_upload'
        | 'suggested_questions_after_answer'
        | 'text_to_speech'
        | 'speech_to_text'
        | 'sensitive_word_avoidance'
        | 'conversation_history',
      enabled: boolean
    ) => {
      setForm(prev => {
        const next = { ...prev } as FeaturesForm;
        switch (key) {
          case 'retriever_resource': {
            next.retriever_resource = { enabled } as WorkflowFeatures['retriever_resource'];
            break;
          }
          case 'file_upload': {
            const prevUpload =
              prev.file_upload ||
              ({
                enabled: false,
                allowed_file_types: [],
                allowed_file_extensions: [],
                allowed_file_upload_methods: [],
                number_limits: 3,
              } as WorkflowFeatures['file_upload']);
            next.file_upload = { ...prevUpload, enabled } as WorkflowFeatures['file_upload'];
            break;
          }
          case 'suggested_questions_after_answer': {
            next.suggested_questions_after_answer = {
              enabled,
            } as WorkflowFeatures['suggested_questions_after_answer'];
            break;
          }
          case 'text_to_speech': {
            const prevTts =
              storeWorkflowData.features.text_to_speech ||
              ({ enabled: false } as WorkflowFeatures['text_to_speech']);
            next.text_to_speech = { ...prevTts, enabled } as WorkflowFeatures['text_to_speech'];
            break;
          }
          case 'speech_to_text': {
            next.speech_to_text = { enabled } as WorkflowFeatures['speech_to_text'];
            break;
          }
          case 'sensitive_word_avoidance': {
            next.sensitive_word_avoidance = {
              enabled,
            } as WorkflowFeatures['sensitive_word_avoidance'];
            break;
          }
          case 'conversation_history': {
            const prevHist =
              prev.conversation_history ||
              ({
                enabled: false,
                history_window_size: 3,
              } as WorkflowFeatures['conversation_history']);
            next.conversation_history = {
              ...prevHist,
              enabled,
            } as WorkflowFeatures['conversation_history'];
            break;
          }
        }
        return next;
      });
    },
    [storeWorkflowData.features]
  );

  const setHistoryWindow = useCallback((n: number) => {
    const v = Math.max(1, Math.min(50, Math.floor(Number.isFinite(n) ? n : 1)));
    setForm(prev => {
      const next = {
        ...prev,
        conversation_history: {
          ...prev.conversation_history,
          history_window_size: v,
        },
      } as FeaturesForm;
      // store commit moved to effect
      return next;
    });
  }, []);

  const setWebAppWorkflowConfig = useCallback(
    (patch: Partial<WorkflowFeatures['webapp_workflow_config']>) => {
      setForm(prev => ({
        ...prev,
        webapp_workflow_config: {
          ...(prev.webapp_workflow_config || {
            allow_view_run_detail: true,
            auto_expand_run_detail: false,
          }),
          ...patch,
        },
      }));
    },
    []
  );

  const handleGenerateSuggestedQuestions = useCallback(async (value: OpeningStatementDialogValue) => {
    if (!storeWorkflowData) return undefined;
    try {
      const result = await generateSuggestedQuestions.mutateAsync({
        agentId,
        payload: {
          locale,
          count: SUGGESTED_QUESTIONS_LIMIT,
          graph: storeWorkflowData.graph,
          features: {
            ...storeWorkflowData.features,
            ...form,
            opening_slogan: value.title,
            opening_statement: value.message,
            suggested_questions: value.suggestedQuestions,
          },
          existing_questions: dedupeSuggestedQuestions(value.suggestedQuestions),
        },
      });

      if (!result.questions?.length) {
        toast.warning(t('workflow.features.suggestedQuestions.generateEmpty'));
        return undefined;
      }

      return {
        questions: result.questions.map(question => question.text),
        warnings: result.warnings ?? [],
      };
    } catch {
      // The mutation hook owns the user-facing error toast.
      return undefined;
    }
  }, [agentId, form, generateSuggestedQuestions, locale, storeWorkflowData, t]);

  // Instant-apply model, no explicit save

  const handleClose = useCallback(() => {
    onClose();
  }, [onClose]);

  // Post-render commit: diff local form vs store and update store
  useEffect(() => {
    if (!open) return;
    const storeF = storeWorkflowData?.features;
    if (!storeF) return;

    const bool = (obj?: { enabled?: unknown }) => Boolean(obj?.enabled);
    const normalizeOpeningSlogan = (value: unknown) => {
      if (typeof value !== 'string') return '';
      return value.trim().length > 0 ? clampOpeningSlogan(value) : '';
    };
    const normalizeOpeningStatement = (value: unknown) => {
      if (typeof value !== 'string') return '';
      return value.trim().length > 0 ? value : '';
    };
    const clampWin = (n: unknown) => {
      const v = typeof n === 'number' ? Math.floor(n) : 1;
      return Math.max(1, Math.min(50, v));
    };
    const normUpload = (u?: WorkflowFeatures['file_upload']) => {
      if (!u) return undefined;
      const types = [...(u.allowed_file_types ?? [])].slice().sort();
      const exts = [...(u.allowed_file_extensions ?? [])]
        .map(e => String(e).toLowerCase().replace(/^\./, ''))
        .slice()
        .sort();
      const methods = [...(u.allowed_file_upload_methods ?? [])].slice().sort();
      const num = typeof u.number_limits === 'number' ? u.number_limits : 0;
      return {
        enabled: Boolean(u.enabled),
        allowed_file_types: types,
        allowed_file_extensions: exts,
        allowed_file_upload_methods: methods,
        number_limits: num,
      } as WorkflowFeatures['file_upload'];
    };
    const normWebAppWorkflowConfig = (
      config?: WorkflowFeatures['webapp_workflow_config']
    ): WorkflowFeatures['webapp_workflow_config'] => ({
      allow_view_run_detail: config?.allow_view_run_detail ?? true,
      auto_expand_run_detail: Boolean(config?.auto_expand_run_detail ?? false),
    });
    const deepEqual = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

    const partial: Partial<WorkflowFeatures> = {};

    const fOpeningType = form.opening_statement_type === 'message' ? 'message' : 'slogan';
    const sOpeningType = storeF.opening_statement_type === 'message' ? 'message' : 'slogan';
    if (fOpeningType !== sOpeningType) {
      partial.opening_statement_type = fOpeningType;
    }

    const fOpeningGuideVersion = form.opening_guide_version === 2 ? 2 : undefined;
    const sOpeningGuideVersion = storeF.opening_guide_version === 2 ? 2 : undefined;
    if (fOpeningGuideVersion !== sOpeningGuideVersion) {
      partial.opening_guide_version = fOpeningGuideVersion;
    }

    const fOpeningSlogan = normalizeOpeningSlogan(form.opening_slogan);
    const sOpeningSlogan = normalizeOpeningSlogan(storeF.opening_slogan);
    if (fOpeningSlogan !== sOpeningSlogan) {
      partial.opening_slogan = fOpeningSlogan;
    }

    const fOpening = normalizeOpeningStatement(form.opening_statement);
    const sOpening = normalizeOpeningStatement(storeF.opening_statement);
    if (fOpening !== sOpening) {
      partial.opening_statement = fOpening;
    }
    if (Boolean(form.opening_statement_enabled) !== Boolean(storeF.opening_statement_enabled)) {
      partial.opening_statement_enabled = Boolean(form.opening_statement_enabled);
    }

    const rawFormSuggestedQuestions = dedupeSuggestedQuestions(form.suggested_questions ?? []);
    const rawStoreSuggestedQuestions = dedupeSuggestedQuestions(storeF.suggested_questions ?? []);
    const fSuggestedQuestions = rawFormSuggestedQuestions.slice(0, SUGGESTED_QUESTIONS_LIMIT);
    const sSuggestedQuestions = rawStoreSuggestedQuestions.slice(0, SUGGESTED_QUESTIONS_LIMIT);
    if (
      !deepEqual(fSuggestedQuestions, sSuggestedQuestions) ||
      rawStoreSuggestedQuestions.length > SUGGESTED_QUESTIONS_LIMIT
    ) {
      partial.suggested_questions = fSuggestedQuestions;
    }

    if (bool(form.retriever_resource) !== bool(storeF.retriever_resource)) {
      partial.retriever_resource = { enabled: bool(form.retriever_resource) };
    }
    if (
      bool(form.suggested_questions_after_answer) !== bool(storeF.suggested_questions_after_answer)
    ) {
      partial.suggested_questions_after_answer = {
        enabled: bool(form.suggested_questions_after_answer),
      } as WorkflowFeatures['suggested_questions_after_answer'];
    }
    if (bool(form.sensitive_word_avoidance) !== bool(storeF.sensitive_word_avoidance)) {
      partial.sensitive_word_avoidance = {
        enabled: bool(form.sensitive_word_avoidance),
      } as WorkflowFeatures['sensitive_word_avoidance'];
    }
    if (bool(form.speech_to_text) !== bool(storeF.speech_to_text)) {
      partial.speech_to_text = {
        enabled: bool(form.speech_to_text),
      } as WorkflowFeatures['speech_to_text'];
    }
    // Preserve tts language/voice
    if (bool(form.text_to_speech) !== bool(storeF.text_to_speech)) {
      partial.text_to_speech = {
        ...(storeF.text_to_speech || ({} as WorkflowFeatures['text_to_speech'])),
        enabled: bool(form.text_to_speech),
      } as WorkflowFeatures['text_to_speech'];
    }

    const fUpload = normUpload(form.file_upload);
    const sUpload = normUpload(storeF.file_upload);
    if (!deepEqual(fUpload, sUpload)) {
      partial.file_upload = fUpload as WorkflowFeatures['file_upload'];
    }

    const fChEnabled = bool(form.conversation_history);
    const fChWin = clampWin(form.conversation_history?.history_window_size);
    const sChEnabled = bool(storeF.conversation_history);
    const sChWin = clampWin(storeF.conversation_history?.history_window_size);
    if (fChEnabled !== sChEnabled || (fChEnabled && fChWin !== sChWin)) {
      partial.conversation_history = {
        enabled: fChEnabled,
        history_window_size: fChWin,
      } as WorkflowFeatures['conversation_history'];
    }

    const fWebApp = normWebAppWorkflowConfig(form.webapp_workflow_config);
    const sWebApp = normWebAppWorkflowConfig(storeF.webapp_workflow_config);
    if (!deepEqual(fWebApp, sWebApp)) {
      partial.webapp_workflow_config = fWebApp;
    }

    if (Object.keys(partial).length > 0) {
      updateWorkflowFeatures(partial);
    }
  }, [open, form, storeWorkflowData?.features, updateWorkflowFeatures]);

  const openingGuidePreviewBrand = useMemo(
    () =>
      buildOpeningGuideBrand({
        title: agentName,
        iconType: agentIconType,
        icon: agentIcon,
        iconUrl: agentIconUrl,
      }),
    [agentIcon, agentIconType, agentIconUrl, agentName]
  );

  if (!open) return null;

  const suggestedQuestions = normalizeSuggestedQuestionsForEditor(form.suggested_questions ?? []);
  const configuredSuggestedQuestionCount = dedupeSuggestedQuestions(suggestedQuestions).length;
  const openingEditorValue = getOpeningGuideEditorValue(form);
  const hasOpeningContent =
    Boolean(openingEditorValue.title.trim()) ||
    Boolean(openingEditorValue.message.trim()) ||
    configuredSuggestedQuestionCount > 0;
  const openingSummaryParts = [
    openingEditorValue.title.trim() ? t('workflow.features.openingStatement.types.slogan') : null,
    openingEditorValue.message.trim() ? t('workflow.features.openingStatement.types.message') : null,
    configuredSuggestedQuestionCount > 0 ? t('workflow.features.suggestedQuestions.label') : null,
  ].filter(Boolean);

  return (
    <Panel
      position="top-right"
      aria-hidden={temporarilyHidden}
      className={getRightPanelMotionClassName(
        `w-[min(400px,calc(100vw-2rem))] overflow-hidden rounded-xl border border-border/70 bg-background p-0 shadow-xl h-[calc(100%-120px)] ${shake ? 'workflow-panel-attention' : ''}`,
        temporarilyHidden
      )}
      style={getRightPanelMotionStyle(panelStyle, temporarilyHidden)}
    >
      <div className="flex flex-col h-full" onContextMenu={e => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b border-border/60 px-4 py-3">
          <div className="flex items-center gap-2 text-base font-semibold">
            <Settings2 className="h-4 w-4 text-muted-foreground" /> {t('title')}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" isIcon onClick={handleClose} aria-label={tCommon('close')}>
              <X size={16} className="text-muted-foreground" />
            </Button>
          </div>
        </div>
        <div className="flex-1 min-h-0 overflow-auto p-4 space-y-3">
          <div className={TWO_ACTION_ITEM_ROW_CLASS}>
            <div className={ITEM_TEXT_CLASS}>
              <Label className={ITEM_LABEL_CLASS}>{t('workflow.features.uploadLabel')}</Label>
              <p className={ITEM_DESC_CLASS}>{t('workflow.features.uploadDesc')}</p>
            </div>
            <div className={ITEM_CONTROL_COLUMN_CLASS}>
              <Switch
                checked={form.file_upload?.enabled ?? false}
                onCheckedChange={v => handleToggle('file_upload', v)}
              />
              <Tooltip disableHoverableContent>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    isIcon
                    size="sm"
                    interactive="subtle"
                    onClick={() => setUploadDialogOpen(true)}
                    disabled={!form.file_upload?.enabled}
                    aria-label={t('workflow.features.uploadConfigure')}
                  >
                    <SlidersHorizontal size={18} strokeWidth={2.2} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top" align="center">
                  {t('workflow.features.uploadConfigure')}
                </TooltipContent>
              </Tooltip>
            </div>
          </div>

          <div className={SECTION_CARD_CLASS}>
            <div className="flex items-center justify-between">
              <div className={ITEM_TEXT_CLASS}>
                <Label className={ITEM_LABEL_CLASS}>
                  {t('workflow.features.conversationHistory.label')}
                </Label>
                <p className={ITEM_DESC_CLASS}>{t('workflow.features.conversationHistory.desc')}</p>
              </div>
              <Switch
                checked={form.conversation_history?.enabled ?? false}
                onCheckedChange={v => handleToggle('conversation_history', v)}
              />
            </div>
            {form.conversation_history?.enabled ? (
              <div className="mt-3 space-y-2">
                <div className="flex items-center justify-between">
                  <div className="text-xs text-muted-foreground">
                    {t('workflow.features.conversationHistory.windowLabel')}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {String(form.conversation_history?.history_window_size ?? 3)}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Slider
                    min={1}
                    max={50}
                    step={1}
                    value={[form.conversation_history?.history_window_size ?? 3]}
                    onValueChange={vals =>
                      setHistoryWindow(
                        Array.isArray(vals) && typeof vals[0] === 'number' ? vals[0] : 1
                      )
                    }
                    className="flex-1"
                  />
                  <Input
                    type="number"
                    inputMode="numeric"
                    min={1}
                    max={50}
                    step={1}
                    className="w-20 h-8 text-center"
                    value={String(form.conversation_history?.history_window_size ?? 3)}
                    onChange={e => setHistoryWindow(e.currentTarget.valueAsNumber)}
                  />
                </div>
              </div>
            ) : null}
          </div>

          <div className={TWO_ACTION_ITEM_ROW_CLASS}>
            <div className={ITEM_TEXT_CLASS}>
              <Label className={ITEM_LABEL_CLASS}>
                {t('workflow.features.openingStatement.enableLabel')}
              </Label>
              <p className={ITEM_DESC_CLASS}>
                {t('workflow.features.openingStatement.enableDesc')}
              </p>
              <p className={ITEM_DESC_CLASS}>
                {openingSummaryParts.length > 0
                  ? openingSummaryParts.join(' / ')
                  : t('workflow.features.openingStatement.previewEmptySlogan')}
              </p>
              {form.opening_statement_enabled && !hasOpeningContent ? (
                <p className={ITEM_DESC_CLASS}>
                  {t('workflow.features.openingStatement.previewEmptyMessage')}
                </p>
              ) : null}
            </div>
            <div className={ITEM_CONTROL_COLUMN_CLASS}>
              <Switch
                checked={Boolean(form.opening_statement_enabled)}
                onCheckedChange={value =>
                  setForm(prev => ({
                    ...prev,
                    opening_statement_enabled: value,
                  }))
                }
              />
              <Tooltip disableHoverableContent>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    isIcon
                    size="sm"
                    interactive="subtle"
                    onClick={() => setOpeningDialogOpen(true)}
                    aria-label={t('workflow.features.openingStatement.dialogTitle')}
                  >
                    <SlidersHorizontal size={18} strokeWidth={2.2} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top" align="center">
                  {t('workflow.features.openingStatement.dialogTitle')}
                </TooltipContent>
              </Tooltip>
            </div>
          </div>

          <div className={SECTION_CARD_CLASS}>
            <div className="space-y-3">
              <div>
                <Label className={ITEM_LABEL_CLASS}>
                  {t('workflow.features.webappWorkflowConfig.title')}
                </Label>
              </div>

              <div className={ITEM_ROW_CLASS}>
                <div className={ITEM_TEXT_CLASS}>
                  <Label className={ITEM_LABEL_CLASS}>
                    {t('workflow.features.webappWorkflowConfig.allowViewRunDetailLabel')}
                  </Label>
                  <p className={ITEM_DESC_CLASS}>
                    {t('workflow.features.webappWorkflowConfig.allowViewRunDetailDesc')}
                  </p>
                </div>
                <Switch
                  checked={form.webapp_workflow_config?.allow_view_run_detail ?? true}
                  onCheckedChange={value =>
                    setWebAppWorkflowConfig({ allow_view_run_detail: value })
                  }
                />
              </div>

              <div
                className={`${ITEM_ROW_CLASS} ${
                  (form.webapp_workflow_config?.allow_view_run_detail ?? true) ? '' : 'opacity-60'
                }`}
              >
                <div className={ITEM_TEXT_CLASS}>
                  <Label className={ITEM_LABEL_CLASS}>
                    {t('workflow.features.webappWorkflowConfig.autoExpandRunDetailLabel')}
                  </Label>
                  <p className={ITEM_DESC_CLASS}>
                    {t('workflow.features.webappWorkflowConfig.autoExpandRunDetailDesc')}
                  </p>
                  {!(form.webapp_workflow_config?.allow_view_run_detail ?? true) ? (
                    <p className={ITEM_DESC_CLASS}>
                      {t('workflow.features.webappWorkflowConfig.autoExpandRunDetailDisabled')}
                    </p>
                  ) : null}
                </div>
                <Switch
                  checked={form.webapp_workflow_config?.auto_expand_run_detail ?? false}
                  disabled={!(form.webapp_workflow_config?.allow_view_run_detail ?? true)}
                  onCheckedChange={value =>
                    setWebAppWorkflowConfig({ auto_expand_run_detail: value })
                  }
                />
              </div>
            </div>
          </div>
        </div>

        {/* Footer removed in instant-apply model */}
      </div>
      <FileUploadSettingsDialog
        open={uploadDialogOpen}
        onOpenChange={setUploadDialogOpen}
        value={form.file_upload}
        onChange={(val: Partial<WorkflowFeatures['file_upload']> | undefined) => {
          setForm(prev => {
            const normalize = (u?: WorkflowFeatures['file_upload']) => {
              if (!u) return undefined;
              const types = [...(u.allowed_file_types ?? [])].slice().sort();
              const exts = [...(u.allowed_file_extensions ?? [])]
                .map(e => String(e).toLowerCase().replace(/^\./, ''))
                .slice()
                .sort();
              const methods = [...(u.allowed_file_upload_methods ?? [])].slice().sort();
              const num = typeof u.number_limits === 'number' ? u.number_limits : 0;
              return {
                enabled: Boolean(u.enabled),
                allowed_file_types: types,
                allowed_file_extensions: exts,
                allowed_file_upload_methods: methods,
                number_limits: num,
              } as WorkflowFeatures['file_upload'];
            };

            const merged: WorkflowFeatures['file_upload'] | undefined =
              val === undefined && !prev.file_upload
                ? prev.file_upload
                : ({ ...prev.file_upload, ...val } as WorkflowFeatures['file_upload']);

            const prevN = normalize(prev.file_upload);
            const mergedN = normalize(merged);
            const same = JSON.stringify(prevN) === JSON.stringify(mergedN);
            if (same) return prev;
            return { ...prev, file_upload: mergedN } as typeof prev;
          });
        }}
      />
      <OpeningStatementDialog
        open={openingDialogOpen}
        onOpenChange={setOpeningDialogOpen}
        value={{
          ...openingEditorValue,
          suggestedQuestions,
        }}
        onSave={value =>
          setForm(prev => ({
            ...prev,
            opening_statement_type: value.message.trim() ? 'message' : 'slogan',
            opening_guide_version: 2,
            opening_slogan: clampOpeningSlogan(value.title),
            opening_statement: value.message,
            suggested_questions: normalizeSuggestedQuestionsForEditor(value.suggestedQuestions),
          }))
        }
        onGenerateSuggestedQuestions={handleGenerateSuggestedQuestions}
        generatingSuggestedQuestions={generateSuggestedQuestions.isPending}
        previewBrand={openingGuidePreviewBrand}
      />
    </Panel>
  );
}
