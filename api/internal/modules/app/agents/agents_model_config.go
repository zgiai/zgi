package agents

import (
	"time"

	"github.com/google/uuid"
)

// AgentsConfig represents default/template-level model and inference configuration for an agent.
type AgentsConfig struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	AgentsID               uuid.UUID  `gorm:"type:uuid;not null;index:agents_agents_id_idx" json:"agents_id"`
	ModelProvider          *string    `gorm:"type:varchar(255);column:model_provider" json:"model_provider"`
	ModelVersionID         *string    `gorm:"type:varchar(255);column:model_version_id" json:"model_version_id"`
	Configs                *string    `gorm:"type:json" json:"configs"`
	CreatedBy              *uuid.UUID `gorm:"type:uuid;column:created_by" json:"created_by"`
	CreatedAt              time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedBy              *uuid.UUID `gorm:"type:uuid;column:updated_by" json:"updated_by"`
	UpdatedAt              time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedBy              *uuid.UUID `gorm:"type:uuid;column:deleted_by" json:"deleted_by"`
	DeletedAt              *time.Time `gorm:"column:deleted_at" json:"deleted_at"`
	GreetingMessage        *string    `gorm:"type:text;column:greeting_message" json:"greeting_message"`
	UserInputForm          *string    `gorm:"type:text;column:user_input_form" json:"user_input_form"`
	DatasetQueryVariable   *string    `gorm:"type:varchar(255);column:dataset_query_variable" json:"dataset_query_variable"`
	PrePrompt              *string    `gorm:"type:text;column:pre_prompt" json:"pre_prompt"`
	AgentMode              *string    `gorm:"type:text;column:agent_mode" json:"agent_mode"`
	SensitiveWordAvoidance *string    `gorm:"type:text;column:sensitive_word_avoidance" json:"sensitive_word_avoidance"`
	RetrieverResource      *string    `gorm:"type:text;column:retriever_resource" json:"retriever_resource"`
	PromptType             string     `gorm:"type:varchar(255);not null;default:'simple';column:prompt_type" json:"prompt_type"`
	ChatPromptConfig       *string    `gorm:"type:text;column:chat_prompt_config" json:"chat_prompt_config"`
	CompletionPromptConfig *string    `gorm:"type:text;column:completion_prompt_config" json:"completion_prompt_config"`
	DatasetConfigs         *string    `gorm:"type:text;column:dataset_configs" json:"dataset_configs"`
	ExternalDataTools      *string    `gorm:"type:text;column:external_data_tools" json:"external_data_tools"`
	FileUpload             *string    `gorm:"type:text;column:file_upload" json:"file_upload"`
}

// TableName overrides the table name used by GORM for AgentsConfig.
func (AgentsConfig) TableName() string {
	return "agents_configs"
}
