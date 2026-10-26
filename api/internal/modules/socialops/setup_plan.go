package socialops

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

type SOPStage string

const (
	StageAccountCheck     SOPStage = "account_check"
	StageProductKnowledge SOPStage = "product_knowledge"
	StageResearch         SOPStage = "research"
	StageSelection        SOPStage = "selection"
	StageProduction       SOPStage = "production"
	StageReview           SOPStage = "review"
	StagePublication      SOPStage = "publication"
	StageInteraction      SOPStage = "interaction"
	StageArchive          SOPStage = "archive"
	StageReviewLoop       SOPStage = "review_loop"
)

func AllSOPStages() []SOPStage {
	return []SOPStage{
		StageAccountCheck, StageProductKnowledge, StageResearch, StageSelection,
		StageProduction, StageReview, StagePublication, StageInteraction,
		StageArchive, StageReviewLoop,
	}
}

type RunBudget struct {
	MaxPages            int `json:"max_pages"`
	MaxRecords          int `json:"max_records"`
	MaxConnectorCalls   int `json:"max_connector_calls"`
	MaxTokens           int `json:"max_tokens"`
	MaxSafeReadAttempts int `json:"max_safe_read_attempts"`
}

// RecordTarget identifies an existing socialops record kind and stable key.
type RecordTarget struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type PlannedTask struct {
	Key                 string                         `json:"key"`
	Stages              []SOPStage                     `json:"stages"`
	Budget              RunBudget                      `json:"budget"`
	ConsumesRecordKinds []string                       `json:"consumes_record_kinds"`
	ProducesRecordKinds []string                       `json:"produces_record_kinds"`
	UpdatesRecords      []RecordTarget                 `json:"updates_records"`
	Request             digitalhuman.CreateTaskRequest `json:"request"`
}

type SetupPlan struct {
	Scope                 Scope         `json:"scope"`
	EmployeeVersionID     uuid.UUID     `json:"employee_version_id"`
	ConfigurationRevision int64         `json:"configuration_revision"`
	Tasks                 []PlannedTask `json:"tasks"`
}

// BuildSetupPlan produces DigitalHuman task requests compatible with the
// existing Automation scheduler. It creates no task and performs no IO.
func BuildSetupPlan(configuration Configuration) (SetupPlan, error) {
	if err := configuration.Validate(); err != nil {
		return SetupPlan{}, err
	}
	refs := fmt.Sprintf(
		"owner %s, employee %s, published employee version %s, configuration revision %d",
		configuration.Scope.OwnerID, configuration.Scope.EmployeeID,
		configuration.EmployeeVersionID, configuration.Revision,
	)
	context := fmt.Sprintf(
		"Operate %s (%s) for audience %s. Publish in %s and review internally in %s. Load the persisted, scoped configuration and journal before any connector call; this run is bound to %s.",
		configuration.Product.Name, configuration.Product.Description, configuration.Audience, configuration.PublicationLanguage,
		configuration.InternalReviewLanguage, refs,
	)
	criterion := []string{
		"Use only persisted records and server-restored connections and grants for " + refs + ".",
		"Persist each successful checkpoint before advancing; uncertain external writes remain blocked and are never retried blindly.",
	}

	tasks := []PlannedTask{
		plannedTask(
			"daily_content", "Social daily content", configuration.Cadence.DailyContentCron,
			configuration.Timezone, 900,
			[]SOPStage{StageAccountCheck, StageProductKnowledge, StageResearch, StageSelection, StageProduction, StageReview, StagePublication, StageArchive},
			RunBudget{MaxPages: 12, MaxRecords: 100, MaxConnectorCalls: 80, MaxTokens: 60000, MaxSafeReadAttempts: 3},
			[]string{"configuration", "source", "product_fact", "topic", "writing_rule", "checkpoint", "review", "archive"},
			[]string{"draft", "review", "checkpoint", "archive"}, nil,
			context+" Research current evidence, select a non-duplicate topic, create and review the complete language/media bundle, reserve within each platform's daily limit, publish once, read back the actual remote object, and archive only verified actual content.",
			appendCriteria(criterion,
				"Every selected claim has a persisted source or product-fact record and review is bound to exact artifact hashes.",
				"Each publication has a durable reservation, provider receipt, authoritative readback, and actual archive; one platform failure does not erase another platform's result."),
		),
		plannedTask(
			"frequent_inbox", "Social inbox", configuration.Cadence.InboxCron,
			configuration.Timezone, 300,
			[]SOPStage{StageAccountCheck, StageProductKnowledge, StageInteraction, StageArchive},
			RunBudget{MaxPages: 10, MaxRecords: 100, MaxConnectorCalls: 60, MaxTokens: 20000, MaxSafeReadAttempts: 3},
			[]string{"configuration", "product_fact", "checkpoint", "feedback", "archive"},
			[]string{"feedback", "checkpoint", "archive"}, nil,
			context+" Resume each persisted inbox watermark, handle a bounded page of mentions, comments, and messages, ground replies in current facts, and route issues or leads through the configured team handoff destination with a stable handoff key.",
			appendCriteria(criterion,
				"Advance each inbox watermark only after its source item, reply or escalation outcome, and checkpoint are durable.",
				"Unsupported optional inbox capabilities are recorded as skipped; required unavailable capabilities leave the run actionable rather than successful."),
		),
		plannedTask(
			"daily_seven_day_samples", "Social seven-day samples", configuration.Cadence.SevenDaySampleCron,
			configuration.Timezone, 300,
			[]SOPStage{StageAccountCheck, StageReviewLoop},
			RunBudget{MaxPages: 10, MaxRecords: 100, MaxConnectorCalls: 50, MaxTokens: 10000, MaxSafeReadAttempts: 3},
			[]string{"configuration", "checkpoint", "archive"},
			[]string{"checkpoint"}, nil,
			context+" Load verified publications due at or after published_at plus seven days, capture each sample once, preserve missing metrics as missing, and persist actual observation age and lateness.",
			appendCriteria(criterion,
				"Only verified publications without an existing sample are considered, with a bounded candidate page and durable continuation checkpoint.",
				"Metric records preserve platform definitions and distinguish late observations from exact seven-day observations."),
		),
		plannedTask(
			"weekly_review", "Social weekly review", configuration.Cadence.WeeklyReviewCron,
			configuration.Timezone, 600,
			[]SOPStage{StageProductKnowledge, StageSelection, StageReview, StageReviewLoop},
			RunBudget{MaxPages: 5, MaxRecords: 200, MaxConnectorCalls: 20, MaxTokens: 30000, MaxSafeReadAttempts: 3},
			[]string{"configuration", "weekly_review", "topic", "writing_rule", "feedback", "archive"},
			[]string{"weekly_review"}, []RecordTarget{{Kind: "topic", Key: "next_topic"}, {Kind: "writing_rule", Key: "current"}},
			context+" Review persisted publications, feedback, and metric samples. Separate engagement, clicks, inquiries, and independently evidenced conversion, then persist the next_topic selection and update the writing_rule for the next runs.",
			appendCriteria(criterion,
				"The weekly_review cites the persisted evidence behind every conclusion and treats unavailable metrics as missing rather than zero.",
				"CAS updates persist topic key next_topic and writing_rule key current so later runs consume the explicit decisions."),
		),
	}
	return SetupPlan{
		Scope: configuration.Scope, EmployeeVersionID: configuration.EmployeeVersionID,
		ConfigurationRevision: configuration.Revision, Tasks: tasks,
	}, nil
}

func plannedTask(key, name, expression, timezone string, timeout int, stages []SOPStage, budget RunBudget, consumes, produces []string, updates []RecordTarget, goal string, criteria []string) PlannedTask {
	return PlannedTask{
		Key: key, Stages: append([]SOPStage(nil), stages...), Budget: budget,
		ConsumesRecordKinds: append([]string(nil), consumes...), ProducesRecordKinds: append([]string(nil), produces...), UpdatesRecords: append([]RecordTarget(nil), updates...),
		Request: digitalhuman.CreateTaskRequest{
			Name: name, Goal: strings.TrimSpace(goal), SuccessCriteria: criteria,
			Priority:                digitalhuman.TaskPriorityNormal,
			Schedule:                digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleCron, Cron: expression, Timezone: timezone},
			DeliveryMode:            digitalhuman.TaskDeliveryTaskOnly,
			ExecutionTimeoutSeconds: timeout, MaxAttempts: 1,
		},
	}
}

func appendCriteria(base []string, extra ...string) []string {
	result := make([]string, 0, len(base)+len(extra))
	result = append(result, base...)
	return append(result, extra...)
}
