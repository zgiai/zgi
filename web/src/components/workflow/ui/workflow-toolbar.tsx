import React from 'react';
import { useReactFlow } from '@xyflow/react';
import { Button } from '@/components/ui/button';
import {
  AlertCircle,
  CheckCircle,
  TriangleAlert,
  Layers,
  GitBranch,
  ArrowUpRight,
} from 'lucide-react';
import { useWorkflowOperations } from '../hooks';
import useWorkflowValidation from '../hooks/use-workflow-validation';
import { toast } from 'sonner';
import { useParams } from 'next/navigation';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useWorkflowStore } from '../store';
import { getNodeAbsolutePosition } from '../store/helpers/graph';
import type { WorkflowNode } from '../store/type';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { NODE_THEMES } from '../nodes/custom/config';
import ImageSafe from '@/components/common/image-safe';
// import useOpenWorkflowIssues from '../hooks/use-open-workflow-issues';

const WorkflowToolbar: React.FC = () => {
  const params = useParams();
  const agentId = params.agentId as string;

  const { getWorkflowStats } = useWorkflowOperations();

  const { isValid, hasWarnings, errors, warnings } = useWorkflowValidation();
  const selectNode = useWorkflowStore.use.selectNode();
  const setSelectionSource = useWorkflowStore.use.setSelectionSource();
  const stats = getWorkflowStats();
  const t = useT('nodes');

  const { setCenter, getNodes } = useReactFlow();

  const handleNodeClick = (nodeId: string) => {
    selectNode(nodeId);
    setSelectionSource('click');

    const nodes = getNodes();
    const node = nodes.find(n => n.id === nodeId);
    if (node) {
      const width = node.measured?.width ?? node.width ?? 240;
      const height = node.measured?.height ?? node.height ?? 120;
      const absPos = getNodeAbsolutePosition(nodeId, nodes as WorkflowNode[]);
      const x = absPos.x + width / 2;
      const y = absPos.y + height / 2;
      setCenter(x, y, { zoom: 1, duration: 800 });
    }
  };

  const [open, setOpen] = React.useState(false);
  const requestOpen = useWorkflowStore.use.openValidationIssues();
  const setRequestOpen = useWorkflowStore.use.setOpenValidationIssues();
  React.useEffect(() => {
    if (requestOpen) {
      setOpen(true);
      setRequestOpen(false);
    }
  }, [requestOpen, setRequestOpen]);

  const nodes = useWorkflowStore.use.nodes();
  const groupByNode = (items: Array<{ nodeId?: string; nodeTitle?: string; message: string }>) => {
    const groups = new Map<
      string,
      { title: string; id: string; type?: string; iconUrl?: string; items: string[] }
    >();
    const globalKey = '__global__';
    for (const it of items) {
      const key = it.nodeId || globalKey;
      const id = it.nodeId || '';
      const node = nodes.find(n => n.id === id);
      const title =
        it.nodeTitle ||
        node?.data?.title ||
        (it.nodeId ? it.nodeId : t('workflow.toolbar.noIssues'));

      const prev = groups.get(key);
      if (prev) {
        prev.items.push(it.message);
      } else {
        groups.set(key, {
          title,
          id,
          type: (node?.data as any)?.type,
          iconUrl: (node?.data as any)?.iconUrl,
          items: [it.message],
        });
      }
    }
    return groups;
  };

  const errorGroups = groupByNode(errors);
  const warningGroups = groupByNode(warnings);

  return (
    <div className="flex items-center gap-2.5 bg-secondary/20 border border-secondary shadow-sm rounded-full px-4 h-9">
      {/* Workflow Stats */}
      <div className="flex items-center gap-4 text-[13px] font-medium text-muted-foreground mr-1">
        <div
          className="flex items-center gap-1.5"
          title={t('workflow.toolbar.stats.nodes', { count: stats.nodeCount })}
        >
          <Layers className="w-3.5 h-3.5" />
          <span className="tabular-nums">{stats.nodeCount}</span>
        </div>
        <div
          className="flex items-center gap-1.5"
          title={t('workflow.toolbar.stats.edges', { count: stats.edgeCount })}
        >
          <GitBranch className="w-3.5 h-3.5" />
          <span className="tabular-nums">{stats.edgeCount}</span>
        </div>
      </div>

      {/* Validation Status with dropdown details */}
      <div className="flex items-center gap-1">
        <DropdownMenu open={open} onOpenChange={setOpen}>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className={cn(
                'h-7 hover:bg-white/40 dark:hover:bg-white/5 px-2 gap-1.5 font-bold transition-all rounded-full group',
                !isValid ? 'text-red-500' : hasWarnings ? 'text-amber-500' : 'text-emerald-500'
              )}
            >
              <div
                className={cn(
                  'flex items-center justify-center w-4 h-4 rounded-full',
                  !isValid ? 'bg-red-500/10' : hasWarnings ? 'bg-amber-500/10' : 'bg-emerald-500/10'
                )}
              >
                {!isValid ? (
                  <AlertCircle className="w-3 h-3 stroke-[3]" />
                ) : hasWarnings ? (
                  <TriangleAlert className="w-3 h-3 stroke-[3]" />
                ) : (
                  <CheckCircle className="w-3 h-3 stroke-[3]" />
                )}
              </div>

              <span className="text-[12px] tracking-tight">
                {!isValid
                  ? t('workflow.toolbar.status.errorCount', { count: errors.length })
                  : hasWarnings
                    ? t('workflow.toolbar.status.warningCount', { count: warnings.length })
                    : t('workflow.toolbar.status.passed')}
              </span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="end"
            className="w-[440px] mt-1 max-h-[80vh] p-0 bg-background border border-border shadow-[0_20px_50px_rgba(0,0,0,0.15)] dark:shadow-[0_20px_50px_rgba(0,0,0,0.3)] rounded-3xl overflow-hidden flex flex-col"
            onCloseAutoFocus={e => e.preventDefault()}
          >
            {!isValid || hasWarnings ? (
              <div className="flex-1 overflow-y-auto custom-scrollbar p-3 space-y-3">
                {/* Errors Section */}
                {!isValid && (
                  <div className="space-y-2">
                    <div className="px-3 py-1.5 flex items-center gap-2.5 text-red-600">
                      <div className="w-1.5 h-1.5 rounded-full bg-red-600 shadow-[0_0_8px_rgba(220,38,38,0.5)]" />
                      <span className="text-xs font-black uppercase tracking-[0.2em] opacity-90">
                        {t('workflow.toolbar.section.errors', { count: errors.length })}
                      </span>
                    </div>
                    <div className="space-y-4">
                      {[...errorGroups.values()].map((g, gi) => {
                        const theme = g.type
                          ? NODE_THEMES[g.type as keyof typeof NODE_THEMES]
                          : NODE_THEMES.default;
                        const Icon = theme?.icon || AlertCircle;

                        return (
                          <DropdownMenuItem
                            key={`err-group-${gi}`}
                            onSelect={() => g.id && handleNodeClick(g.id)}
                            className="p-0 mb-4 last:mb-0 focus:bg-transparent hover:shadow-md rounded-2xl overflow-hidden border border-border/50 bg-card/10 hover:bg-muted/40 transition-all cursor-pointer group/node-issue block"
                          >
                            <div className="flex flex-col w-full">
                              {/* Node Header */}
                              <div className="px-3 py-1.5 flex items-center gap-3 border-b border-border/40 bg-muted/40">
                                <div
                                  className={cn(
                                    'p-1.5 rounded-lg shadow-sm flex items-center justify-center overflow-hidden transition-all',
                                    theme?.classNames?.iconBg ||
                                      'bg-background border border-border/60',
                                    theme?.classNames?.iconBg ? 'text-white' : 'text-foreground/70'
                                  )}
                                >
                                  {g.iconUrl ? (
                                    <ImageSafe
                                      src={g.iconUrl}
                                      className="w-3.5 h-3.5 rounded-full"
                                    />
                                  ) : (
                                    <Icon
                                      className="text-primary-foreground"
                                      size={14}
                                      strokeWidth={2.5}
                                    />
                                  )}
                                </div>
                                <span className="text-[11px] font-black text-foreground/80 uppercase tracking-widest leading-none flex-1 truncate">
                                  {g.title}
                                </span>
                                <div className="flex items-center gap-2">
                                  <ArrowUpRight className="!size-5 text-muted-foreground group-hover/node-issue:text-highlight transition-colors stroke-[2]" />
                                </div>
                              </div>

                              {/* Error Items */}
                              <div className="bg-background/20 py-1.5">
                                {g.items.map((msg, idx) => (
                                  <div
                                    key={`err-${gi}-${idx}`}
                                    className="flex items-start gap-3 px-4 py-1"
                                  >
                                    <div className="w-1.5 h-1.5 rounded-full bg-red-500/50 mt-1.5 shrink-0" />
                                    <div className="text-[12px] leading-relaxed flex-1 text-foreground/70">
                                      {msg}
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>
                          </DropdownMenuItem>
                        );
                      })}
                    </div>
                  </div>
                )}

                {/* Warnings Section */}
                {hasWarnings && (
                  <div className="space-y-2">
                    <div className="px-3 py-1.5 flex items-center gap-2.5 text-amber-600 text-shadow-sm">
                      <div className="w-1.5 h-1.5 rounded-full bg-amber-600 shadow-[0_0_8px_rgba(217,119,6,0.5)]" />
                      <span className="text-xs font-black uppercase tracking-[0.2em] opacity-90">
                        {t('workflow.toolbar.section.warnings', { count: warnings.length })}
                      </span>
                    </div>
                    <div className="space-y-4">
                      {[...warningGroups.values()].map((g, gi) => {
                        const theme = g.type
                          ? NODE_THEMES[g.type as keyof typeof NODE_THEMES]
                          : NODE_THEMES.default;
                        const Icon = theme?.icon || TriangleAlert;

                        return (
                          <DropdownMenuItem
                            key={`warn-group-${gi}`}
                            onSelect={() => g.id && handleNodeClick(g.id)}
                            className="p-0 mb-4 last:mb-0 focus:bg-transparent hover:shadow-md rounded-2xl overflow-hidden border border-border/50 bg-card/10 hover:bg-muted/40 transition-all cursor-pointer group/node-issue block"
                          >
                            <div className="flex flex-col w-full">
                              {/* Node Header */}
                              <div className="px-3 py-1.5 flex items-center gap-3 border-b border-border/40 bg-muted/40">
                                <div
                                  className={cn(
                                    'p-1.5 rounded-lg shadow-sm flex items-center justify-center overflow-hidden transition-all',
                                    theme?.classNames?.iconBg ||
                                      'bg-background border border-border/60',
                                    theme?.classNames?.iconBg ? 'text-white' : 'text-foreground/70'
                                  )}
                                >
                                  {g.iconUrl ? (
                                    <ImageSafe
                                      src={g.iconUrl}
                                      className="w-3.5 h-3.5 rounded-full"
                                    />
                                  ) : (
                                    <Icon
                                      className="text-primary-foreground"
                                      size={14}
                                      strokeWidth={2.5}
                                    />
                                  )}
                                </div>
                                <span className="text-[11px] font-black text-foreground/80 uppercase tracking-widest leading-none flex-1 truncate">
                                  {g.title}
                                </span>
                                <div className="flex items-center gap-2">
                                  <ArrowUpRight className="!size-5 text-muted-foreground group-hover/node-issue:text-highlight transition-colors stroke-[2]" />
                                </div>
                              </div>

                              {/* Warning Items */}
                              <div className="bg-background/20 py-1">
                                {g.items.map((msg, idx) => (
                                  <div
                                    key={`warn-${gi}-${idx}`}
                                    className="flex items-start gap-3 px-4 py-1.5"
                                  >
                                    <div className="w-1.5 h-1.5 rounded-full bg-amber-500/50 mt-1.5 shrink-0" />
                                    <div className="text-[12px] leading-relaxed flex-1 text-foreground/70">
                                      {msg}
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>
                          </DropdownMenuItem>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            ) : (
              <div className="py-10 px-8 text-center bg-gradient-to-b from-emerald-50/20 to-transparent">
                <div className="w-16 h-16 bg-gradient-to-br from-emerald-50 to-emerald-100 dark:from-emerald-900/30 dark:to-emerald-800/20 text-emerald-600 rounded-[2rem] flex items-center justify-center mx-auto mb-4 border border-emerald-200/50 shadow-md shadow-emerald-500/10 transition-transform hover:scale-105 duration-500">
                  <CheckCircle className="w-8 h-8 stroke-[1.5]" />
                </div>
                <h3 className="text-xl font-black text-foreground mb-2 tracking-tight">
                  {t('workflow.toolbar.passedTitle')}
                </h3>
                <p className="text-sm text-muted-foreground font-semibold opacity-80">
                  {t('workflow.toolbar.noIssues')}
                </p>
              </div>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
};

export default WorkflowToolbar;
