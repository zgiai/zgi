import React from 'react';
import { useT } from '@/i18n';
import { NODE_THEMES } from '../../../nodes/custom/config';
import { NODE_TYPES } from '../../../nodes';
import { useAuthStore } from '@/store/auth-store';
import { isNotificationSMSWorkflowNodeEnabled } from '@/lib/features/notification-sms';

export type NodeGroupKey = 'flow' | 'ai' | 'data' | 'tool';

export interface NodeType {
  type: string;
  title: string;
  description: string;
  icon: React.ReactNode;
  bgColor: string;
  // Whether the node has both input and output handles, eligible for edge insertion
  io: boolean;
  // Group for catalog display
  group: NodeGroupKey;
}

export function useNodeTypesI18n(): NodeType[] {
  const t = useT('nodes');
  const iconSize = 18;
  const systemFeatures = useAuthStore.use.systemFeatures();
  const notificationSMSEnabled = isNotificationSMSWorkflowNodeEnabled(systemFeatures);

  return React.useMemo(() => {
    const types: NodeType[] = [
      {
        type: NODE_TYPES.START,
        title: t('catalog.start.title'),
        description: t('catalog.start.description'),
        icon: React.createElement(NODE_THEMES.start.icon, { size: iconSize }),
        bgColor: NODE_THEMES.start.classNames.iconBg ?? 'bg-green-500',
        io: false,
        group: 'flow',
      },
      {
        type: NODE_TYPES.VARIABLE_AGGREGATOR,
        title: t('catalog.variable-aggregator.title'),
        description: t('catalog.variable-aggregator.description'),
        icon: React.createElement(NODE_THEMES['variable-aggregator'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['variable-aggregator'].classNames.iconBg ?? 'bg-orange-500',
        io: true,
        group: 'data',
      },
      {
        type: NODE_TYPES.PARAMETER_EXTRACTOR,
        title: t('catalog.parameter-extractor.title'),
        description: t('catalog.parameter-extractor.description'),
        icon: React.createElement(NODE_THEMES['parameter-extractor'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['parameter-extractor'].classNames.iconBg ?? 'bg-rose-500',
        io: true,
        group: 'ai',
      },
      {
        type: NODE_TYPES.KNOWLEDGE_RETRIEVAL,
        title: t('catalog.knowledge-retrieval.title'),
        description: t('catalog.knowledge-retrieval.description'),
        icon: React.createElement(NODE_THEMES['knowledge-retrieval'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['knowledge-retrieval'].classNames.iconBg ?? 'bg-rose-500',
        io: true,
        group: 'ai',
      },
      {
        type: NODE_TYPES.LLM,
        title: t('catalog.llm.title'),
        description: t('catalog.llm.description'),
        icon: React.createElement(NODE_THEMES.llm.icon, { size: iconSize }),
        bgColor: NODE_THEMES.llm.classNames.iconBg ?? 'bg-blue-600',
        io: true,
        group: 'ai',
      },
      {
        type: NODE_TYPES.IMAGE_GEN,
        title: t('catalog.image-gen.title'),
        description: t('catalog.image-gen.description'),
        icon: React.createElement(NODE_THEMES['image-gen'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['image-gen'].classNames.iconBg ?? 'bg-purple-500',
        io: true,
        group: 'ai',
      },
      {
        type: NODE_TYPES.HTTP_REQUEST,
        title: t('catalog.http-request.title'),
        description: t('catalog.http-request.description'),
        icon: React.createElement(NODE_THEMES['http-request'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['http-request'].classNames.iconBg ?? 'bg-slate-500',
        io: true,
        group: 'tool',
      },
      {
        type: NODE_TYPES.CREATE_SCHEDULED_TASK,
        title: t('catalog.create-scheduled-task.title'),
        description: t('catalog.create-scheduled-task.description'),
        icon: React.createElement(NODE_THEMES['create-scheduled-task'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['create-scheduled-task'].classNames.iconBg ?? 'bg-indigo-500',
        io: true,
        group: 'tool',
      },
      ...(notificationSMSEnabled
        ? [
            {
              type: NODE_TYPES.NOTIFICATION_SMS,
              title: t('catalog.notification-sms.title'),
              description: t('catalog.notification-sms.description'),
              icon: React.createElement(NODE_THEMES['notification-sms'].icon, { size: iconSize }),
              bgColor: NODE_THEMES['notification-sms'].classNames.iconBg ?? 'bg-slate-500',
              io: true,
              group: 'tool' as const,
            },
          ]
        : []),
      {
        type: NODE_TYPES.ANNOUNCEMENT,
        title: t('catalog.announcement.title'),
        description: t('catalog.announcement.description'),
        icon: React.createElement(NODE_THEMES.announcement.icon, { size: iconSize }),
        bgColor: NODE_THEMES.announcement.classNames.iconBg ?? 'bg-slate-500',
        io: true,
        group: 'tool',
      },
      {
        type: NODE_TYPES.CALL_DATABASE,
        title: t('catalog.call-database.title'),
        description: t('catalog.call-database.description'),
        icon: React.createElement(NODE_THEMES['call-database'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['call-database'].classNames.iconBg ?? 'bg-slate-500',
        io: true,
        group: 'tool',
      },
      {
        type: NODE_TYPES.SQL_GENERATOR,
        title: t('catalog.sql-generator.title'),
        description: t('catalog.sql-generator.description'),
        icon: React.createElement(NODE_THEMES['sql-generator'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['sql-generator'].classNames.iconBg ?? 'bg-slate-500',
        io: true,
        group: 'tool',
      },
      {
        type: NODE_TYPES.CODE,
        title: t('catalog.code.title'),
        description: t('catalog.code.description'),
        icon: React.createElement(NODE_THEMES.code.icon, { size: iconSize }),
        bgColor: NODE_THEMES.code.classNames.iconBg ?? 'bg-slate-500',
        io: true,
        group: 'tool',
      },
      {
        type: NODE_TYPES.IF_ELSE,
        title: t('catalog.if-else.title'),
        description: t('catalog.if-else.description'),
        icon: React.createElement(NODE_THEMES['if-else'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['if-else'].classNames.iconBg ?? 'bg-teal-500',
        io: true,
        group: 'flow',
      },
      {
        type: NODE_TYPES.APPROVAL,
        title: t('catalog.approval.title'),
        description: t('catalog.approval.description'),
        icon: React.createElement(NODE_THEMES.approval.icon, { size: iconSize }),
        bgColor: NODE_THEMES.approval.classNames.iconBg ?? 'bg-amber-500',
        io: true,
        group: 'flow',
      },
      {
        type: NODE_TYPES.QUESTION_ANSWER,
        title: t('catalog.question-answer.title'),
        description: t('catalog.question-answer.description'),
        icon: React.createElement(NODE_THEMES['question-answer'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['question-answer'].classNames.iconBg ?? 'bg-blue-500',
        io: true,
        group: 'flow',
      },
      {
        type: NODE_TYPES.ITERATION,
        title: t('catalog.iteration.title'),
        description: t('catalog.iteration.description'),
        icon: React.createElement(NODE_THEMES.iteration.icon, { size: iconSize }),
        bgColor: NODE_THEMES.iteration.classNames.iconBg ?? 'bg-purple-600',
        io: true,
        group: 'flow',
      },
      {
        type: NODE_TYPES.LOOP,
        title: t('catalog.loop.title'),
        description: t('catalog.loop.description'),
        icon: React.createElement(NODE_THEMES.loop.icon, { size: iconSize }),
        bgColor: NODE_THEMES.loop.classNames.iconBg ?? 'bg-blue-500',
        io: true,
        group: 'flow',
      },
      {
        type: NODE_TYPES.ASSIGNER,
        title: t('catalog.assigner.title'),
        description: t('catalog.assigner.description'),
        icon: React.createElement(NODE_THEMES.assigner.icon, { size: iconSize }),
        bgColor: NODE_THEMES.assigner.classNames.iconBg ?? 'bg-orange-500',
        io: true,
        group: 'data',
      },
      {
        type: NODE_TYPES.ANSWER,
        title: t('catalog.answer.title'),
        description: t('catalog.answer.description'),
        icon: React.createElement(NODE_THEMES.answer.icon, { size: iconSize }),
        bgColor: NODE_THEMES.answer.classNames.iconBg ?? 'bg-cyan-500',
        io: true,
        group: 'flow',
      },
      {
        type: NODE_TYPES.DOCUMENT_EXTRACTOR,
        title: t('catalog.document-extractor.title'),
        description: t('catalog.document-extractor.description'),
        icon: React.createElement(NODE_THEMES['document-extractor'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['document-extractor'].classNames.iconBg ?? 'bg-orange-500',
        io: true,
        group: 'data',
      },
      {
        type: NODE_TYPES.JSON_PARSER,
        title: t('catalog.json-parser.title'),
        description: t('catalog.json-parser.description'),
        icon: React.createElement(NODE_THEMES['json-parser'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['json-parser'].classNames.iconBg ?? 'bg-orange-500',
        io: true,
        group: 'data',
      },
      {
        type: NODE_TYPES.END,
        title: t('catalog.end.title'),
        description: t('catalog.end.description'),
        icon: React.createElement(NODE_THEMES.end.icon, { size: iconSize }),
        bgColor: NODE_THEMES.end.classNames.iconBg ?? 'bg-red-500',
        io: false,
        group: 'flow',
      },
      {
        type: NODE_TYPES.LOOP_END,
        title: t('catalog.loop-end.title'),
        description: t('catalog.loop-end.description'),
        icon: React.createElement(NODE_THEMES['loop-end'].icon, { size: iconSize }),
        bgColor: NODE_THEMES['loop-end'].classNames.iconBg ?? 'bg-teal-500',
        io: true,
        group: 'flow',
      },
    ];

    return types;
  }, [notificationSMSEnabled, t]);
}
