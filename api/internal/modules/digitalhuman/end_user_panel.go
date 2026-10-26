package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/gorm"
)

// endUserPanelLimit bounds the memories and tasks one panel lists; the
// counts cover all of them.
const endUserPanelLimit = 50

// EndUserPanel is what each DigitalHuman keeps for one end user (ADR 0006):
// their instance, its memory, their tasks, and their own connections.
type EndUserPanel struct {
	AppKey         string                 `json:"app_key"`
	DigitalHumanID uuid.UUID              `json:"digital_human_id"`
	Name           string                 `json:"name"`
	Instance       *EndUserPanelInstance  `json:"instance,omitempty"`
	MemoryCount    int64                  `json:"memory_count"`
	Memory         []EndUserPanelMemory   `json:"memory"`
	TaskCount      int64                  `json:"task_count"`
	Tasks          []EndUserPanelTask     `json:"tasks"`
	Connections    []PerUserConnectorSlot `json:"connections"`
}

// EndUserPanelInstance is the end user's instance of a DigitalHuman.
type EndUserPanelInstance struct {
	ID           uuid.UUID `json:"id"`
	Status       string    `json:"status"`
	FirstSeenAt  time.Time `json:"first_seen_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// EndUserPanelMemory is one active instance memory entry.
type EndUserPanelMemory struct {
	ID         uuid.UUID `json:"id"`
	MemoryKey  string    `json:"memory_key"`
	MemoryType string    `json:"memory_type"`
	Content    string    `json:"content"`
	Revision   int64     `json:"revision"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// EndUserPanelTask is one task the end user handed to the DigitalHuman.
type EndUserPanelTask struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	WorkStatus   *string    `json:"work_status,omitempty"`
	ScheduleType string     `json:"schedule_type"`
	CreatedAt    time.Time  `json:"created_at"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
}

// EndUserPanels lists, for each DigitalHuman an external access serves, what
// it keeps for one end user. DigitalHumans the end user never used are left
// out.
func (*EndUserData) EndUserPanels(ctx context.Context, db *gorm.DB, organizationID, subjectID uuid.UUID, apps []EndUserApp) ([]EndUserPanel, error) {
	panels := []EndUserPanel{}
	connections := map[string][]PerUserConnectorSlot{}
	if manager := RegisteredEndUserConnections(); manager != nil && PerUserConnectorsEnabled() {
		slots, err := manager.List(ctx, organizationID, subjectID, apps)
		if err != nil {
			return nil, err
		}
		for _, entry := range slots {
			connections[entry.AppKey] = entry.Slots
		}
	}
	for _, app := range apps {
		panel := EndUserPanel{AppKey: app.AppKey, DigitalHumanID: app.DigitalHumanID, Memory: []EndUserPanelMemory{}, Tasks: []EndUserPanelTask{},
			Connections: connections[app.AppKey]}
		if panel.Connections == nil {
			panel.Connections = []PerUserConnectorSlot{}
		}
		var name []string
		if err := db.WithContext(ctx).Table("digital_humans").Where("id = ? AND organization_id = ?", app.DigitalHumanID, organizationID).
			Pluck("name", &name).Error; err != nil {
			return nil, fmt.Errorf("load DigitalHuman name: %w", err)
		}
		if len(name) == 1 {
			panel.Name = name[0]
		}
		var instances []EndUserInstance
		if err := db.WithContext(ctx).Where("digital_human_id = ? AND subject_id = ?", app.DigitalHumanID, subjectID).Limit(1).Find(&instances).Error; err != nil {
			return nil, fmt.Errorf("load end-user instance: %w", err)
		}
		if len(instances) == 1 {
			instance := instances[0]
			panel.Instance = &EndUserPanelInstance{ID: instance.ID, Status: string(instance.Status), FirstSeenAt: instance.FirstSeenAt, LastActiveAt: instance.LastActiveAt}
			// The DigitalHuman's notes on the person live in their relationship
			// memory (ADR 0015).
			memory := relationshipmemory.NewService(db)
			relationship, err := memory.FindRelationship(ctx, instance.WorkspaceID, subjectID, relationshipmemory.ConsumerDigitalHuman, app.DigitalHumanID)
			if err != nil && !errors.Is(err, relationshipmemory.ErrRelationshipNotFound) {
				return nil, fmt.Errorf("load end-user relationship: %w", err)
			}
			if relationship != nil {
				notes, err := memory.Notes(ctx, relationship.ID)
				if err != nil {
					return nil, err
				}
				panel.MemoryCount = int64(len(notes))
				for index, note := range notes {
					if index == endUserPanelLimit {
						break
					}
					panel.Memory = append(panel.Memory, EndUserPanelMemory{ID: note.ID, MemoryKey: relationshipmemory.NoteName(note.MemoryKey),
						MemoryType: note.Category, Content: note.ContentPayload, Revision: note.Revision, UpdatedAt: note.UpdatedAt})
				}
			}
		}
		tasks := db.WithContext(ctx).Table("automation_tasks").
			Where("owner_subject_id = ? AND subject_type = 'digital_human' AND subject_id = ?", subjectID, app.DigitalHumanID.String())
		if err := tasks.Count(&panel.TaskCount).Error; err != nil {
			return nil, fmt.Errorf("count end-user tasks: %w", err)
		}
		if err := tasks.Select("id, name, status, work_status, schedule_type, created_at, last_run_at").Order("created_at DESC").Limit(endUserPanelLimit).
			Scan(&panel.Tasks).Error; err != nil {
			return nil, fmt.Errorf("list end-user tasks: %w", err)
		}
		connected := false
		for _, slot := range panel.Connections {
			connected = connected || slot.ConnectionID != nil
		}
		if panel.Instance == nil && panel.TaskCount == 0 && !connected {
			continue
		}
		panels = append(panels, panel)
	}
	return panels, nil
}
