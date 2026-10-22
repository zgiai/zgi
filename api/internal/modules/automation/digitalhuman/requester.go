package digitalhumantask

import (
	"context"
	"fmt"
	"strings"

	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
)

// maxTaskRequesterFacets bounds the requester choices of one task center.
const maxTaskRequesterFacets = 200

// requesterKindSQL classifies a task row by who asked for it: a workspace
// member (including tasks a manager created in the task center), an API end
// user, or a channel conversation.
const requesterKindSQL = `CASE WHEN owner_kind = 'conversation' THEN 'channel'
	WHEN owner_subject_kind = 'api_end_user' THEN 'api_user' ELSE 'member' END`

// requesterIDSQL is the requester a task is filtered by: the requesting
// subject, or the manager who created a task in the task center. An account
// subject's ID is its account ID.
const requesterIDSQL = `CASE WHEN owner_kind = 'digital_human' THEN created_by ELSE owner_subject_id END`

// showsRequesters reports whether a reader sees who asked for each task. Only
// the console task center does; requesters see their own tasks elsewhere.
func showsRequesters(access digitalhuman.TaskAccessContext) bool {
	return access.Viewer.Mode == digitalhuman.TaskViewTaskCenter
}

func applyRequesterFilter(query *gorm.DB, filter digitalhuman.TaskListQuery) (*gorm.DB, error) {
	if len(filter.RequesterKinds) > 0 {
		kinds := make([]string, 0, len(filter.RequesterKinds))
		for _, kind := range filter.RequesterKinds {
			switch kind {
			case digitalhuman.TaskRequesterMember, digitalhuman.TaskRequesterAPIUser, digitalhuman.TaskRequesterChannel:
				kinds = append(kinds, string(kind))
			default:
				return nil, digitalhuman.ErrInvalidRequest
			}
		}
		query = query.Where("("+requesterKindSQL+") IN ?", kinds)
	}
	if id := strings.TrimSpace(filter.RequesterID); id != "" {
		query = query.Where("("+requesterIDSQL+") = ?", id)
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

// requesterFacets lists everyone who asked for a task the reader can see,
// with how many tasks each asked for, so the task center can filter by them.
func (s *Service) requesterFacets(ctx context.Context, access digitalhuman.TaskAccessContext) ([]digitalhuman.TaskRequesterFacet, error) {
	var rows []struct {
		Kind  string
		ID    *string
		Count int64
	}
	if err := s.taskQuery(ctx, access).Select("(" + requesterKindSQL + ") AS kind, (" + requesterIDSQL + ") AS id, COUNT(*) AS count").
		Group("1, 2").Order("count DESC").Limit(maxTaskRequesterFacets).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count digital human task requesters: %w", err)
	}
	keys := make([]requesterKey, 0, len(rows))
	for _, row := range rows {
		key := requesterKey{Kind: row.Kind}
		if row.Kind != string(digitalhuman.TaskRequesterChannel) {
			key.ID = pointerValue(row.ID)
		}
		keys = append(keys, key)
	}
	requesters, err := s.describeRequesters(ctx, access, keys)
	if err != nil {
		return nil, err
	}
	// Channel tasks are one choice however many conversations asked.
	merged := make(map[requesterKey]int, len(keys))
	facets := make([]digitalhuman.TaskRequesterFacet, 0, len(keys))
	for index, key := range keys {
		if at, ok := merged[key]; ok {
			facets[at].Count += rows[index].Count
			continue
		}
		merged[key] = len(facets)
		facets = append(facets, digitalhuman.TaskRequesterFacet{TaskRequester: *requesters[key], Count: rows[index].Count})
	}
	return facets, nil
}
