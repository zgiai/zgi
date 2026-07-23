import assert from 'node:assert/strict';

import {
  DOCUMENT_EXTENSIONS,
  IMAGE_EXTENSIONS,
  getEffectiveChatUploadExtensions,
} from '../src/utils/file-helpers.ts';

const documentParserExtensions = ['pdf', 'docx', 'xlsx'];

assert.deepEqual(
  getEffectiveChatUploadExtensions(['image'], [], documentParserExtensions),
  [...IMAGE_EXTENSIONS],
  'image uploads must not be removed by the document parser capability list'
);

assert.deepEqual(
  getEffectiveChatUploadExtensions(['document'], [], documentParserExtensions),
  documentParserExtensions,
  'document uploads should follow the server document parser capability list'
);

assert.deepEqual(
  getEffectiveChatUploadExtensions(['image', 'document'], [], documentParserExtensions),
  [...IMAGE_EXTENSIONS, ...documentParserExtensions],
  'mixed upload categories should preserve media and supported document extensions'
);

assert.deepEqual(
  getEffectiveChatUploadExtensions(['custom'], ['HEIC', '.PSD'], documentParserExtensions),
  ['heic', 'psd'],
  'custom upload extensions should follow the workflow configuration'
);

assert.deepEqual(
  getEffectiveChatUploadExtensions(['document'], [], []),
  [...DOCUMENT_EXTENSIONS],
  'document uploads should retain the local fallback while capabilities are unavailable'
);

console.log('Chat upload extension policy checks passed.');
