package start

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
)

type NodeData struct {
	base.NodeData
	Variables []VariableEntity `json:"variables"`
}

type VariableEntity struct {
	Val                    string
	Label                  string
	Desc                   string
	Kind                   VariableEntityType
	Required               bool
	Hide                   bool
	MaxLen                 int
	Option                 []string
	AllowFileTypes         []FileType
	AllowFileExtensions    []string
	AllowFileUploadMethods []FileTransferMethod
}

type VariableEntityType string

var (
	TextInput        VariableEntityType = "text-input"
	SELECT           VariableEntityType = "select"
	ParaGraph        VariableEntityType = "paragraph"
	Number           VariableEntityType = "number"
	DateTime         VariableEntityType = "datetime"
	ExternalDataTool VariableEntityType = "external_data_tool"
	File             VariableEntityType = "file"
	FileList         VariableEntityType = "file-list"
	ModelConfig      VariableEntityType = "model_config"
)

type FileType string

var (
	IMAGE    FileType = "image"
	DOCUMENT FileType = "document"
	AUDIO    FileType = "audio"
	VIDEO    FileType = "video"
	CUSTOM   FileType = "custom"
)

type FileTransferMethod string

var (
	RemoteUrl = "remote_url"
	LocalFile = "local_file"
	ToolFile  = "tool_file"
)
