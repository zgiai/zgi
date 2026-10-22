package digitalhumantask

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
)

// maxTaskRequesterPageSize bounds one page of the requester search.
const maxTaskRequesterPageSize = 50

// maxRequesterLabelRunes matches the end user instance label column.
const maxRequesterLabelRunes = 128

// requesterKindSQL classifies a task row by who asked for it: a workspace
// member (including tasks a manager created in the task center), an API end
// user, or a channel conversation. It is used to group, never to filter.
const requesterKindSQL = `CASE WHEN owner_kind = 'conversation' THEN 'channel'
	WHEN owner_subject_kind = 'api_end_user' THEN 'api_user' ELSE 'member' END`

// requesterIDSQL is who a task is grouped under: the requesting subject, or
// the manager who created a task in the task center. An account subject's ID
// is its account ID.
const requesterIDSQL = `CASE WHEN owner_kind = 'digital_human' THEN created_by ELSE owner_subject_id END`

// showsRequesters reports whether a reader sees who asked for each task. Only
// the console task center does; requesters see their own tasks elsewhere.
func showsRequesters(access digitalhuman.TaskAccessContext) bool {
	return access.Viewer.Mode == digitalhuman.TaskViewTaskCenter
}

// requesterKindCondition filters to one kind of requester with plain column
// predicates, so the owner indexes serve it.
func requesterKindCondition(kind digitalhuman.TaskRequesterKind) (string, bool) {
	switch kind {
	case digitalhuman.TaskRequesterMember:
		return "((owner_kind = 'user_subject' AND owner_subject_kind = 'account') OR owner_kind = 'digital_human')", true
	case digitalhuman.TaskRequesterAPIUser:
		return "owner_subject_kind = 'api_end_user'", true
	case digitalhuman.TaskRequesterChannel:
		return "owner_kind = 'conversation'", true
	default:
		return "", false
	}
}

func applyRequesterFilter(query *gorm.DB, filter digitalhuman.TaskListQuery) (*gorm.DB, error) {
	if len(filter.RequesterKinds) > 0 {
		conditions := make([]string, 0, len(filter.RequesterKinds))
		for _, kind := range filter.RequesterKinds {
			condition, ok := requesterKindCondition(kind)
			if !ok {
				return nil, digitalhuman.ErrInvalidRequest
			}
			conditions = append(conditions, condition)
		}
		query = query.Where("(" + strings.Join(conditions, " OR ") + ")")
	}
	if raw := strings.TrimSpace(filter.RequesterID); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			return nil, digitalhuman.ErrInvalidRequest
		}
		query = query.Where("(owner_subject_id = ? OR (owner_kind = 'digital_human' AND created_by = ?))", id.String(), id.String())
	}
	return query, nil
}

type requesterKey struct {
	Kind string
	ID   string
}

func taskRequesterKey(row *automationmodel.AutomationTask) requesterKey {
	switch {
	case row.OwnerKind != nil && *row.OwnerKind == automationmodel.AutomationTaskOwnerConversation:
		return requesterKey{Kind: string(digitalhuman.TaskRequesterChannel)}
	case row.OwnerKind != nil && *row.OwnerKind == automationmodel.AutomationTaskOwnerDigitalHuman:
		return requesterKey{Kind: string(digitalhuman.TaskRequesterMember), ID: row.CreatedBy}
	case pointerValue(row.OwnerSubjectKind) == "api_end_user":
		return requesterKey{Kind: string(digitalhuman.TaskRequesterAPIUser), ID: pointerValue(row.OwnerSubjectID)}
	default:
		return requesterKey{Kind: string(digitalhuman.TaskRequesterMember), ID: pointerValue(row.OwnerSubjectID)}
	}
}

// describeRequesters names the requesters of tasks: members by their account
// name and email, API end users by the label an operator gave their instance.
func (s *Service) describeRequesters(ctx context.Context, access digitalhuman.TaskAccessContext, keys []requesterKey) (map[requesterKey]*digitalhuman.TaskRequester, error) {
	result := make(map[requesterKey]*digitalhuman.TaskRequester, len(keys))
	var accountIDs, subjectIDs []string
	for _, key := range keys {
		if _, ok := result[key]; ok {
			continue
		}
		result[key] = &digitalhuman.TaskRequester{Kind: digitalhuman.TaskRequesterKind(key.Kind), ID: key.ID}
		if key.ID == "" {
			continue
		}
		if key.Kind == string(digitalhuman.TaskRequesterMember) {
			accountIDs = append(accountIDs, key.ID)
		} else if key.Kind == string(digitalhuman.TaskRequesterAPIUser) {
			subjectIDs = append(subjectIDs, key.ID)
		}
	}
	if len(accountIDs) > 0 {
		var accounts []struct {
			ID    string
			Name  string
			Email string
		}
		if err := s.db.WithContext(ctx).Table("accounts").Select("id, name, email").Where("id IN ?", accountIDs).Scan(&accounts).Error; err != nil {
			return nil, fmt.Errorf("load task requesters: %w", err)
		}
		for _, account := range accounts {
			if requester := result[requesterKey{Kind: string(digitalhuman.TaskRequesterMember), ID: account.ID}]; requester != nil {
				requester.Name, requester.Email = account.Name, account.Email
			}
		}
	}
	if len(subjectIDs) > 0 {
		var instances []struct {
			SubjectID string
			Label     *string
		}
		if err := s.db.WithContext(ctx).Table("digital_human_end_user_instances").Select("subject_id, label").
			Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND subject_id IN ?", access.OrganizationID, access.WorkspaceID, access.DigitalHumanID, subjectIDs).
			Scan(&instances).Error; err != nil {
			return nil, fmt.Errorf("load task requester instances: %w", err)
		}
		for _, instance := range instances {
			if requester := result[requesterKey{Kind: string(digitalhuman.TaskRequesterAPIUser), ID: instance.SubjectID}]; requester != nil {
				requester.Name = strings.TrimSpace(pointerValue(instance.Label))
			}
		}
	}
	return result, nil
}

// attachRequesters sets the requester of each task for readers who see them.
func (s *Service) attachRequesters(ctx context.Context, access digitalhuman.TaskAccessContext, rows []*automationmodel.AutomationTask, items []*digitalhuman.Task) error {
	if !showsRequesters(access) || len(rows) == 0 {
		return nil
	}
	keys := make([]requesterKey, len(rows))
	for index, row := range rows {
		keys[index] = taskRequesterKey(row)
	}
	requesters, err := s.describeRequesters(ctx, access, keys)
	if err != nil {
		return err
	}
	for index, item := range items {
		requester := *requesters[keys[index]]
		item.Requester = &requester
	}
	return nil
}

// ListTaskRequesters searches the people who asked for tasks the task center
// sees, most recently active first. Channel conversations have no person to
// pick, so they are filtered by kind only.
func (s *Service) ListTaskRequesters(ctx context.Context, access digitalhuman.TaskAccessContext, query digitalhuman.TaskRequesterQuery) (*digitalhuman.TaskRequesterPage, error) {
	if err := validateBoundary(access); err != nil {
		return nil, err
	}
	if !showsRequesters(access) {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	page, pageSize := query.Page, query.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > maxTaskRequesterPageSize {
		pageSize = 20
	}
	rows := s.taskQuery(ctx, access).Where("owner_kind <> 'conversation'")
	if query.Kind != "" {
		condition, ok := requesterKindCondition(query.Kind)
		if !ok || query.Kind == digitalhuman.TaskRequesterChannel {
			return nil, digitalhuman.ErrInvalidRequest
		}
		rows = rows.Where(condition)
	}
	if keyword := strings.ToLower(strings.TrimSpace(query.Query)); keyword != "" {
		like := "%" + keyword + "%"
		rows = rows.Where(`(LOWER(CAST(`+requesterIDSQL+` AS TEXT)) LIKE ? OR client_reference = ?
			OR EXISTS (SELECT 1 FROM accounts WHERE accounts.id = (`+requesterIDSQL+`) AND (LOWER(accounts.name) LIKE ? OR LOWER(accounts.email) LIKE ?))
			OR EXISTS (SELECT 1 FROM digital_human_end_user_instances AS instances WHERE instances.digital_human_id = automation_tasks.subject_id
				AND instances.subject_id = automation_tasks.owner_subject_id AND LOWER(instances.label) LIKE ?))`,
			keyword+"%", strings.TrimSpace(query.Query), like, like, like)
	}
	grouped := s.db.WithContext(ctx).Table("(?) AS requesters",
		rows.Select("("+requesterKindSQL+") AS kind, ("+requesterIDSQL+") AS requester_id, updated_at")).
		Select("kind, requester_id, COUNT(*) AS count").Group("kind, requester_id")
	var total int64
	if err := s.db.WithContext(ctx).Table("(?) AS grouped", grouped).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count digital human task requesters: %w", err)
	}
	var found []struct {
		Kind        string
		RequesterID string
		Count       int64
	}
	if err := grouped.Order("MAX(updated_at) DESC").Offset((page - 1) * pageSize).Limit(pageSize).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("list digital human task requesters: %w", err)
	}
	keys := make([]requesterKey, len(found))
	for index, row := range found {
		keys[index] = requesterKey{Kind: row.Kind, ID: row.RequesterID}
	}
	requesters, err := s.describeRequesters(ctx, access, keys)
	if err != nil {
		return nil, err
	}
	items := make([]digitalhuman.TaskRequesterFacet, 0, len(found))
	for index, row := range found {
		items = append(items, digitalhuman.TaskRequesterFacet{TaskRequester: *requesters[keys[index]], Count: row.Count})
	}
	return &digitalhuman.TaskRequesterPage{Items: items, Total: total, Page: page, PageSize: pageSize, HasMore: int64(page*pageSize) < total}, nil
}

// LabelTaskRequester names an API end user for the task center. The label is
// kept on their end user instance, which only exists once they have used the
// DigitalHuman; an empty label clears it.
func (s *Service) LabelTaskRequester(ctx context.Context, access digitalhuman.TaskAccessContext, subjectID uuid.UUID, label string) (*digitalhuman.TaskRequester, error) {
	if err := validateBoundary(access); err != nil {
		return nil, err
	}
	if !showsRequesters(access) || !access.Viewer.Manager {
		return nil, digitalhuman.ErrTaskPolicyRejected
	}
	label = strings.TrimSpace(label)
	if subjectID == uuid.Nil || utf8.RuneCountInString(label) > maxRequesterLabelRunes || strings.ContainsFunc(label, unicode.IsControl) {
		return nil, digitalhuman.ErrInvalidRequest
	}
	var value any
	if label != "" {
		value = label
	}
	result := s.db.WithContext(ctx).Table("digital_human_end_user_instances").
		Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND subject_id = ?", access.OrganizationID, access.WorkspaceID, access.DigitalHumanID, subjectID).
		Updates(map[string]any{"label": value, "updated_at": s.now()})
	if result.Error != nil {
		return nil, fmt.Errorf("label digital human task requester: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, digitalhuman.ErrTaskNotFound
	}
	return &digitalhuman.TaskRequester{Kind: digitalhuman.TaskRequesterAPIUser, ID: subjectID.String(), Name: label}, nil
}
