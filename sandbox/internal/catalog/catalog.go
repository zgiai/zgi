package catalog

type Blueprint struct {
	ProductLabel string   `json:"product_label"`
	Subtitle     string   `json:"subtitle"`
	Highlights   []string `json:"highlights"`
	Runtimes     []Panel  `json:"runtimes"`
	Modules      []Panel  `json:"modules"`
	Phases       []Panel  `json:"phases"`
	Navigation   []Nav    `json:"navigation"`
}

type Nav struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Panel struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Category     string   `json:"category"`
	Summary      string   `json:"summary"`
	Capabilities []string `json:"capabilities"`
	Actions      []string `json:"actions"`
	Metrics      []Metric `json:"metrics"`
}

type Metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func DefaultBlueprint() Blueprint {
	return Blueprint{
		ProductLabel: "zgi-sandbox",
		Subtitle:     "A native ZGI execution plane with product-grade control, runtime visibility, and iterative delivery built into one service.",
		Highlights: []string{
			"Compatible /v1/sandbox/run entrypoint",
			"Session and interactive runtimes mapped for later phases",
			"Clickable workspace with right-side inspector binding",
		},
		Navigation: []Nav{
			{ID: "runtime-lite", Label: "Lite Runtime"},
			{ID: "runtime-session", Label: "Session Runtime"},
			{ID: "runtime-interactive", Label: "Interactive Runtime"},
			{ID: "module-compat", Label: "Compat Gateway"},
			{ID: "module-lifecycle", Label: "Lifecycle Manager"},
			{ID: "module-exec", Label: "Exec Plane"},
			{ID: "module-policy", Label: "Policy Layer"},
			{ID: "module-observer", Label: "Observer"},
			{ID: "phase-1", Label: "Phase 1"},
			{ID: "playground", Label: "Execution Lab"},
		},
		Runtimes: []Panel{
			{
				ID:       "runtime-lite",
				Label:    "Lite Runtime",
				Category: "Runtime",
				Summary:  "Single-shot execution for workflow code nodes with fast startup and a deliberately small surface area.",
				Capabilities: []string{
					"Python and Node.js execution",
					"Timeout and output caps",
					"Compatibility payload support",
				},
				Actions: []string{
					"Serve /v1/sandbox/run",
					"Replace the current external execution dependency",
					"Keep the upstream protocol stable",
				},
				Metrics: []Metric{
					{Label: "Launch model", Value: "process"},
					{Label: "TTL", Value: "request scoped"},
					{Label: "Primary goal", Value: "fast replacement"},
				},
			},
			{
				ID:       "runtime-session",
				Label:    "Session Runtime",
				Category: "Runtime",
				Summary:  "Reusable execution state for multi-step workflows, shell tools, and artifact-oriented jobs.",
				Capabilities: []string{
					"Repeated code execution",
					"Command and file workflows",
					"Workflow-run binding",
				},
				Actions: []string{
					"Allocate sandbox sessions",
					"Attach metadata and TTL",
					"Export artifacts at run completion",
				},
				Metrics: []Metric{
					{Label: "Launch model", Value: "container"},
					{Label: "TTL", Value: "workflow scoped"},
					{Label: "Primary goal", Value: "tool chaining"},
				},
			},
			{
				ID:       "runtime-interactive",
				Label:    "Interactive Runtime",
				Category: "Runtime",
				Summary:  "Longer-lived, endpoint-aware runtime for coding agents, browser agents, and preview services.",
				Capabilities: []string{
					"Port exposure",
					"Endpoint discovery",
					"Renewable expiration",
				},
				Actions: []string{
					"Expose sandbox endpoints",
					"Apply egress rules",
					"Offer stronger isolation classes",
				},
				Metrics: []Metric{
					{Label: "Launch model", Value: "container or microVM"},
					{Label: "TTL", Value: "renewable"},
					{Label: "Primary goal", Value: "agent workspace"},
				},
			},
		},
		Modules: []Panel{
			{
				ID:       "module-compat",
				Label:    "Compat Gateway",
				Category: "Module",
				Summary:  "The compatibility edge that keeps `zgi-api` stable while the sandbox implementation changes underneath.",
				Capabilities: []string{
					"Health endpoint",
					"Run endpoint",
					"Dependency profile endpoints",
				},
				Actions: []string{
					"Translate incoming execution requests",
					"Attach upstream context metadata",
					"Return stable response payloads",
				},
				Metrics: []Metric{
					{Label: "Main contract", Value: "/v1/sandbox/run"},
					{Label: "Audience", Value: "zgi-api"},
					{Label: "Risk focus", Value: "protocol drift"},
				},
			},
			{
				ID:       "module-lifecycle",
				Label:    "Lifecycle Manager",
				Category: "Module",
				Summary:  "The control layer that owns sandbox identity, runtime profile selection, and expiration control.",
				Capabilities: []string{
					"Create and delete sandboxes",
					"Assign runtime profiles",
					"Resolve endpoint metadata",
				},
				Actions: []string{
					"Track active sandboxes",
					"Apply resource policies",
					"Renew interactive sessions",
				},
				Metrics: []Metric{
					{Label: "Resource type", Value: "sandbox"},
					{Label: "Lifecycle mode", Value: "managed"},
					{Label: "Phase target", Value: "V2+"},
				},
			},
			{
				ID:       "module-exec",
				Label:    "Exec Plane",
				Category: "Module",
				Summary:  "The execution surface for code, commands, files, and runtime logs within an allocated sandbox.",
				Capabilities: []string{
					"Code execution",
					"Command execution",
					"File and artifact flows",
				},
				Actions: []string{
					"Run user code with timeout control",
					"Capture stdout and stderr",
					"Stream execution results back into the UI",
				},
				Metrics: []Metric{
					{Label: "Current state", Value: "live preview"},
					{Label: "Languages", Value: "python3, nodejs"},
					{Label: "Phase target", Value: "V1+"},
				},
			},
			{
				ID:       "module-policy",
				Label:    "Policy Layer",
				Category: "Module",
				Summary:  "The rule engine for network permissions, dependency profiles, quotas, and tenant-level controls.",
				Capabilities: []string{
					"Network policy",
					"Quota control",
					"Dependency profile selection",
				},
				Actions: []string{
					"Gate risky capabilities",
					"Separate tenant policies",
					"Apply runtime envelopes",
				},
				Metrics: []Metric{
					{Label: "Default posture", Value: "deny by default"},
					{Label: "Primary goal", Value: "safety"},
					{Label: "Phase target", Value: "V2+"},
				},
			},
			{
				ID:       "module-observer",
				Label:    "Observer",
				Category: "Module",
				Summary:  "The audit and telemetry layer that captures lifecycle, exec, and file events across sandboxes.",
				Capabilities: []string{
					"Audit event stream",
					"Sandbox event filtering",
					"Operational timeline",
				},
				Actions: []string{
					"Record lifecycle transitions",
					"Track code and command execution",
					"Expose operator event queries",
				},
				Metrics: []Metric{
					{Label: "Data model", Value: "event stream"},
					{Label: "Primary goal", Value: "traceability"},
					{Label: "Phase target", Value: "V1+"},
				},
			},
		},
		Phases: []Panel{
			{
				ID:       "phase-1",
				Label:    "Phase 1",
				Category: "Delivery",
				Summary:  "Ship a real service that handles compatibility, live execution, and a concrete product surface.",
				Capabilities: []string{
					"Go HTTP server",
					"Live Python and Node.js runs",
					"Clickable control console",
				},
				Actions: []string{
					"Prove protocol compatibility",
					"Validate UI mapping",
					"Stabilize the minimum operator workflow",
				},
				Metrics: []Metric{
					{Label: "Target", Value: "execution replacement"},
					{Label: "UI state", Value: "operator-ready"},
					{Label: "API state", Value: "live"},
				},
			},
			{
				ID:       "phase-2",
				Label:    "Phase 2",
				Category: "Delivery",
				Summary:  "Introduce managed sessions and the first real lifecycle APIs for repeated execution flows.",
				Capabilities: []string{
					"Create, get, delete sandboxes",
					"Session binding",
					"Command and file APIs",
				},
				Actions: []string{
					"Add sandbox metadata storage",
					"Support artifact shipping",
					"Bind lifecycle to workflow runs",
				},
				Metrics: []Metric{
					{Label: "Target", Value: "workflow sessions"},
					{Label: "State store", Value: "optional DB/Redis"},
					{Label: "Risk focus", Value: "lifecycle drift"},
				},
			},
			{
				ID:       "phase-3",
				Label:    "Phase 3",
				Category: "Delivery",
				Summary:  "Expand into interactive runtimes with endpoint routing and stronger isolation options.",
				Capabilities: []string{
					"Interactive runtimes",
					"Endpoint exposure",
					"Renewable sessions",
				},
				Actions: []string{
					"Support coding and browser agents",
					"Attach endpoint routing",
					"Introduce secure runtime classes",
				},
				Metrics: []Metric{
					{Label: "Target", Value: "agent platform"},
					{Label: "Exposure", Value: "routed endpoints"},
					{Label: "Risk focus", Value: "runtime isolation"},
				},
			},
		},
	}
}
