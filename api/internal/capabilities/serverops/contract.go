package serverops

// This file declares the operation and input boundary an adapter may expose for
// the server operations capability: three read-only tools and four controlled
// service write tools, each mapped one to one onto an existing Action* constant
// and an existing Service result type.
//
// It declares that boundary and nothing more. It cannot advertise or deliver the
// capability on its own: an adapter still has to own the credential store, obtain
// explicit user approval for every single use, build the request-scoped
// Connection, invoke the matching Service method, and map the typed results and
// errors onto its own contract.
//
// The manifest is inert by construction. It holds no connection, target,
// credential, or route; it registers nothing; it performs no approval; and it
// declares no package-level state, so reading it can never start a runtime,
// mutate a registry, or reach the network.
//
// ApprovalPolicy states an obligation on the adapter, not behaviour this package
// performs: every single use of a stored credential, read or write, must be
// explicitly approved by the user first, after the user has seen the exact
// command it runs, and one approval never authorizes a second use.
// The capability itself only consumes a Connection the adapter already approved
// and built.

// SkillID is the stable identity of the server operations Skill.
const SkillID = "server-operations"

// SkillSummary is the one-line description of what the Skill offers. It is
// conditional: every action happens only after explicit per-use user approval,
// and the Skill cannot deliver it on its own. It describes the inspection
// surface and the four fixed service actions only and claims no deployment,
// configuration change, or arbitrary command capability.
const SkillSummary = "After per-use user approval, inspection of a single Linux server over SSH " +
	"(a system snapshot, systemd service status, and a bounded service journal tail) " +
	"restart, start, stop, or reload of one systemd service, and running a shell command as the SSH user."

// ToolName is a stable tool name inside SkillID.
type ToolName string

// The Skill's complete tool set: three read-only tools, four service write
// tools, and the command run tool, and no other.
const (
	// ToolSystemSnapshot reads host facts and maps to ActionSystemSnapshot.
	ToolSystemSnapshot ToolName = "system_snapshot"
	// ToolServiceStatus reads one systemd unit's state and maps to
	// ActionServiceStatus.
	ToolServiceStatus ToolName = "service_status"
	// ToolJournalTail reads a bounded journal window and maps to
	// ActionJournalTail.
	ToolJournalTail ToolName = "journal_tail"
	// ToolServiceRestart restarts one systemd unit and maps to
	// ActionServiceRestart.
	ToolServiceRestart ToolName = "service_restart"
	// ToolServiceStart starts one systemd unit and maps to ActionServiceStart.
	ToolServiceStart ToolName = "service_start"
	// ToolServiceStop stops one systemd unit and maps to ActionServiceStop.
	ToolServiceStop ToolName = "service_stop"
	// ToolServiceReload reloads one systemd unit and maps to
	// ActionServiceReload.
	ToolServiceReload ToolName = "service_reload"
	// ToolCommandRun runs one caller-written shell command and maps to
	// ActionCommandRun.
	ToolCommandRun ToolName = "command_run"
)

// ApprovalPolicy is the approval an adapter must obtain before it may use a
// stored credential for one tool call.
//
// It is a declaration only. This package holds no credential, performs no
// approval, and exposes no runtime that could record, reuse, or bypass one.
type ApprovalPolicy string

// ApprovalPerUseUserApproval requires explicit user approval for every single
// use of a credential. There is no session, standing, or blanket approval, and
// an approval for one call never authorizes another. The approver must see the
// exact command the call runs. An adapter must enforce this before it builds
// the request-scoped Connection for a call, and a call without an interactive
// approver (a scheduled or background run) must be refused.
const ApprovalPerUseUserApproval ApprovalPolicy = "per_use_user_approval"

// ParameterKind is the closed set of parameter types a tool may declare.
type ParameterKind string

const (
	// ParameterKindString is a UTF-8 string parameter validated by
	// ParameterDescriptor.Pattern exactly as supplied, without trimming.
	ParameterKindString ParameterKind = "string"
	// ParameterKindInteger is an integer parameter bounded inclusively by
	// ParameterDescriptor.Minimum and Maximum.
	ParameterKindInteger ParameterKind = "integer"
)

// ParameterDescriptor describes exactly one accepted tool parameter. Its fields
// mirror the runtime validation in read_actions.go, so an adapter can never
// advertise a looser shape than the capability accepts.
type ParameterDescriptor struct {
	// Name is the exact input key. It is never a credential, a target, or a
	// command.
	Name string
	// Kind is the closed parameter type.
	Kind ParameterKind
	// Required reports whether the caller must supply the parameter.
	Required bool
	// Pattern is the exact accepted grammar for a string parameter; it is empty
	// for an integer parameter.
	Pattern string
	// Minimum and Maximum bound an integer parameter inclusively. They are zero
	// for a string parameter.
	Minimum int64
	Maximum int64
	// Default is the effective value when an optional integer parameter is
	// omitted, and HasDefault distinguishes it from a legitimate zero.
	Default    int64
	HasDefault bool
	// Description explains the parameter without carrying a target or secret.
	Description string
}

// ToolDescriptor is the static description of one tool.
type ToolDescriptor struct {
	// Name is the stable tool name inside SkillID.
	Name ToolName
	// ActionID is the existing capability action constant the tool maps to.
	ActionID string
	// Summary is the one-line human description.
	Summary string
	// ReadOnly is true for the three inspection tools and false for the four
	// service write tools, which change the state of one systemd unit.
	ReadOnly bool
	// RequiredApproval is the approval an adapter must obtain per use.
	RequiredApproval ApprovalPolicy
	// Parameters is the exact accepted parameter set, in declaration order.
	Parameters []ParameterDescriptor
	// Returns names the existing Service result type this tool yields. It is a
	// result type name, never a route or an endpoint.
	Returns string
}

// SkillDescriptor is the whole static contract: one stable id and its complete,
// ordered tool set.
type SkillDescriptor struct {
	// ID is the stable Skill identity.
	ID string
	// Summary is the one-line Skill description.
	Summary string
	// Tools is the complete ordered tool set.
	Tools []ToolDescriptor
}

// Skill returns the static Skill manifest. Every call builds an independent
// value, so no caller can observe or mutate another caller's copy.
func Skill() SkillDescriptor {
	return SkillDescriptor{
		ID:      SkillID,
		Summary: SkillSummary,
		Tools: []ToolDescriptor{
			{
				Name:     ToolSystemSnapshot,
				ActionID: ActionSystemSnapshot,
				Summary: "Read host facts: hostname, operating system, kernel, uptime, " +
					"load average, memory, and mounted-disk usage.",
				ReadOnly:         true,
				RequiredApproval: ApprovalPerUseUserApproval,
				Returns:          "SystemSnapshot",
			},
			{
				Name:     ToolServiceStatus,
				ActionID: ActionServiceStatus,
				Summary: "Read one systemd unit's load, active, and sub state together " +
					"with its description.",
				ReadOnly:         true,
				RequiredApproval: ApprovalPerUseUserApproval,
				Parameters: []ParameterDescriptor{
					{
						Name:        "service_name",
						Kind:        ParameterKindString,
						Required:    true,
						Pattern:     SystemdServiceNamePattern,
						Description: "Exact systemd unit name, for example nginx.service.",
					},
				},
				Returns: "ServiceStatus",
			},
			{
				Name:             ToolJournalTail,
				ActionID:         ActionJournalTail,
				Summary:          "Read a bounded tail of one systemd unit's journal.",
				ReadOnly:         true,
				RequiredApproval: ApprovalPerUseUserApproval,
				Parameters: []ParameterDescriptor{
					{
						Name:        "service_name",
						Kind:        ParameterKindString,
						Required:    true,
						Pattern:     SystemdServiceNamePattern,
						Description: "Exact systemd unit name, for example nginx.service.",
					},
					{
						Name:        "lines",
						Kind:        ParameterKindInteger,
						Minimum:     1,
						Maximum:     maxJournalLines,
						Default:     defaultJournalLines,
						HasDefault:  true,
						Description: "Maximum number of journal lines to return.",
					},
					{
						Name:        "since_minutes",
						Kind:        ParameterKindInteger,
						Minimum:     1,
						Maximum:     maxJournalSinceMinutes,
						Default:     defaultJournalSinceMinutes,
						HasDefault:  true,
						Description: "Only read journal entries from the last N minutes.",
					},
				},
				Returns: "JournalTail",
			},
			serviceWriteTool(ToolServiceRestart, ActionServiceRestart,
				"Restart one systemd unit, then read its load, active, and sub state."),
			serviceWriteTool(ToolServiceStart, ActionServiceStart,
				"Start one systemd unit, then read its load, active, and sub state."),
			serviceWriteTool(ToolServiceStop, ActionServiceStop,
				"Stop one systemd unit, then read its load, active, and sub state."),
			serviceWriteTool(ToolServiceReload, ActionServiceReload,
				"Reload one systemd unit's configuration, then read its load, active, and sub state."),
			{
				Name:     ToolCommandRun,
				ActionID: ActionCommandRun,
				Summary: "Run one shell command as the SSH user and return its exit code and " +
					"bounded output. The command can read or change anything that user may.",
				ReadOnly:         false,
				RequiredApproval: ApprovalPerUseUserApproval,
				Parameters: []ParameterDescriptor{
					{
						Name:        "command",
						Kind:        ParameterKindString,
						Required:    true,
						Pattern:     CommandPattern,
						Description: "The exact shell command to run, for example df -h.",
					},
				},
				Returns: "CommandRun",
			},
		},
	}
}

// serviceWriteTool builds the descriptor of one service write tool. Every write
// tool accepts exactly the one service_name parameter of the read tools, never
// a free-form argument, and returns the state read after the change.
func serviceWriteTool(name ToolName, actionID, summary string) ToolDescriptor {
	return ToolDescriptor{
		Name:             name,
		ActionID:         actionID,
		Summary:          summary,
		ReadOnly:         false,
		RequiredApproval: ApprovalPerUseUserApproval,
		Parameters: []ParameterDescriptor{
			{
				Name:        "service_name",
				Kind:        ParameterKindString,
				Required:    true,
				Pattern:     SystemdServiceNamePattern,
				Description: "Exact systemd unit name, for example nginx.service.",
			},
		},
		Returns: "ServiceChange",
	}
}

// SkillTool returns the descriptor for one stable tool name and reports whether
// the name is part of this Skill. The returned value is independent of the
// manifest and of every other caller.
func SkillTool(name ToolName) (ToolDescriptor, bool) {
	for _, tool := range Skill().Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return ToolDescriptor{}, false
}
