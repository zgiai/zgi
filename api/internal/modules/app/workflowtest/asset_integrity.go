package workflowtest

import (
	"encoding/json"
	"fmt"
	"strings"
)

type generatedAssetReference struct {
	UploadFileID string `json:"upload_file_id"`
	Name         string `json:"name"`
}

func validateGeneratedAssetBindings(turns CaseTurns) error {
	for turnIndex, turn := range turns {
		if turn.Inputs == nil {
			continue
		}
		source, _ := turn.Inputs[generatedAssetSourceKey].(string)
		rawFixtures, hasFixtures := turn.Inputs[generatedFixtureSpecKey]
		if strings.TrimSpace(source) != generatedAssetSource && !hasFixtures {
			continue
		}

		fixtures, err := decodeGeneratedAssetReferences(rawFixtures)
		if err != nil {
			return fmt.Errorf("第 %d 轮系统生成文件说明无效: %w", turnIndex+1, err)
		}
		if len(fixtures) == 0 {
			return fmt.Errorf("第 %d 轮缺少系统生成文件说明", turnIndex+1)
		}

		attachments := make(map[string]CaseAttachment, len(turn.Attachments))
		for _, attachment := range turn.Attachments {
			fileID := strings.TrimSpace(attachment.UploadFileID)
			if fileID != "" {
				attachments[fileID] = attachment
			}
		}
		for _, fixture := range fixtures {
			fileID := strings.TrimSpace(fixture.UploadFileID)
			if fileID == "" {
				return fmt.Errorf("第 %d 轮系统生成文件说明缺少文件 ID", turnIndex+1)
			}
			attachment, exists := attachments[fileID]
			if !exists {
				name := strings.TrimSpace(fixture.Name)
				if name == "" {
					name = fileID
				}
				return fmt.Errorf("第 %d 轮系统生成文件与实际附件不一致：缺少 %s", turnIndex+1, name)
			}
			fixtureName := strings.TrimSpace(fixture.Name)
			attachmentName := strings.TrimSpace(attachment.Name)
			if fixtureName != "" && attachmentName != "" && fixtureName != attachmentName {
				return fmt.Errorf("第 %d 轮系统生成文件名称与实际附件不一致：%s / %s", turnIndex+1, fixtureName, attachmentName)
			}
		}
	}
	return nil
}

func decodeGeneratedAssetReferences(value interface{}) ([]generatedAssetReference, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fixtures []generatedAssetReference
	if err := json.Unmarshal(data, &fixtures); err != nil {
		return nil, err
	}
	return fixtures, nil
}
