package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0126_seed_subscription_plans_base ensures base subscription plans exist.
// Some environments ran schema migrations without running seed data, leaving
// subscription_plans empty and breaking free-plan fallback flows.
func M0126_seed_subscription_plans_base() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000126",
		Migrate: func(tx *gorm.DB) error {
			sql := `
INSERT INTO subscription_plans (
	id,
	plan_code,
	plan_name,
	display_order,
	pricing,
	currency,
	quota_config,
	features,
	description,
	is_active,
	created_at,
	updated_at
) VALUES
(
	'plan_free_001',
	'free',
	'Free',
	1,
	'{"monthly": {"price": 0, "original_price": null, "discount_label": null}}'::jsonb,
	'CNY',
	'{
		"seats": 1,
		"storage_mb": 1024,
		"db_rows": 5000,
		"knowledge_bases": 2,
		"ai_agents": 3,
		"ocr_page": 2,
		"monthly_ai_credits": 10000000,
		"workflow_executions": 200,
		"features": {
			"core_features": true,
			"batch_testing": false,
			"audit_logs": false,
			"custom_plugins": false,
			"private_deployment": false
		}
	}'::jsonb,
	'{
		"target_users": ["individual", "developer", "team_trial"],
		"feature_tags": ["core_features", "community_support"]
	}'::jsonb,
	'Free trial plan for individuals and developers',
	true,
	CURRENT_TIMESTAMP,
	CURRENT_TIMESTAMP
),
(
	'plan_team_001',
	'team',
	'Team',
	2,
	'{
		"monthly": {"price": 1999.00, "original_price": null, "discount_label": null},
		"yearly": {"price": 19990.00, "original_price": 23988.00, "discount_label": "save_17_percent"}
	}'::jsonb,
	'CNY',
	'{
		"seats": 5,
		"storage_mb": 51200,
		"db_rows": 10000,
		"knowledge_bases": 20,
		"ai_agents": 30,
		"ocr_page": 20,
		"monthly_ai_credits": 100000000,
		"workflow_executions": 5000,
		"features": {
			"core_features": true,
			"batch_testing": true,
			"audit_logs": false,
			"custom_plugins": true,
			"private_deployment": false
		}
	}'::jsonb,
	'{
		"target_users": ["small_team"],
		"feature_tags": ["team_collaboration", "batch_testing", "custom_plugins", "priority_support"]
	}'::jsonb,
	'Team plan for small teams and startups',
	true,
	CURRENT_TIMESTAMP,
	CURRENT_TIMESTAMP
),
(
	'plan_business_001',
	'business',
	'Business',
	3,
	'{
		"monthly": {"price": 5999.00, "original_price": null, "discount_label": null},
		"yearly": {"price": 59990.00, "original_price": 71988.00, "discount_label": "save_17_percent"}
	}'::jsonb,
	'CNY',
	'{
		"seats": 20,
		"storage_mb": 204800,
		"db_rows": 100000,
		"knowledge_bases": -1,
		"ai_agents": -1,
		"ocr_page": 60,
		"monthly_ai_credits": 500000000,
		"workflow_executions": 20000,
		"features": {
			"core_features": true,
			"batch_testing": true,
			"audit_logs": true,
			"custom_plugins": true,
			"private_deployment": false
		}
	}'::jsonb,
	'{
		"target_users": ["medium_enterprise"],
		"feature_tags": ["unlimited_knowledge_bases", "unlimited_agents", "audit_logs", "advanced_features", "priority_support"]
	}'::jsonb,
	'Business plan for medium-sized enterprises',
	true,
	CURRENT_TIMESTAMP,
	CURRENT_TIMESTAMP
),
(
	'plan_enterprise_001',
	'enterprise',
	'Enterprise',
	4,
	'{
		"custom": {"price": 0, "original_price": null, "discount_label": "contact_sales", "min_seats": 50, "billing_mode": "annual_contract"}
	}'::jsonb,
	'CNY',
	'{
		"seats": -1,
		"storage_mb": -1,
		"db_rows": -1,
		"knowledge_bases": -1,
		"ai_agents": -1,
		"ocr_page": -1,
		"monthly_ai_credits": -1,
		"workflow_executions": -1,
		"features": {
			"core_features": true,
			"batch_testing": true,
			"audit_logs": true,
			"custom_plugins": true,
			"private_deployment": true
		}
	}'::jsonb,
	'{
		"target_users": ["large_enterprise", "group"],
		"feature_tags": ["unlimited_all", "private_deployment", "dedicated_support", "sla_guarantee", "custom_development"]
	}'::jsonb,
	'Enterprise plan with custom pricing for large organizations',
	true,
	CURRENT_TIMESTAMP,
	CURRENT_TIMESTAMP
)
ON CONFLICT (plan_code) DO UPDATE SET
	plan_name = EXCLUDED.plan_name,
	display_order = EXCLUDED.display_order,
	pricing = EXCLUDED.pricing,
	currency = EXCLUDED.currency,
	quota_config = EXCLUDED.quota_config,
	features = EXCLUDED.features,
	description = EXCLUDED.description,
	is_active = EXCLUDED.is_active,
	updated_at = CURRENT_TIMESTAMP;
`

			return tx.Exec(sql).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
