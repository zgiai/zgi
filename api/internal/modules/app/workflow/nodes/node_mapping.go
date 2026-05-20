package nodes

import (
	"fmt"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/answer"
	approvalnode "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/approval"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/calldatabase"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/code"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/condbranch"
	createscheduledtask "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/create_scheduled_task"
	documentextractor "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/document_extractor"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/end"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/httprequest"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/imagegen"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/iter"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/jsonparser"
	knowledgeretrieval "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/knowledge_retrieval"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/llm"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/loop"
	notificationsmsnode "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/notification_sms"
	parameterextractor "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/parameter_extractor"
	questionanswer "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/question_answer"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/sqlgenerator"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/start"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/tools"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/varassign"
	variableaggregator "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/variable_aggregator"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

const (
	LatestVersion = "latest"
	Version1      = "v1"
	Version2      = "v2"
)

type NodeFactory func(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error)

var NodeTypeClassesMapping = map[shared.NodeType]map[string]NodeFactory{
	// Basic flow nodes
	shared.Start: {
		LatestVersion: start.New,
		Version1:      start.New,
	},
	shared.End: {
		LatestVersion: end.New,
		Version1:      end.New,
	},
	shared.HTTPRequest: {
		LatestVersion: httprequest.NewHTTPRequestNode,
		Version1:      httprequest.NewHTTPRequestNode,
	},
	shared.LLM: {
		LatestVersion: llm.New,
		Version1:      llm.New,
	},
	shared.KnowledgeRetrieval: {
		LatestVersion: knowledgeretrieval.New,
		Version1:      knowledgeretrieval.New,
	},
	shared.Code: {
		LatestVersion: code.New,
		Version1:      code.New,
	},
	shared.Answer: {
		LatestVersion: answer.New,
		Version1:      answer.New,
	},
	shared.VariableAssigner: {
		LatestVersion: varassign.New,
		Version1:      varassign.New,
	},
	shared.IfElse: {
		LatestVersion: condbranch.New,
		Version1:      condbranch.New,
	},
	shared.Iteration: {
		LatestVersion: iter.New,
		Version1:      iter.New,
	},
	shared.IterationStart: {
		LatestVersion: iter.NewStartNode,
		Version1:      iter.NewStartNode,
	},
	shared.Loop: {
		LatestVersion: loop.New,
		Version1:      loop.New,
	},
	shared.LoopStart: {
		LatestVersion: loop.NewStartNode,
		Version1:      loop.NewStartNode,
	},
	shared.LoopEnd: {
		LatestVersion: loop.NewEndNode,
		Version1:      loop.NewEndNode,
	},
	shared.Tools: {
		LatestVersion: tools.New,
		Version1:      tools.New,
	},
	shared.CallDatabase: {
		LatestVersion: calldatabase.New,
		Version1:      calldatabase.New,
	},
	shared.SQLGenerator: {
		LatestVersion: sqlgenerator.New,
		Version1:      sqlgenerator.New,
	},
	shared.DocumentExtractor: {
		LatestVersion: documentextractor.New,
		Version1:      documentextractor.New,
	},
	shared.ImageGen: {
		LatestVersion: imagegen.New,
		Version1:      imagegen.New,
	},
	shared.VariableAggregator: {
		LatestVersion: variableaggregator.New,
		Version1:      variableaggregator.New,
	},
	shared.ParameterExtractor: {
		LatestVersion: parameterextractor.New,
		Version1:      parameterextractor.New,
	},
	shared.JSONParser: {
		LatestVersion: jsonparser.New,
		Version1:      jsonparser.New,
	},
	shared.CreateScheduledTask: {
		LatestVersion: createscheduledtask.New,
		Version1:      createscheduledtask.New,
	},
	shared.Approval: {
		LatestVersion: approvalnode.New,
		Version1:      approvalnode.New,
	},
	shared.QuestionAnswer: {
		LatestVersion: questionanswer.New,
		Version1:      questionanswer.New,
	},
	shared.NotificationSMS: {
		LatestVersion: notificationsmsnode.New,
		Version1:      notificationsmsnode.New,
	},
}

// GetNodeFactory gets node factory by type and version
func GetNodeFactory(nodeType shared.NodeType, version string) (NodeFactory, error) {
	// 1. Find node type from mapping table
	nodeVersions, exists := NodeTypeClassesMapping[nodeType]
	if !exists {
		return nil, fmt.Errorf("node type %s not found in mapping", nodeType)
	}

	// 2. Get corresponding factory function by version
	nodeFactory, exists := nodeVersions[version]
	if !exists {
		// 3. If version doesn't exist, try to use latest version
		nodeFactory, exists = nodeVersions[LatestVersion]
		if !exists {
			return nil, fmt.Errorf("node class not found for node type %s version %v", nodeType, version)
		}
	}

	// 4. Return factory function or error
	return nodeFactory, nil
}

// GetSupportedVersions gets all supported versions for node type
func GetSupportedVersions(nodeType shared.NodeType) ([]string, error) {
	nodeVersions, exists := NodeTypeClassesMapping[nodeType]
	if !exists {
		return nil, fmt.Errorf("node type %s not found in mapping", nodeType)
	}

	versions := make([]string, 0, len(nodeVersions))
	for version := range nodeVersions {
		versions = append(versions, version)
	}

	return versions, nil
}

// IsNodeTypeSupported checks if node type is supported
func IsNodeTypeSupported(nodeType shared.NodeType) bool {
	_, exists := NodeTypeClassesMapping[nodeType]
	return exists
}

// GetNodeTypeCount gets total count of supported node types
func GetNodeTypeCount() int {
	return len(NodeTypeClassesMapping)
}

// GetNodeTypeList gets list of all supported node types
func GetNodeTypeList() []shared.NodeType {
	nodeTypes := make([]shared.NodeType, 0, len(NodeTypeClassesMapping))
	for nodeType := range NodeTypeClassesMapping {
		nodeTypes = append(nodeTypes, nodeType)
	}
	return nodeTypes
}

// ValidateNodeTypeAndVersion validates if node type and version are valid
func ValidateNodeTypeAndVersion(nodeType shared.NodeType, version string) error {
	if !IsNodeTypeSupported(nodeType) {
		return fmt.Errorf("unsupported node type: %s", nodeType)
	}

	supportedVersions, err := GetSupportedVersions(nodeType)
	if err != nil {
		return err
	}

	for _, supportedVersion := range supportedVersions {
		if supportedVersion == version {
			return nil
		}
	}

	return fmt.Errorf("unsupported version %s for node type %s", version, nodeType)
}
