import type { LucideIcon } from 'lucide-react';
import { BellRing, Mail, Smartphone } from 'lucide-react';
import type {
  CreateScheduledTaskActionType,
  CreateScheduledTaskChannelType,
} from './config';

export interface ScheduledTaskActionRegistryItem {
  value: CreateScheduledTaskActionType;
  icon: LucideIcon;
  labelKey: string;
  descriptionKey: string;
  channelTypes: CreateScheduledTaskChannelType[];
}

export interface ScheduledTaskChannelRegistryItem {
  value: CreateScheduledTaskChannelType;
  icon: LucideIcon;
  labelKey: string;
  descriptionKey: string;
}

export const scheduledTaskActionRegistry: Record<
  CreateScheduledTaskActionType,
  ScheduledTaskActionRegistryItem
> = {
  send_notification: {
    value: 'send_notification',
    icon: BellRing,
    labelKey: 'createScheduledTask.actionTypeSendNotification',
    descriptionKey: 'createScheduledTask.help.sendNotificationAction',
    channelTypes: ['email', 'sms'],
  },
  // Future expansion:
  // run_workflow: {
  //   value: 'run_workflow',
  //   icon: Workflow,
  //   labelKey: 'createScheduledTask.actionTypeRunWorkflow',
  //   descriptionKey: 'createScheduledTask.help.runWorkflowAction',
  //   channelTypes: ['webhook'],
  // },
  run_workflow: {
    value: 'run_workflow',
    icon: BellRing,
    labelKey: 'createScheduledTask.actionTypeRunWorkflow',
    descriptionKey: 'createScheduledTask.help.runWorkflowAction',
    channelTypes: ['webhook'],
  },
};

export const scheduledTaskChannelRegistry: Record<
  CreateScheduledTaskChannelType,
  ScheduledTaskChannelRegistryItem
> = {
  email: {
    value: 'email',
    icon: Mail,
    labelKey: 'createScheduledTask.channelTypeEmail',
    descriptionKey: 'createScheduledTask.help.emailChannel',
  },
  sms: {
    value: 'sms',
    icon: Smartphone,
    labelKey: 'createScheduledTask.channelTypeSms',
    descriptionKey: 'createScheduledTask.help.smsChannel',
  },
  // Future expansion:
  // webhook: {
  //   value: 'webhook',
  //   icon: Webhook,
  //   labelKey: 'createScheduledTask.channelTypeWebhook',
  //   descriptionKey: 'createScheduledTask.help.webhookChannel',
  // },
  webhook: {
    value: 'webhook',
    icon: Mail,
    labelKey: 'createScheduledTask.channelTypeWebhook',
    descriptionKey: 'createScheduledTask.help.webhookChannel',
  },
};

export const scheduledTaskActionOptions = [scheduledTaskActionRegistry.send_notification];
export const scheduledTaskChannelOptions = [scheduledTaskChannelRegistry.email];
