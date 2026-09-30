package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var retiredSkillContract = regexp.MustCompile(`(?i)\b(integration_requirements|integration_id|allowed_action_ids|execute_action|get_action_guide|external_apps)\b`)

// Inspect external manifests without modifying user-authored files. The root is
// explicit and every resolved file must remain under it, including symlinks.
// Only a digest and classification enter the plan; file text never enters reports.
func inventorySkillFiles(snapshot databaseSnapshot, root string) error {
	snapshot["connector_skill_file_inventory"] = []row{}
	for _, skill := range snapshot["chat_runtime_custom_skills"] {
		if text(skill, "storage_path") == "" {
			continue
		}
		if root == "" {
			return errors.New("--skills-root is required to verify stored Skill manifests")
		}
		base, err := filepath.EvalSymlinks(root)
		if err != nil {
			return errors.New("cannot resolve Skill storage root")
		}
		base, err = filepath.Abs(base)
		if err != nil {
			return errors.New("cannot resolve Skill storage root")
		}
		dir := text(skill, "storage_path")
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(base, dir)
		}
		path, err := filepath.EvalSymlinks(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			snapshot["connector_skill_file_inventory"] = append(snapshot["connector_skill_file_inventory"], row{"id": skill["id"], "state": "unreadable"})
			continue
		}
		rel, err := filepath.Rel(base, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return errors.New("Skill manifest resolves outside --skills-root")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
			return errors.New("Skill manifest exceeds bounded file contract")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return errors.New("cannot inspect Skill manifest")
		}
		state := "native"
		if retiredSkillContract.Match(data) {
			state = "requires_reimport"
		}
		snapshot["connector_skill_file_inventory"] = append(snapshot["connector_skill_file_inventory"], row{"id": skill["id"], "state": state, "digest": hash(string(data))})
		clear(data)
	}
	return nil
}
