package v1

import (
	"time"

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
		Endpoint:              appconfig.GlobalConfig.CodeExec.Endpoint,
		APIKey:                appconfig.GlobalConfig.CodeExec.APIKey,
		ConnectTimeout:        secondsDuration(appconfig.GlobalConfig.CodeExec.ConnectTimeoutSeconds),
		CreateTimeout:         secondsDuration(appconfig.GlobalConfig.CodeExec.CreateTimeoutSeconds),
		UploadTimeout:         secondsDuration(appconfig.GlobalConfig.CodeExec.UploadTimeoutSeconds),
		CommandTimeoutPadding: secondsDuration(appconfig.GlobalConfig.CodeExec.CommandTimeoutPaddingSeconds),
		ArtifactTimeout:       secondsDuration(appconfig.GlobalConfig.CodeExec.ArtifactTimeoutSeconds),
		CleanupTimeout:        secondsDuration(appconfig.GlobalConfig.CodeExec.CleanupTimeoutSeconds),
	}))
}

func secondsDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
