# Connector service marks

- Exa: existing `modelicons/Exa` dependency, monochrome/currentColor variant.
- GitHub: existing `Icons.GitHub` asset; no global icon changes.
- Feishu / Lark: IconPark Lark by ByteDance, Apache-2.0. SVG geometry from the [IconPark SVG mirror](https://www.svgviewer.dev/s/29773/lark); [upstream catalog](https://github.com/bytedance/IconPark/blob/master/packages/svg/src/map.ts) and [license](https://github.com/bytedance/IconPark/blob/master/LICENSE). Converted to JSX, omitted transparent canvas rectangle, applied teal/blue fills. This is the IconPark rendition, not a newly designed logo.
- DingTalk, Weixin / WeChat, and WeCom: Ant Design Icons by Ant UED, MIT. SVG geometry from [DingTalk](https://github.com/ant-design/ant-design-icons/blob/master/packages/icons-svg/svg/outlined/dingtalk.svg), [WeChat](https://github.com/ant-design/ant-design-icons/blob/master/packages/icons-svg/svg/filled/wechat.svg), and [WeChat Work](https://github.com/ant-design/ant-design-icons/blob/master/packages/icons-svg/svg/outlined/wechat-work.svg), retrieved 2026-09-03. Converted to JSX with blue, green, and blue fills respectively; geometry and view boxes are preserved. See LICENSE-Ant-Design-Icons.txt for the bundled MIT notice.

Copyright 2019-present Bytedance Inc. See LICENSE-IconPark.txt. Service names and marks identify integrations; no endorsement is implied. All bundled marks render locally, without third-party image requests.
