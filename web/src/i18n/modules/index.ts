import common from './common/en-US';
import navigation from './navigation/en-US';
import auth from './auth/en-US';
import users from './users/en-US';
import dashboard from './dashboard/en-US';
import settings from './settings/en-US';
import aiProviders from './aiProviders/en-US';
import models from './models/en-US';
import ui from './ui/en-US';
import datasets from './datasets/en-US';
import dbs from './dbs/en-US';
import agents from './agents/en-US';
import nodes from './nodes/en-US';
import files from './files/en-US';
import webapp from './webapp/en-US';
import profile from './profile/en-US';
import channels from './channels/en-US';
import apikeys from './apikeys/en-US';
import market from './market/en-US';
import workspace from './workspace/en-US';
import automation from './automation/en-US';
import contentParse from './contentParse/en-US';
import prompts from './prompts/en-US';

/**
 * Static messages type derived from English (en-US) files.
 * This is used for IDE auto-completion and type checking.
 * Using en-US as the base ensures all keys are accounted for in the primary international language.
 */
export const messages = {
  common,
  navigation,
  auth,
  users,
  dashboard,
  settings,
  aiProviders,
  models,
  ui,
  datasets,
  dbs,
  agents,
  nodes,
  files,
  webapp,
  profile,
  channels,
  apikeys,
  market,
  workspace,
  automation,
  contentParse,
  prompts,
} as const;

export type Messages = typeof messages;
