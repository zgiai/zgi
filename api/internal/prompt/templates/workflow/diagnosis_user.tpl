Node [{{.NodeName}}] execution failed.
==========
[Error Message]
{{.ErrorMessage}}
==========
[Context Information]
- Error Type: {{.ErrorType}}
- Node Type: {{.NodeType}}
- Node Configuration:
{{.NodeYAML}}
{{if .UpstreamContexts}}
- Related Upstream Nodes:
{{range $nodeId, $ctx := .UpstreamContexts}}
  * Node [{{$nodeId}}]:
    - Configuration:
{{$ctx.YAML}}
    - Output:
{{$ctx.Output}}
{{end}}
{{end}}
{{if .InputSnapshot}}
- Runtime Input Variables:
{{.InputSnapshot}}
{{end}}
==========
Please answer in English. Strictly keep the length under 100 words.
Your answer must be clearly structured and cover:
1. Problem Location
2. Probable Cause
3. Fix Suggestions
