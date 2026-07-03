import { readFileSync } from 'node:fs';

const source = readFileSync('src/app/console/dataset/[datasetId]/settings/page.tsx', 'utf8');

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const titleIndex = source.indexOf("t('datasets.settingsTitle')");
const saveButtonIndex = source.indexOf('<Button', titleIndex);
const headerSection = source.slice(titleIndex, saveButtonIndex);
const basicInfoTitleIndex = source.indexOf("t('datasets.settings.basicInfo')");
const cardContentIndex = source.indexOf('<CardContent', basicInfoTitleIndex);
const basicInfoHeader = source.slice(basicInfoTitleIndex, cardContentIndex);

assert(titleIndex !== -1, 'Dataset settings page should render the settings title.');
assert(basicInfoTitleIndex !== -1, 'Dataset settings page should render the basic info card title.');
assert(
  !headerSection.includes("t('datasets.settingsDescription')"),
  'Settings description should not render below the page title.'
);
assert(
  basicInfoHeader.includes("t('datasets.settingsDescription')"),
  'Settings description should render below the basic info title.'
);

console.log('Dataset settings basic info description placement check passed.');
