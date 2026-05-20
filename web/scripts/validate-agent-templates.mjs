/* eslint-env node */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parse } from 'yaml';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..');
const manifestPath = path.join(rootDir, 'src/components/agents/templates/template-manifest.ts');
const templatesDir = path.join(rootDir, 'public/templates/agents');
const locales = ['en-US', 'zh-Hans'];
const expectedNodeTypes = new Set([
  'answer',
  'approval',
  'assigner',
  'call-database',
  'code',
  'create-scheduled-task',
  'document-extractor',
  'end',
  'http-request',
  'if-else',
  'image-gen',
  'iteration',
  'iteration-start',
  'json-parser',
  'knowledge-retrieval',
  'llm',
  'loop',
  'loop-end',
  'loop-start',
  'note',
  'notification-sms',
  'parameter-extractor',
  'sql-generator',
  'start',
  'tools',
  'variable-aggregator',
]);

const localeModel = {
  'en-US': {
    provider: 'openai',
    name: 'gpt-5.5',
    noteKeyword: 'Run guide',
    debugKeyword: 'Suggested test input',
  },
  'zh-Hans': {
    provider: 'openai',
    name: 'gpt-5.5',
    noteKeyword: '运行说明',
    debugKeyword: '调试输入',
  },
};

const imageModel = { provider: 'qwen', name: 'qwen-image-2.0' };
const cjkPattern = /[\u4e00-\u9fff]/;
const zhHansDisallowedPhrases = [
  'Please review',
  'Review notes',
  'Approve',
  'Revise',
  'Reject',
  'Please open the link',
  'Owner email',
  'Reason for follow-up',
  'Follow-up reminder',
  'Notify the owner',
  'Workflow follow-up required',
  'Please review the workflow output',
  'Follow-up required',
  'Configure Data Source',
  'Generate a safe read-only SQL query',
  'Create an INSERT or UPDATE SQL statement',
  'Urgent workflow alert',
  'Plan created',
  'I need a clearer',
  'This looks urgent',
  'Suggested next step',
  'This can follow the standard service path',
  'Configure the HTTP endpoint.',
];
const errors = [];

function fail(message) {
  errors.push(message);
}

function readText(filePath) {
  return fs.readFileSync(filePath, 'utf8');
}

function parseManifestTemplateFiles() {
  const manifest = readText(manifestPath);
  const matches = [...manifest.matchAll(/templateYaml\('([^']+)'\)/g)].map(match => match[1]);
  const unique = new Set(matches);

  if (matches.length === 0) {
    fail('No templateYaml(...) entries found in template manifest.');
  }

  if (matches.length !== unique.size) {
    fail('Template manifest contains duplicate template file names.');
  }

  return matches;
}

function listLocaleFiles(locale) {
  const localeDir = path.join(templatesDir, locale);
  return fs
    .readdirSync(localeDir)
    .filter(fileName => fileName.endsWith('.yml'))
    .sort();
}

function validateManifestFiles(fileNames) {
  const rootFiles = fs
    .readdirSync(templatesDir, { withFileTypes: true })
    .filter(entry => entry.isFile() && entry.name.endsWith('.yml'))
    .map(entry => entry.name);

  if (rootFiles.length > 0) {
    fail(`Template YAML files must live under locale folders: ${rootFiles.join(', ')}`);
  }

  for (const locale of locales) {
    const localeFiles = listLocaleFiles(locale);
    const localeSet = new Set(localeFiles);

    for (const fileName of fileNames) {
      if (!localeSet.has(fileName)) {
        fail(`${locale}/${fileName} is missing.`);
      }
    }

    for (const fileName of localeFiles) {
      if (!fileNames.includes(fileName)) {
        fail(`${locale}/${fileName} is not referenced by the manifest.`);
      }
    }
  }
}

function validateNodeModels(locale, fileName, nodes) {
  const model = localeModel[locale];

  for (const node of nodes) {
    const nodeType = node?.data?.type;

    if (nodeType === 'llm') {
      const nodeModel = node.data.model;
      if (nodeModel?.provider !== model.provider || nodeModel?.name !== model.name) {
        fail(
          `${locale}/${fileName} node ${node.id} should use ${model.provider}/${model.name}, got ${nodeModel?.provider}/${nodeModel?.name}.`
        );
      }
    }

    if (nodeType === 'image-gen') {
      const nodeModel = node.data.model;
      if (nodeModel?.provider !== imageModel.provider || nodeModel?.name !== imageModel.name) {
        fail(
          `${locale}/${fileName} node ${node.id} should use ${imageModel.provider}/${imageModel.name}, got ${nodeModel?.provider}/${nodeModel?.name}.`
        );
      }
    }
  }
}

function validateParameterExtractorNodes(locale, fileName, nodes) {
  const supportedParameterTypes = new Set(['string', 'number', 'select']);

  for (const node of nodes) {
    if (node?.data?.type !== 'parameter-extractor') continue;

    const parameters = Array.isArray(node.data.parameters) ? node.data.parameters : [];
    for (const parameter of parameters) {
      if (!supportedParameterTypes.has(parameter.type)) {
        fail(
          `${locale}/${fileName} parameter-extractor node ${node.id} uses unsupported parameter type ${parameter.type} for ${parameter.name}.`
        );
      }
    }
  }
}

function validateRunGuide(locale, fileName, doc, nodes) {
  const notes = nodes.filter(node => node?.data?.type === 'note');
  const noteText = notes
    .map(node => `${node.data.title ?? ''}\n${node.data.text ?? ''}`)
    .join('\n');
  const model = localeModel[locale];
  const hasLlmNode = nodes.some(node => node?.data?.type === 'llm');

  if (!noteText.includes(model.noteKeyword)) {
    fail(`${locale}/${fileName} is missing a localized run guide note.`);
  }

  if (!noteText.includes(model.debugKeyword)) {
    fail(`${locale}/${fileName} run guide note is missing a debug input section.`);
  }

  if (hasLlmNode && !noteText.includes(model.name)) {
    fail(`${locale}/${fileName} run guide note does not mention ${model.name}.`);
  }

  if (doc?.app?.mode === 'CONVERSATIONAL_WORKFLOW') {
    const suggestedQuestions = doc?.workflow?.features?.suggested_questions;
    const firstQuestion = Array.isArray(suggestedQuestions) ? suggestedQuestions[0] : null;

    if (typeof firstQuestion !== 'string' || firstQuestion.trim().length === 0) {
      fail(`${locale}/${fileName} conversational template needs a suggested debug question.`);
      return;
    }

    if (!noteText.includes(firstQuestion)) {
      fail(`${locale}/${fileName} run guide note must include the suggested debug question.`);
    }
  }
}

function validateStartDefaults(locale, fileName, nodes) {
  const starts = nodes.filter(node => node?.data?.type === 'start');

  for (const start of starts) {
    const variables = Array.isArray(start?.data?.variables) ? start.data.variables : [];
    for (const variable of variables) {
      if (variable.type === 'file-list') continue;
      if (variable.default === undefined || variable.default === '') {
        fail(
          `${locale}/${fileName} start variable ${variable.variable} is missing a debug default.`
        );
      }
    }
  }
}

function collectStringValues(value, pathParts = [], values = []) {
  if (typeof value === 'string') {
    values.push({ path: pathParts.join('.'), value });
    return values;
  }

  if (Array.isArray(value)) {
    value.forEach((item, index) => collectStringValues(item, [...pathParts, String(index)], values));
    return values;
  }

  if (value && typeof value === 'object') {
    for (const [key, child] of Object.entries(value)) {
      collectStringValues(child, [...pathParts, key], values);
    }
  }

  return values;
}

function validateLocaleText(locale, fileName, doc) {
  const stringValues = collectStringValues(doc);

  if (locale === 'en-US') {
    for (const { path: valuePath, value } of stringValues) {
      if (cjkPattern.test(value)) {
        fail(`${locale}/${fileName} contains non-English text at ${valuePath}.`);
      }
    }
    return;
  }

  if (locale === 'zh-Hans') {
    for (const { path: valuePath, value } of stringValues) {
      for (const phrase of zhHansDisallowedPhrases) {
        if (value.includes(phrase)) {
          fail(`${locale}/${fileName} contains English UI/debug text "${phrase}" at ${valuePath}.`);
        }
      }
    }
  }
}

function validateGraph(locale, fileName, doc) {
  const nodes = doc?.workflow?.graph?.nodes;
  const edges = doc?.workflow?.graph?.edges;

  if (!doc?.app?.name) {
    fail(`${locale}/${fileName} is missing app.name.`);
  }

  if (!Array.isArray(nodes) || nodes.length === 0) {
    fail(`${locale}/${fileName} has no workflow graph nodes.`);
    return new Set();
  }

  if (!Array.isArray(edges)) {
    fail(`${locale}/${fileName} workflow graph edges must be an array.`);
  }

  const nodeIds = new Set();
  const nodeTypes = new Set();

  for (const node of nodes) {
    if (!node?.id) {
      fail(`${locale}/${fileName} has a node without id.`);
      continue;
    }

    if (nodeIds.has(node.id)) {
      fail(`${locale}/${fileName} has duplicate node id ${node.id}.`);
    }
    nodeIds.add(node.id);

    if (!node?.data?.type) {
      fail(`${locale}/${fileName} node ${node.id} is missing data.type.`);
    } else {
      nodeTypes.add(node.data.type);
    }
  }

  validateRunGuide(locale, fileName, doc, nodes);
  validateStartDefaults(locale, fileName, nodes);
  validateNodeModels(locale, fileName, nodes);
  validateParameterExtractorNodes(locale, fileName, nodes);
  validateLocaleText(locale, fileName, doc);

  return nodeTypes;
}

function validateYamlFiles(fileNames) {
  const nodeTypesByLocale = new Map(locales.map(locale => [locale, new Set()]));

  for (const locale of locales) {
    for (const fileName of fileNames) {
      const filePath = path.join(templatesDir, locale, fileName);
      let doc;

      try {
        doc = parse(readText(filePath));
      } catch (error) {
        fail(`${locale}/${fileName} is not valid YAML: ${error.message}`);
        continue;
      }

      const nodeTypes = validateGraph(locale, fileName, doc);
      const localeTypes = nodeTypesByLocale.get(locale);
      for (const nodeType of nodeTypes) {
        localeTypes.add(nodeType);
      }
    }
  }

  for (const locale of locales) {
    const actualTypes = nodeTypesByLocale.get(locale);
    for (const expectedType of expectedNodeTypes) {
      if (!actualTypes.has(expectedType)) {
        fail(`${locale} templates do not cover node type ${expectedType}.`);
      }
    }
  }

  return nodeTypesByLocale;
}

const fileNames = parseManifestTemplateFiles();
validateManifestFiles(fileNames);
const nodeTypesByLocale = validateYamlFiles(fileNames);

if (errors.length > 0) {
  console.error(`Agent template validation failed with ${errors.length} issue(s):`);
  for (const error of errors) {
    console.error(`- ${error}`);
  }
  process.exit(1);
}

console.log(
  `Agent template validation passed: ${fileNames.length} templates x ${locales.length} locales.`
);
for (const [locale, nodeTypes] of nodeTypesByLocale.entries()) {
  console.log(`${locale} node coverage: ${[...nodeTypes].sort().join(', ')}`);
}
