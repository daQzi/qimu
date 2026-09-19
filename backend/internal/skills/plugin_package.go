package skills

import (
	"fmt"
	"infinite-canvas/backend/internal/model"
)

// PreparePluginSkill reuses the skill archive format without publishing DB
// rows. The plugin catalog transaction publishes all bindings atomically.
func (s *Service) PreparePluginSkill(skillID, versionID, name, description, label, publisher string, files map[string][]byte) (*model.Skill, *model.SkillVersion, []model.SkillFile, error) {
	var total int
	for _, raw := range files {
		total += len(raw)
		if len(raw) > maxSkillFileBytes {
			return nil, nil, nil, fmt.Errorf("plugin skill file exceeds 8 MiB")
		}
	}
	if total > maxSkillPackageBytes {
		return nil, nil, nil, fmt.Errorf("plugin skill exceeds 20 MiB")
	}
	archive, err := finalizeSkillArchive(files, skillPackageMetadata{Name: name, Description: description, Version: label})
	if err != nil {
		return nil, nil, nil, err
	}
	_, version, entries, err := s.persistSkillArchive(skillID, versionID, archive, "")
	if err != nil {
		return nil, nil, nil, err
	}
	version.VersionLabel = label
	skill := &model.Skill{ID: skillID, Name: name, Description: description, Instruction: string(files["SKILL.md"]), CurrentVersionID: versionID, VersionLabel: label, ContentHash: archive.ContentHash, FileCount: len(files), TotalBytes: archive.TotalBytes, SourceType: "plugin", Source: skillSourceUser, Status: skillStatusEnabled, Tag: "others", AuthorName: publisher, ShowcaseMediaJSON: "[]", SyncStatus: "synced"}
	return skill, version, entries, nil
}
