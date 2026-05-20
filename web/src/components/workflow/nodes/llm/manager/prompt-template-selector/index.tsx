'use client';

import React from 'react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
} from '@/components/ui/dropdown-menu';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import { Lightbulb, FileText, X, AlertTriangle } from 'lucide-react';
import { Label } from '@/components/ui/label';
import { useTranslations, useLocale } from 'next-intl';
import { useWorkflowStore } from '@/components/workflow/store';
import { AgentType } from '@/services/types/agent';

// Strict template type to avoid any usage
interface PromptTemplateItem {
  id: string;
  title: string;
  description: string;
  text: string; // Applied to system prompt when selected
}

export interface PromptTemplateSelectorProps {
  className?: string;
  // Called when a template is chosen; apply text to system prompt
  onApply: (text: string) => void;
  disabled?: boolean;
}

type TemplateKey =
  | 'customerService'
  | 'contentCreator'
  | 'dataAnalyst'
  | 'productConsultant'
  // Workflow-oriented
  | 'translation'
  | 'copywriting'
  | 'story'
  | 'codeGeneration';

type SupportedLocale = 'en-US' | 'zh-Hans';

const TEXTS: Readonly<Record<SupportedLocale, Readonly<Record<TemplateKey, string>>>> = {
  'en-US': {
    customerService:
      "You are a professional customer service assistant. The user's request is: {{#sys.query#}}. First, restate the request in one sentence to confirm understanding. If key information is missing, ask one concise clarifying question. Then provide a step-by-step solution, followed by a brief final answer. Keep the tone friendly and concise.",
    contentCreator:
      'You are a seasoned content creator. Topic: {{#sys.query#}}. Produce content with a clear structure (Introduction / Body / Conclusion). Use bullet points or subheadings when helpful. Keep the tone natural and easy to read; avoid verbosity.',
    dataAnalyst:
      'You are a data analyst. Analysis question: {{#sys.query#}}. Provide data-driven insights, explain possible causes and impacts, and offer actionable recommendations. Explicitly mark uncertainties and suggest methods for further validation.',
    productConsultant:
      'You are a product consultant. User requirements: {{#sys.query#}}. Recommend suitable products and explain the rationale, pros/cons, and applicable scenarios. If information is incomplete, first ask for the essential requirements.',
    // Workflow-oriented templates
    translation:
      'You are a professional localization specialist. Perform faithful, fluent translation that preserves domain terminology, tone, intent, and formatting (markdown, lists, tables). Keep inline code, placeholders, and variables unchanged. Ensure numbers, units, and punctuation follow target-language conventions. Do not add explanations or notes; output the translated text only. Maintain sentence-level coherence and paragraph breaks. If the source contains code or commands, keep them unchanged unless localization is explicitly required. Align with brand style when inferable.',
    copywriting:
      'You are a senior copywriter. Produce high-converting copy aligned with modern marketing best practices. Deliver 3 distinct variants with different tones: informative, persuasive, playful. Each variant must include a concise headline (<= 12 words), a 1–2 sentence body, and a clear CTA. Keep benefits concrete, avoid cliches, reflect brand voice and target audience when inferable, and do not invent facts. Use concise language and strong verbs. Separate variants with a blank line.',
    story:
      'You are a professional storyteller. Write an original short story (~400–600 words) with a clear arc (setup, development, climax, resolution). Use vivid imagery, consistent point of view, and natural dialogue. Avoid cliches and moralizing; show, not tell. Maintain thematic coherence and believable character motivation. Output the title on the first line, then the story body. Output story only.',
    codeGeneration:
      'You are a senior software engineer. Produce clean, production-ready code that solves the specified task. Use TypeScript by default unless another language is explicitly required. Requirements: strong typing (no any), clear function signature, small pure functions, basic input validation and error handling, minimal but helpful inline comments, and a brief header comment indicating time/space complexity. Provide a minimal usage example or unit-test-style snippet after the main function. Output code only.',
  },
  'zh-Hans': {
    customerService:
      '你是一名专业的客服助手。用户请求：{{#sys.query#}}。请先用一句话复述用户需求以确认理解；若信息不足，提出一个关键的澄清问题；随后给出步骤化解决方案，并提供简洁的最终答复。语气保持友好、简洁。',
    contentCreator:
      '你是一名资深内容创作者。主题：{{#sys.query#}}。请按引言 / 主体 / 结论输出内容，必要时使用要点或小标题，保持自然易读，避免冗长。',
    dataAnalyst:
      '你是一名数据分析师。分析问题：{{#sys.query#}}。请基于数据提供洞察，解释可能的原因与影响，并给出可操作的建议；对不确定性要明确标注，并说明进一步验证的方法。',
    productConsultant:
      '你是一名产品顾问。用户需求：{{#sys.query#}}。请推荐合适的产品，并说明选择依据、优缺点与适用场景；信息不全时先询问关键需求。',
    // Workflow-oriented templates
    translation:
      '你是一名专业本地化专家。进行忠实、流畅的翻译，保持领域术语、语气、意图与格式（Markdown、列表、表格）一致。内联代码、占位符与变量保持不变。数字、单位与标点遵循目标语言规范。不添加解释或注释，仅输出译文。保持句子连贯与段落分隔。若源文包含代码或命令，除非明确需要本地化，否则保持不变。在可推断的情况下贴合品牌风格。',
    copywriting:
      '你是一名资深文案。按照现代营销最佳实践生成高转化文案。输出 3 个风格不同的版本：信息型、说服型、活泼型。每个版本包含：标题（不超过 12 个词）、1–2 句正文，以及清晰的 CTA。利益点具体，避免陈词滥调，尽量贴合可推断的品牌调性与目标受众，禁止虚构事实。语言简洁有力。各版本之间请空一行分隔。',
    story:
      '你是一名专业故事作者。创作一篇原创短篇（约 400–600 字），具备完整叙事弧（铺垫、发展、高潮、收束）。使用生动意象、统一视角与自然对话；避免陈词滥调与说教，以“展现”代替“告知”。确保主题连贯、人物动机可信。首行输出标题，其后为正文。仅输出故事内容。',
    codeGeneration:
      '你是一名资深工程师。编写可用于生产的代码以解决指定任务。默认使用 TypeScript（除非明确要求其他语言）。要求：强类型（禁止 any）、清晰函数签名、小而纯的函数、基础输入校验与错误处理、少量必要行内注释，并在文件开头用简短注释标注时间/空间复杂度。在主函数后提供最小使用示例或类单测片段。仅输出代码。',
  },
};

const PromptTemplateSelector: React.FC<PromptTemplateSelectorProps> = ({
  className,
  onApply,
  disabled = false,
}) => {
  const [open, setOpen] = React.useState(false);
  const [previewTemplate, setPreviewTemplate] = React.useState<PromptTemplateItem | null>(null);
  const t = useTranslations('nodes');
  const locale = useLocale();
  const currentLocale: SupportedLocale = locale === 'zh-Hans' ? 'zh-Hans' : 'en-US';
  const agentType = useWorkflowStore.use.agentType();
  const isConversational = agentType === AgentType.CONVERSATIONAL_AGENT;

  const templates: readonly PromptTemplateItem[] = React.useMemo(() => {
    const convKeys: ReadonlyArray<{ key: TemplateKey; id: string }> = [
      { key: 'customerService', id: 'customer-service' },
      { key: 'contentCreator', id: 'content-creator' },
      { key: 'dataAnalyst', id: 'data-analyst' },
      { key: 'productConsultant', id: 'product-consultant' },
    ] as const;
    const workflowKeys: ReadonlyArray<{ key: TemplateKey; id: string }> = [
      { key: 'translation', id: 'translation' },
      { key: 'copywriting', id: 'copywriting' },
      { key: 'story', id: 'story' },
      { key: 'codeGeneration', id: 'code-generation' },
    ] as const;
    const keys = isConversational ? convKeys : workflowKeys;
    return keys.map(({ key, id }) => ({
      id,
      title: t(`llm.promptTemplates.items.${key}.title`),
      description: t(`llm.promptTemplates.items.${key}.description`),
      text: TEXTS[currentLocale][key],
    }));
  }, [t, currentLocale, isConversational]);

  const handleTemplateClick = (template: PromptTemplateItem) => {
    setPreviewTemplate(template);
    setOpen(false);
  };

  const handleConfirmApply = () => {
    if (previewTemplate) {
      onApply(previewTemplate.text);
      setPreviewTemplate(null);
    }
  };

  return (
    <>
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger asChild>
          <span className={cn('inline-flex', className)}>
            <Button
              variant="ghost"
              size="xs"
              isIcon
              className="hover:bg-background"
              aria-label={t('llm.actions.selectPromptTemplate')}
              title={t('llm.actions.selectPromptTemplate')}
              disabled={disabled}
            >
              <Lightbulb className="h-4 w-4" />
            </Button>
          </span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="p-0 w-[340px] rounded-xl" sideOffset={6}>
          {/* Header */}
          <div className="flex items-center justify-between px-3 py-2 border-b">
            <div className="text-sm font-medium">{t('llm.promptTemplates.title')}</div>
            <button
              type="button"
              className="inline-flex items-center justify-center rounded-md hover:bg-muted p-1"
              aria-label={t('common.cancel')}
              onClick={() => setOpen(false)}
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          {/* List */}
          <div className="p-3 space-y-2">
            {templates.map(item => (
              <button
                key={item.id}
                type="button"
                aria-label={item.title}
                onClick={() => handleTemplateClick(item)}
                className={cn(
                  'w-full text-left rounded-lg border hover:bg-accent hover:text-accent-foreground transition-colors',
                  'px-3 py-2 flex items-start gap-2'
                )}
              >
                <span className="mt-0.5 inline-flex items-center justify-center rounded-md bg-muted text-muted-foreground p-1">
                  <FileText className="h-4 w-4" />
                </span>
                <span className="min-w-0 grow">
                  <div className="text-sm font-medium leading-none mb-1">{item.title}</div>
                  <div className="text-xs text-muted-foreground leading-relaxed">
                    {item.description}
                  </div>
                </span>
              </button>
            ))}
          </div>

          {/* Footer tip */}
          <div className="px-3 py-2 border-t text-xs text-muted-foreground">
            <span role="img" aria-label="tip" className="mr-1">
              💡
            </span>
            {t('llm.promptTemplates.footerTip')}
          </div>
        </DropdownMenuContent>
      </DropdownMenu>

      {/* Preview Dialog */}
      <Dialog open={!!previewTemplate} onOpenChange={isOpen => !isOpen && setPreviewTemplate(null)}>
        <DialogContent className="max-w-2xl p-0 overflow-hidden">
          <DialogHeader className="pb-2">
            <DialogTitle className="flex items-center gap-3 text-xl font-bold tracking-tight">
              <div className="h-8 w-8 bg-primary/10 text-primary flex items-center justify-center rounded-lg">
                <FileText className="h-5 w-5" />
              </div>
              {previewTemplate?.title}
            </DialogTitle>
          </DialogHeader>

          <DialogBody className="space-y-6 py-6 scrollbar-thin">
            <div className="space-y-5">
              {/* Description Section */}
              <div className="space-y-2">
                <div className="flex items-center gap-2 px-1">
                  <div className="h-4 w-1 bg-primary rounded-full" />
                  <Label className="text-sm font-bold uppercase tracking-wider text-primary/80">
                    {t('llm.promptTemplates.preview.description')}
                  </Label>
                </div>
                <div className="bg-neutral-50/50 p-4 rounded-2xl border border-neutral-100 shadow-sm text-sm text-neutral-600 font-medium leading-relaxed">
                  {previewTemplate?.description}
                </div>
              </div>

              {/* Preview Content Section */}
              <div className="space-y-2 flex flex-col min-h-0">
                <div className="flex items-center gap-2 px-1">
                  <div className="h-4 w-1 bg-primary rounded-full" />
                  <Label className="text-sm font-bold uppercase tracking-wider text-primary/80">
                    {t('llm.promptTemplates.preview.previewLabel')}
                  </Label>
                </div>
                <div className="grow bg-white rounded-2xl border border-neutral-100 shadow-premium overflow-hidden flex flex-col">
                  <div className="p-5 overflow-y-auto max-h-[36vh] scrollbar-thin">
                    <pre className="text-sm font-mono whitespace-pre-wrap leading-relaxed text-neutral-800 selection:bg-primary/20">
                      {previewTemplate?.text}
                    </pre>
                  </div>
                </div>
              </div>

              {/* Warning Section */}
              <div className="flex items-start gap-3 p-4 rounded-2xl border border-amber-100 bg-amber-50/50 shadow-sm animate-in fade-in slide-in-from-top-2">
                <AlertTriangle className="h-5 w-5 text-amber-500 shrink-0 mt-0.5" />
                <span className="text-xs font-bold text-amber-700 leading-relaxed">
                  {t('llm.promptTemplates.preview.warning')}
                </span>
              </div>
            </div>
          </DialogBody>

          <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
            <Button
              variant="ghost"
              onClick={() => setPreviewTemplate(null)}
              className="font-semibold"
            >
              {t('llm.promptTemplates.preview.cancel')}
            </Button>
            <Button onClick={handleConfirmApply} size="lg" className="px-10 font-bold shadow-sm">
              {t('llm.promptTemplates.preview.apply')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default PromptTemplateSelector;
