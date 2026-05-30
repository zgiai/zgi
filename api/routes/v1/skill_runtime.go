package v1

import (
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func newSkillRuntimeWithSandbox(toolEngine *tools.ToolEngine, toolManager *tools.ToolManager) *skills.Runtime {
	runtime := skills.NewRuntime(toolEngine, toolManager)
	if appconfig.GlobalConfig == nil {
		return runtime
	}
	return runtime.WithScriptRunner(skills.NewSandboxScriptRunner(skills.SandboxScriptRunnerConfig{
		Endpoint: appconfig.GlobalConfig.CodeExec.Endpoint,
		APIKey:   appconfig.GlobalConfig.CodeExec.APIKey,
	}))
}
