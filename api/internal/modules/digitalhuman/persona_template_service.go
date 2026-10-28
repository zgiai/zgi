package digitalhuman

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

func (s *Service) CreatePersonaTemplate(ctx context.Context, scope Scope, request CreatePersonaTemplateRequest) (*PersonaTemplate, error) {
	request.Name = strings.TrimSpace(request.Name)
	identityName := request.Name
	if request.IdentityName != nil {
		identityName = strings.TrimSpace(*request.IdentityName)
	}
	request.Description = strings.TrimSpace(request.Description)
	request.Persona = strings.TrimSpace(request.Persona)
	request.Goals = normalizeStringList(request.Goals)
	request.Rules = normalizeRules(request.Rules)
	request.MemoryInstructions = strings.TrimSpace(request.MemoryInstructions)
	request.SkillIDs = normalizeStringList(request.SkillIDs)
	if err := validatePersonaTemplate(scope, request.Name, request.Persona, request.Goals, request.Rules); err != nil {
		return nil, applicationError(err, "digital_human.persona_template.create")
	}
	if err := validatePersonaTemplateWork(identityName, request.Description, request.MemoryInstructions, request.SkillIDs); err != nil {
		return nil, applicationError(err, "digital_human.persona_template.create")
	}
	exists, err := s.repo.PersonaTemplateNameExists(ctx, scope, request.Name, uuid.Nil)
	if err != nil {
		return nil, applicationError(err, "digital_human.persona_template.create")
	}
	if exists {
		return nil, applicationError(ErrPersonaTemplateNameConflict, "digital_human.persona_template.create")
	}
	rules, err := marshalRules(request.Rules)
	if err != nil {
		return nil, applicationError(err, "digital_human.persona_template.create")
	}
	goals, err := marshalStringList(request.Goals)
	if err != nil {
		return nil, applicationError(err, "digital_human.persona_template.create")
	}
	skillIDs, err := marshalStringList(request.SkillIDs)
	if err != nil {
		return nil, applicationError(err, "digital_human.persona_template.create")
	}
	now := s.now()
	actorID := scope.ActorID
	template := &PersonaTemplate{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		Name: request.Name, IdentityName: identityName, Description: request.Description,
		Persona: request.Persona, Goals: goals, Rules: rules,
		MemoryInstructions: request.MemoryInstructions, SkillIDs: skillIDs,
		Revision: 1, CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreatePersonaTemplate(ctx, template); err != nil {
		return nil, applicationError(err, "digital_human.persona_template.create")
	}
	return template, nil
}

func (s *Service) GetPersonaTemplate(ctx context.Context, scope Scope, id uuid.UUID) (*PersonaTemplate, error) {
	template, err := s.repo.PersonaTemplate(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.persona_template.get")
	}
	return template, nil
}

func (s *Service) ListPersonaTemplates(ctx context.Context, scope Scope, query ListQuery) (*PersonaTemplateListResult, error) {
	query, err := normalizeListQuery(scope, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.persona_template.list")
	}
	items, total, err := s.repo.ListPersonaTemplates(ctx, scope, query)
	if err != nil {
		return nil, applicationError(err, "digital_human.persona_template.list")
	}
	return &PersonaTemplateListResult{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, HasMore: int64(query.Page*query.PageSize) < total}, nil
}

func (s *Service) UpdatePersonaTemplate(ctx context.Context, scope Scope, id uuid.UUID, request UpdatePersonaTemplateRequest) (*PersonaTemplate, error) {
	if request.ExpectedRevision < 1 || id == uuid.Nil || !validScope(scope) {
		return nil, applicationError(ErrInvalidRequest, "digital_human.persona_template.update")
	}
	current, err := s.repo.PersonaTemplate(ctx, scope, id)
	if err != nil {
		return nil, applicationError(err, "digital_human.persona_template.update")
	}
	name, persona := current.Name, current.Persona
	var goals []string
	if err := jsonUnmarshalRules(current.Goals, &goals); err != nil {
		return nil, applicationError(err, "digital_human.persona_template.update")
	}
	var rules []string
	if err := jsonUnmarshalRules(current.Rules, &rules); err != nil {
		return nil, applicationError(err, "digital_human.persona_template.update")
	}
	var skillIDs []string
	if err := jsonUnmarshalRules(current.SkillIDs, &skillIDs); err != nil {
		return nil, applicationError(err, "digital_human.persona_template.update")
	}
	identityName, description, memoryInstructions := current.IdentityName, current.Description, current.MemoryInstructions
	updates := make(map[string]any)
	if request.IdentityName != nil {
		identityName = strings.TrimSpace(*request.IdentityName)
		updates["identity_name"] = identityName
	}
	if request.Description != nil {
		description = strings.TrimSpace(*request.Description)
		updates["description"] = description
	}
	if request.MemoryInstructions != nil {
		memoryInstructions = strings.TrimSpace(*request.MemoryInstructions)
		updates["memory_instructions"] = memoryInstructions
	}
	if request.SkillIDs != nil {
		skillIDs = normalizeStringList(*request.SkillIDs)
		encoded, encodeErr := marshalStringList(skillIDs)
		if encodeErr != nil {
			return nil, applicationError(encodeErr, "digital_human.persona_template.update")
		}
		updates["skill_ids"] = encoded
	}
	if request.Name != nil {
		name = strings.TrimSpace(*request.Name)
		updates["name"] = name
	}
	if request.Persona != nil {
		persona = strings.TrimSpace(*request.Persona)
		updates["persona"] = persona
	}
	if request.Goals != nil {
		goals = normalizeStringList(*request.Goals)
		encoded, encodeErr := marshalStringList(goals)
		if encodeErr != nil {
			return nil, applicationError(encodeErr, "digital_human.persona_template.update")
		}
		updates["goals"] = encoded
	}
	if request.Rules != nil {
		rules = normalizeRules(*request.Rules)
		encoded, encodeErr := marshalRules(rules)
		if encodeErr != nil {
			return nil, applicationError(encodeErr, "digital_human.persona_template.update")
		}
		updates["rules"] = encoded
	}
	if len(updates) == 0 || validatePersonaTemplate(scope, name, persona, goals, rules) != nil || validatePersonaTemplateWork(identityName, description, memoryInstructions, skillIDs) != nil {
		return nil, applicationError(ErrInvalidRequest, "digital_human.persona_template.update")
	}
	if request.Name != nil {
		exists, existsErr := s.repo.PersonaTemplateNameExists(ctx, scope, name, id)
		if existsErr != nil {
			return nil, applicationError(existsErr, "digital_human.persona_template.update")
		}
		if exists {
			return nil, applicationError(ErrPersonaTemplateNameConflict, "digital_human.persona_template.update")
		}
	}
	if err := s.repo.UpdatePersonaTemplateCAS(ctx, scope, id, request.ExpectedRevision, updates); err != nil {
		return nil, applicationError(err, "digital_human.persona_template.update")
	}
	return s.GetPersonaTemplate(ctx, scope, id)
}

func (s *Service) DeletePersonaTemplate(ctx context.Context, scope Scope, id uuid.UUID, expectedRevision int64) error {
	if expectedRevision < 1 || id == uuid.Nil || !validScope(scope) {
		return applicationError(ErrInvalidRequest, "digital_human.persona_template.delete")
	}
	if _, err := s.repo.PersonaTemplate(ctx, scope, id); err != nil {
		return applicationError(err, "digital_human.persona_template.delete")
	}
	if err := s.repo.DeletePersonaTemplateCAS(ctx, scope, id, expectedRevision); err != nil {
		return applicationError(err, "digital_human.persona_template.delete")
	}
	return nil
}

func validatePersonaTemplate(scope Scope, name, persona string, goals, rules []string) error {
	if !validScope(scope) || name == "" || persona == "" || utf8.RuneCountInString(name) > maxNameRunes || utf8.RuneCountInString(persona) > maxPersonaRunes || !validStringList(goals, maxGoals, maxGoalRunes) || len(rules) > maxRules {
		return ErrInvalidRequest
	}
	for _, rule := range rules {
		if rule == "" || utf8.RuneCountInString(rule) > maxRuleRunes {
			return ErrInvalidRequest
		}
	}
	return nil
}

// maxTemplateSkills bounds the skills one saved template binds.
const maxTemplateSkills = 32

// validatePersonaTemplateWork checks what a saved template carries beyond its
// persona (ADR 0022). An empty name in conversation leaves a DigitalHuman's
// name as it is.
func validatePersonaTemplateWork(identityName, description, memoryInstructions string, skillIDs []string) error {
	if utf8.RuneCountInString(identityName) > maxNameRunes || utf8.RuneCountInString(description) > maxDescriptionRunes ||
		utf8.RuneCountInString(memoryInstructions) > maxMemoryInstructionsRunes || !validStringList(skillIDs, maxTemplateSkills, maxNameRunes*4) {
		return ErrInvalidRequest
	}
	return nil
}

// ListTemplates returns the template catalog: the built-in templates in the
// requested locale, then up to a page of the workspace's saved templates
// (ADR 0022). A search narrows both.
func (s *Service) ListTemplates(ctx context.Context, scope Scope, locale, search string) (*TemplateCatalog, error) {
	catalog := BuiltinTemplates(locale)
	search = strings.TrimSpace(search)
	if search != "" {
		needle := strings.ToLower(search)
		matched := catalog.Templates[:0]
		for _, template := range catalog.Templates {
			if strings.Contains(strings.ToLower(template.Title+"\n"+template.Description), needle) {
				matched = append(matched, template)
			}
		}
		catalog.Templates = matched
	}
	saved, err := s.ListPersonaTemplates(ctx, scope, ListQuery{Page: 1, PageSize: maxPageSize, Search: search})
	if err != nil {
		return nil, err
	}
	for _, item := range saved.Items {
		template, convertErr := savedTemplate(item)
		if convertErr != nil {
			return nil, applicationError(convertErr, "digital_human.persona_template.list")
		}
		catalog.Templates = append(catalog.Templates, template)
	}
	return &catalog, nil
}

// savedTemplate presents a workspace's saved template in the catalog. Its
// skills are suggestions: the workspace chose them, and may since have
// removed one.
func savedTemplate(item *PersonaTemplate) (Template, error) {
	var goals, rules, skillIDs []string
	if err := jsonUnmarshalRules(item.Goals, &goals); err != nil {
		return Template{}, err
	}
	if err := jsonUnmarshalRules(item.Rules, &rules); err != nil {
		return Template{}, err
	}
	if len(item.SkillIDs) > 0 {
		if err := jsonUnmarshalRules(item.SkillIDs, &skillIDs); err != nil {
			return Template{}, err
		}
	}
	skills := make([]TemplateSkill, 0, len(skillIDs))
	for _, id := range skillIDs {
		skills = append(skills, TemplateSkill{ID: id})
	}
	return Template{
		ID: item.ID.String(), Source: TemplateSourceWorkspace, Title: item.Name, Description: item.Description,
		IdentityName: item.IdentityName, Responsibility: item.Description, Persona: item.Persona,
		Goals: nonNilStrings(goals), Rules: nonNilStrings(rules), Tags: []string{}, Skills: skills,
		MemoryInstructions: item.MemoryInstructions, Revision: item.Revision,
	}, nil
}

func jsonUnmarshalRules(encoded []byte, target *[]string) error {
	if err := json.Unmarshal(encoded, target); err != nil {
		return ErrInvalidRequest
	}
	return nil
}
