import assert from 'node:assert/strict';
import { mergeCurrentWebApp } from '../src/app/console/work/app/sidebar-items.ts';

const app = web_app_id => ({ web_app_id });
const ids = items => items.map(item => item.web_app_id);

assert.deepEqual(
  ids(mergeCurrentWebApp([app('one'), app('current'), app('current'), app('three')], app('current'))),
  ['one', 'current', 'three'],
  'an existing current app must keep its original position and be deduplicated'
);

assert.deepEqual(
  ids(mergeCurrentWebApp([app('one'), app('two')], app('current'))),
  ['one', 'two', 'current'],
  'a missing current app must be appended instead of moved to the front'
);

assert.deepEqual(
  ids(
    mergeCurrentWebApp(
      [app('one'), app('two'), app('three'), app('four'), app('five'), app('six')],
      app('current'),
      { limit: 6 }
    )
  ),
  ['one', 'two', 'three', 'four', 'five', 'current'],
  'the collapsed list must reserve its last slot for a missing current app'
);

assert.deepEqual(
  ids(
    mergeCurrentWebApp(
      [app('one'), app('two'), app('three'), app('four'), app('five'), app('six'), app('current')],
      app('current'),
      { limit: 6 }
    )
  ),
  ['one', 'two', 'three', 'four', 'five', 'current'],
  'the current app must remain visible when it falls outside the collapsed limit'
);

assert.deepEqual(
  ids(
    mergeCurrentWebApp([app('matching-result')], app('current'), {
      includeCurrent: false,
    })
  ),
  ['matching-result'],
  'search results must not be polluted with a non-matching current app'
);

console.log('App sidebar current-item merge checks passed.');
