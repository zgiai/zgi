package digitalhuman

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"gorm.io/datatypes"
)

// SetupRequirements are what a DigitalHuman started from a built-in template
// needs before it can be published (ADR 0023): the template's required skills
// and connector connections. They are kept with the DigitalHuman, so a later
// change to the template does not move it.
type SetupRequirements struct {
	TemplateID string              `json:"template_id,omitempty"`
	Skills     []string            `json:"skills,omitempty"`
	Connectors []TemplateConnector `json:"connectors,omitempty"`
}

// Setup item kinds.
const (
	SetupItemSkill     = "skill"
	SetupItemConnector = "connector"
)

// SetupItem is one requirement and whether the draft meets it.
type SetupItem struct {
	Kind                 string   `json:"kind"`
	ID                   string   `json:"id"`
	OperationIDs         []string `json:"operation_ids,omitempty"`
	OptionalOperationIDs []string `json:"optional_operation_ids,omitempty"`
	Met                  bool     `json:"met"`
}

// SetupStatus is the setup checklist of a DigitalHuman's draft.
type SetupStatus struct {
	TemplateID string      `json:"template_id,omitempty"`
	Complete   bool        `json:"complete"`
	Items      []SetupItem `json:"items"`
}

// setupRequirementsFromTemplate returns the required items of a built-in
// template. Saved workspace templates only suggest skills, so they require
// nothing; an unknown ID requires nothing either.
func setupRequirementsFromTemplate(templateID string) SetupRequirements {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return SetupRequirements{}
	}
	for _, template := range BuiltinTemplates("en").Templates {
		if template.ID != templateID {
			continue
		}
		requirements := SetupRequirements{TemplateID: template.ID}
		for _, skill := range template.Skills {
			if skill.Required {
				requirements.Skills = append(requirements.Skills, skill.ID)
			}
		}
		for _, connector := range template.Connectors {
			if connector.Required {
				requirements.Connectors = append(requirements.Connectors, connector)
			}
		}
		return requirements
	}
	return SetupRequirements{}
}

func (r SetupRequirements) empty() bool {
	return len(r.Skills) == 0 && len(r.Connectors) == 0
}

func encodeSetupRequirements(requirements SetupRequirements) (datatypes.JSON, error) {
	if requirements.empty() {
		return datatypes.JSON([]byte(`{}`)), nil
	}
	encoded, err := json.Marshal(requirements)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(encoded), nil
}

func decodeSetupRequirements(raw datatypes.JSON) (SetupRequirements, error) {
	var requirements SetupRequirements
	if len(raw) == 0 {
		return requirements, nil
	}
	if err := json.Unmarshal(raw, &requirements); err != nil {
		return SetupRequirements{}, ErrInvalidRequest
	}
	return requirements, nil
}

// evaluateSetup checks requirements against a draft's bindings. A skill is
// met when it is bound; a connector need is met when a connection of that
// connector is bound and allows every required operation.
func evaluateSetup(requirements SetupRequirements, bindings []*ResourceBinding) SetupStatus {
	status := SetupStatus{TemplateID: requirements.TemplateID, Complete: true, Items: []SetupItem{}}
	boundSkills := map[string]bool{}
	connectorOperations := map[string][]map[string]bool{}
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		switch binding.BindingType {
		case resourcebindings.BindingTypeSkill:
			boundSkills[binding.ResourceID] = true
		case resourcebindings.BindingTypeConnector:
			var metadata struct {
				Operations []string `json:"allowed_operation_ids"`
			}
			_ = json.Unmarshal(binding.Metadata, &metadata)
			allowed := make(map[string]bool, len(metadata.Operations))
			for _, operation := range metadata.Operations {
				allowed[operation] = true
			}
			connectorOperations[binding.ParentResourceID] = append(connectorOperations[binding.ParentResourceID], allowed)
		}
	}
	for _, skill := range requirements.Skills {
		item := SetupItem{Kind: SetupItemSkill, ID: skill, Met: boundSkills[skill]}
		status.Complete = status.Complete && item.Met
		status.Items = append(status.Items, item)
	}
	for _, connector := range requirements.Connectors {
		item := SetupItem{
			Kind: SetupItemConnector, ID: connector.ConnectorID,
			OperationIDs:         append([]string{}, connector.RequiredOperationIDs...),
			OptionalOperationIDs: append([]string{}, connector.OptionalOperationIDs...),
		}
		for _, allowed := range connectorOperations[connector.ConnectorID] {
			covers := true
			for _, operation := range connector.RequiredOperationIDs {
				if !allowed[operation] {
					covers = false
					break
				}
			}
			if covers {
				item.Met = true
				break
			}
		}
		status.Complete = status.Complete && item.Met
		status.Items = append(status.Items, item)
	}
	return status
}

// setupStatusOf evaluates a DigitalHuman's requirements against its draft.
func setupStatusOf(ctx context.Context, repo *GormRepository, scope Scope, definition *Definition, draftID uuid.UUID) (SetupStatus, error) {
	requirements, err := decodeSetupRequirements(definition.SetupRequirements)
	if err != nil {
		return SetupStatus{}, err
	}
	if requirements.empty() {
		return SetupStatus{TemplateID: requirements.TemplateID, Complete: true, Items: []SetupItem{}}, nil
	}
	bindings, err := repo.ListResourceBindingRows(ctx, scope, definition.ID, draftID)
	if err != nil {
		return SetupStatus{}, err
	}
	return evaluateSetup(requirements, bindings), nil
}

// SetupStatus returns the setup checklist of a DigitalHuman's draft.
func (s *Service) SetupStatus(ctx context.Context, scope Scope, id uuid.UUID) (*SetupStatus, error) {
	if !validScope(scope) || id == uuid.Nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.setup")
	}
	definition, err := s.repo.Definition(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.setup")
	}
	draft, err := s.repo.Draft(ctx, scope, definition, false)
	if err != nil {
		return nil, applicationError(err, "digital_human.setup")
	}
	status, err := setupStatusOf(ctx, s.repo, scope, definition, draft.ID)
	if err != nil {
		return nil, applicationError(err, "digital_human.setup")
	}
	return &status, nil
}
