import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import Exa from 'modelicons/Exa';
import { load, jsx, ui, harness, nodes, read } from './connector-fixture.mjs';
const require = createRequire(import.meta.url);
const model = load('src/components/connectors/connector-icon-model.ts');
const real = {
  'react/jsx-runtime': require('react/jsx-runtime'),
  'lucide-react': require('lucide-react'),
};
const icons = load('src/components/ui/icons.tsx', real);
const feishu = load('src/components/connectors/feishu-icon.tsx', real);
const messaging = load('src/components/connectors/messaging-icons.tsx', real);
const dependencies = {
  ...real,
  react: require('react'),
  'modelicons/Exa': { default: Exa },
  '@/components/ui/icons': icons,
  './feishu-icon': feishu,
  './messaging-icons': messaging,
  './connector-icon-model': model,
  '@/lib/utils': { cn: (...values) => values.filter(Boolean).join(' ') },
};
const { ConnectorIcon } = load('src/components/connectors/connector-icon.tsx', dependencies);
for (const [id, token, brand] of [
  ['web-search', 'globe-2', 'exa'],
  ['github', 'github', 'github'],
  ['feishu', 'feishu', 'feishu'],
  ['dingtalk', 'dingtalk', 'dingtalk'],
  ['weixin', 'weixin', 'weixin'],
  ['wecom', 'wecom', 'wecom'],
]) {
  assert.equal(model.connectorBrand(id, token), brand);
  assert.equal(model.connectorBrand(undefined, token), brand);
  assert.equal(model.safeConnectorIconSource(token), undefined);
  for (const size of ['sm', 'md', 'lg'])
    for (const theme of ['dark', 'light']) {
      const markup = renderToStaticMarkup(
        createElement(
          'div',
          { className: theme },
          createElement(ConnectorIcon, {
            connectorId: id,
            name: '服务名称',
            icon: token,
            classification: 'connector',
            size,
          })
        )
      );
      assert.match(markup, /<svg/);
      assert.doesNotMatch(markup, /<img|lucide-plug/);
      assert.match(markup, /aria-label="服务名称"/);
      assert.match(markup, /text-foreground/);
      const color =
        { feishu: '#00C5B1', dingtalk: '#0089FF', weixin: '#07C160', wecom: '#2E7BFA' }[brand] ??
        'currentColor';
      assert.ok(markup.includes(color));
    }
}
assert.equal(model.connectorBrand('unrelated', 'github'), undefined);
for (const value of [
  '',
  'javascript:alert(1)',
  'https://untrusted.test/x.svg',
  '//evil/x.svg',
  '/icons/../x.svg',
  '/icons/%2e%2e/x.svg',
  'globe-2',
])
  assert.equal(model.safeConnectorIconSource(value), undefined);
assert.equal(model.safeConnectorIconSource('/icons/local.svg'), '/icons/local.svg');
for (const classification of ['connector', 'data_source']) {
  const markup = renderToStaticMarkup(
    createElement(ConnectorIcon, { name: 'Unknown', icon: 'unsafe', classification })
  );
  assert.match(markup, classification === 'connector' ? /lucide-plug/ : /lucide-database/);
}
// Exercise the real local image component's load/error states without network or a browser.
const imageHarness = () =>
  harness(react => {
    const module = load(
      'src/components/connectors/connector-icon.tsx',
      { ...dependencies, react, 'react/jsx-runtime': jsx, 'lucide-react': ui },
      '\nexports.ImageFixture = ConnectorImage;'
    );
    return props =>
      module.ImageFixture({ className: 'size-7', fallback: 'visible fallback', ...props });
  });
let h = imageHarness();
h.render({ src: '/icons/one.svg' });
assert.match(JSON.stringify(h.value), /visible fallback/);
nodes(h.value)
  .find(n => n.type === 'img')
  .props.onLoad();
h.render({ src: '/icons/one.svg' });
assert.doesNotMatch(JSON.stringify(h.value), /visible fallback|opacity-0/);
nodes(h.value)
  .find(n => n.type === 'img')
  .props.onError();
h.render({ src: '/icons/one.svg' });
assert.match(JSON.stringify(h.value), /visible fallback/);
assert.equal(
  nodes(h.value).some(n => n.type === 'img'),
  false
);
h.unmount();
h = imageHarness();
h.render({ src: '/icons/two.svg' });
assert.equal(nodes(h.value).find(n => n.type === 'img').props.src, '/icons/two.svg');
assert.match(
  read('src/components/connectors/connector-icon.tsx'),
  /ConnectorImage\s+key=\{source\}/,
  'a new src gets a fresh load/error state'
);
assert.doesNotMatch(
  read('src/components/common/image-safe/index.tsx'),
  /connectorId|web-search|feishu/
);
assert.match(
  read('src/components/workflow/ui/node-catalog/tool-catalog-list.tsx'),
  /provider.type === 'connector'/
);
console.log(
  'Connector actual brand renders, fallback/loading/error/reset and isolated host icons passed'
);
