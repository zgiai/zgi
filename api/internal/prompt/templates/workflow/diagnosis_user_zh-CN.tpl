节点 [{{.NodeName}}] 运行失败。
==========
【报错信息】
{{.ErrorMessage}}
==========
【上下文信息】
- 错误类型：{{.ErrorType}}
- 当前节点类型：{{.NodeType}}
- 当前节点配置：
{{.NodeYAML}}
{{if .UpstreamContexts}}
- 相关上游节点：
{{range $nodeId, $ctx := .UpstreamContexts}}
  * 节点 [{{$nodeId}}]:
    - 配置：
{{$ctx.YAML}}
    - 输出：
{{$ctx.Output}}
{{end}}
{{end}}
{{if .InputSnapshot}}
- 节点实时输入变量：
{{.InputSnapshot}}
{{end}}
==========
请使用中文回答，字数严格控制在 200 字以内。
回答必须结构清晰，包含三点含义：
1. 问题定位
2. 可能原因
3. 修复建议
