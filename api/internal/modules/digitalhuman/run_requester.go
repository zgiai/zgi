package digitalhuman

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskRequesterVisitor is a landing page visitor who has not signed in. Only
// runs have them; a task always belongs to someone.
const TaskRequesterVisitor TaskRequesterKind = "visitor"

// runRequesterDescriber names who asked for each run, for console readers.
type runRequesterDescriber interface {
	DescribeRunRequesters(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, runs []*Run) error
}

// DescribeRunRequesters names who asked for each run: members by account,
// API end users by the label an operator gave their instance, channels by
// provider, and visitors who have not signed in.
func (s *RuntimeService) DescribeRunRequesters(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, runs []*Run) error {
	describer, ok := s.store.(runRequesterDescriber)
	if !ok || len(runs) == 0 {
		return nil
	}
	return describer.DescribeRunRequesters(ctx, scope, digitalHumanID, runs)
}

// runRequesterKindCondition filters runs to one kind of requester. A task run
// without a requesting subject belongs to a task a manager created.
func runRequesterKindCondition(kind TaskRequesterKind) (string, bool) {
	switch kind {
	case TaskRequesterMember:
		return "(surface <> 'channel' AND (user_subject_kind = 'account' OR (surface = 'digital_human_task' AND user_subject_kind = '')))", true
	case TaskRequesterAPIUser:
		return "(surface <> 'channel' AND user_subject_kind = 'api_end_user')", true
	case TaskRequesterChannel:
		return "surface = 'channel'", true
	case TaskRequesterVisitor:
		return "(surface = 'digital_human_webapp' AND user_subject_kind = '')", true
	default:
		return "", false
	}
}

func applyRunRequesterFilter(db *gorm.DB, query ListQuery) (*gorm.DB, error) {
	if len(query.RequesterKinds) > 0 {
		conditions := make([]string, 0, len(query.RequesterKinds))
		for _, kind := range query.RequesterKinds {
			condition, ok := runRequesterKindCondition(kind)
			if !ok {
				return nil, ErrInvalidRequest
			}
			conditions = append(conditions, condition)
		}
		db = db.Where("(" + strings.Join(conditions, " OR ") + ")")
	}
	if raw := strings.TrimSpace(query.RequesterID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			return nil, ErrInvalidRequest
		}
		db = db.Where("user_subject_id = ?", id)
	}
	return db, nil
}

func (r *GormRepository) DescribeRunRequesters(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, runs []*Run) error {
	accounts := map[string][]*Run{}
	subjects := map[string][]*Run{}
	for _, run := range runs {
		if run == nil {
			continue
		}
		subject := ""
		if run.UserSubjectID != nil && *run.UserSubjectID != uuid.Nil {
			subject = run.UserSubjectID.String()
		}
		switch {
		case run.Surface == SurfaceChannel:
			run.Requester = &TaskRequester{Kind: TaskRequesterChannel, Name: runChannelProvider(run)}
		case run.UserSubjectKind == "account" && subject != "":
			run.Requester = &TaskRequester{Kind: TaskRequesterMember, ID: subject}
			accounts[subject] = append(accounts[subject], run)
		case run.UserSubjectKind == "api_end_user" && subject != "":
			run.Requester = &TaskRequester{Kind: TaskRequesterAPIUser, ID: subject}
			subjects[subject] = append(subjects[subject], run)
		case run.Surface == SurfaceWebApp:
			run.Requester = &TaskRequester{Kind: TaskRequesterVisitor}
		}
	}
	if len(accounts) > 0 {
		var rows []struct {
			ID    string
			Name  string
			Email string
		}
		if err := r.db.WithContext(ctx).Table("accounts").Select("id, name, email").Where("id IN ?", mapKeys(accounts)).Scan(&rows).Error; err != nil {
			return fmt.Errorf("load DigitalHuman run requesters: %w", err)
		}
		for _, row := range rows {
			for _, run := range accounts[row.ID] {
				run.Requester.Name, run.Requester.Email = row.Name, row.Email
			}
		}
	}
	if len(subjects) > 0 {
		var rows []struct {
			SubjectID string
			Label     *string
		}
		if err := r.db.WithContext(ctx).Table("digital_human_end_user_instances").Select("subject_id, label").
			Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND subject_id IN ?", scope.OrganizationID, scope.WorkspaceID, digitalHumanID, mapKeys(subjects)).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("load DigitalHuman run requester instances: %w", err)
		}
		for _, row := range rows {
			if row.Label == nil {
				continue
			}
			for _, run := range subjects[row.SubjectID] {
				run.Requester.Name = strings.TrimSpace(*row.Label)
			}
		}
	}
	return nil
}

// runChannelProvider is the connected channel a run came from, such as feishu.
func runChannelProvider(run *Run) string {
	var event struct {
		Origin struct {
			Provider string `json:"provider"`
		} `json:"origin"`
	}
	if len(run.InvocationEvent) == 0 || json.Unmarshal(run.InvocationEvent, &event) != nil {
		return ""
	}
	return strings.TrimSpace(event.Origin.Provider)
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
