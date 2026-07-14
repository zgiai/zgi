package workflowtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateGeneratedAssetBindingsAcceptsMatchingGeneratedFile(t *testing.T) {
	turns := CaseTurns{{
		Role: "user",
		Attachments: []CaseAttachment{{
			TransferMethod: "local_file",
			UploadFileID:   "file-1",
			Name:           "sample.docx",
		}},
		Inputs: JSONMap{
			generatedAssetSourceKey: generatedAssetSource,
			generatedFixtureSpecKey: []map[string]interface{}{{
				"upload_file_id": "file-1",
				"name":           "sample.docx",
			}},
		},
	}}

	require.NoError(t, validateGeneratedAssetBindings(turns))
}

func TestValidateGeneratedAssetBindingsAcceptsManualAttachment(t *testing.T) {
	turns := CaseTurns{{
		Role: "user",
		Attachments: []CaseAttachment{{
			TransferMethod: "local_file",
			UploadFileID:   "manual-file",
			Name:           "manual.docx",
		}},
	}}

	require.NoError(t, validateGeneratedAssetBindings(turns))
}

func TestValidateGeneratedAssetBindingsRejectsMissingGeneratedFixtureSpec(t *testing.T) {
	turns := CaseTurns{{
		Role: "user",
		Inputs: JSONMap{
			generatedAssetSourceKey: generatedAssetSource,
		},
	}}

	err := validateGeneratedAssetBindings(turns)
	require.Error(t, err)
	require.Contains(t, err.Error(), "缺少系统生成文件说明")
}

func TestValidateGeneratedAssetBindingsRejectsAttachmentMismatch(t *testing.T) {
	turns := CaseTurns{{
		Role: "user",
		Attachments: []CaseAttachment{{
			TransferMethod: "local_file",
			UploadFileID:   "previous-file",
			Name:           "previous.pdf",
		}},
		Inputs: JSONMap{
			generatedAssetSourceKey: generatedAssetSource,
			generatedFixtureSpecKey: []interface{}{map[string]interface{}{
				"upload_file_id": "generated-file",
				"name":           "generated.docx",
			}},
		},
	}}

	err := validateGeneratedAssetBindings(turns)
	require.Error(t, err)
	require.Contains(t, err.Error(), "系统生成文件与实际附件不一致")
	require.Contains(t, err.Error(), "generated.docx")
}

func TestValidateGeneratedAssetExtractionRejectsGarbledText(t *testing.T) {
	fixture := GeneratedFileFixture{
		Format:  "pdf",
		Content: "第1章 安全须知\n警告：操作前务必断电。\n按下 POWER 键。",
	}

	require.NoError(t, validateGeneratedAssetExtraction(fixture, "第1章 安全须知 警告：操作前务必断电。按下 POWER 键。"))
	err := validateGeneratedAssetExtraction(fixture, "嬀Yd蚈bKQ 媐TJd蚈RMR舉璾50 c N POWER")
	require.Error(t, err)
	require.Contains(t, err.Error(), "回读内容与生成内容不一致")
}
