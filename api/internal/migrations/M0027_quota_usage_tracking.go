package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0027_quota_usage_tracking creates quota usage tracking table
func M0027_quota_usage_tracking() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251127000000",
		Migrate: func(tx *gorm.DB) error {
			// Create quota_usage_history table
			if err := tx.Exec(`
				CREATE TABLE "public"."quota_usage_history" (
					"id" varchar(255) NOT NULL,
					"group_id" uuid NOT NULL,
					"account_id" uuid NOT NULL,
					"tenant_id" uuid,
					"resource_type" varchar(50) NOT NULL,
					"operation_type" varchar(20) NOT NULL,
					"delta" bigint NOT NULL,
					"value_before" bigint NOT NULL,
					"value_after" bigint NOT NULL,
					"resource_id" varchar(255),
					"resource_name" varchar(500),
					"metadata" jsonb,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE INDEX idx_quota_history_group ON quota_usage_history(group_id);
				CREATE INDEX idx_quota_history_account ON quota_usage_history(account_id);
				CREATE INDEX idx_quota_history_tenant ON quota_usage_history(tenant_id);
				CREATE INDEX idx_quota_history_resource_type ON quota_usage_history(resource_type);
				CREATE INDEX idx_quota_history_created_at ON quota_usage_history(created_at);
				CREATE INDEX idx_quota_history_group_resource ON quota_usage_history(group_id, resource_type);
				CREATE INDEX idx_quota_history_group_created ON quota_usage_history(group_id, created_at);
				
				COMMENT ON TABLE quota_usage_history IS '配额使用历史记录表';
				COMMENT ON COLUMN quota_usage_history.group_id IS '组织ID';
				COMMENT ON COLUMN quota_usage_history.account_id IS '操作账号ID';
				COMMENT ON COLUMN quota_usage_history.tenant_id IS '部门ID(可选)';
				COMMENT ON COLUMN quota_usage_history.resource_type IS '资源类型: seats, storage, db_rows, knowledge_bases, ai_agents, workflows, workflow_executions';
				COMMENT ON COLUMN quota_usage_history.operation_type IS '操作类型: increase(增加), decrease(减少)';
				COMMENT ON COLUMN quota_usage_history.delta IS '变化量,正数表示增加,负数表示减少';
				COMMENT ON COLUMN quota_usage_history.value_before IS '变化前的值';
				COMMENT ON COLUMN quota_usage_history.value_after IS '变化后的值';
				COMMENT ON COLUMN quota_usage_history.resource_id IS '关联的资源ID(如文件ID、知识库ID等)';
				COMMENT ON COLUMN quota_usage_history.resource_name IS '资源名称';
				COMMENT ON COLUMN quota_usage_history.metadata IS '详细元数据,JSON格式,包含操作的详细信息';
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS quota_usage_history CASCADE").Error
		},
	}
}
