package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0030_enterprise_invite creates tables for enterprise invite links and join requests
func M0030_enterprise_invite() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251215000000",
		Migrate: func(tx *gorm.DB) error {
			// 1. Create enterprise_invite_links table
			if err := tx.Exec(`
				CREATE TABLE "public"."enterprise_invite_links" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"group_id" uuid NOT NULL,
					"department_id" uuid,
					"tenant_id" uuid,
					"token" varchar(255) NOT NULL,
					"status" varchar(32) NOT NULL,
					"require_approval" bool NOT NULL DEFAULT true,
					"default_group_role" varchar(32) NOT NULL DEFAULT 'normal',
					"default_tenant_role" varchar(32) NOT NULL DEFAULT 'normal',
					"expires_at" timestamptz,
					"created_by" uuid NOT NULL,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id"),
					CONSTRAINT uk_enterprise_invite_links_token UNIQUE (token),
					CONSTRAINT fk_eil_group FOREIGN KEY (group_id)
						REFERENCES enterprise_groups(id) ON DELETE CASCADE,
					CONSTRAINT fk_eil_department FOREIGN KEY (department_id)
						REFERENCES departments(id) ON DELETE SET NULL,
					CONSTRAINT fk_eil_tenant FOREIGN KEY (tenant_id)
						REFERENCES tenants(id) ON DELETE SET NULL,
					CONSTRAINT fk_eil_created_by FOREIGN KEY (created_by)
						REFERENCES accounts(id) ON DELETE RESTRICT
				);

				CREATE INDEX idx_eil_group_id ON enterprise_invite_links(group_id);
				CREATE INDEX idx_eil_department_id ON enterprise_invite_links(department_id);
				CREATE INDEX idx_eil_status ON enterprise_invite_links(status);
				CREATE INDEX idx_eil_expires_at ON enterprise_invite_links(expires_at);

				COMMENT ON TABLE enterprise_invite_links IS 'Enterprise invite links for departments and tenants';
				COMMENT ON COLUMN enterprise_invite_links.group_id IS 'Enterprise group ID';
				COMMENT ON COLUMN enterprise_invite_links.department_id IS 'Target department ID (current phase only department invites are used)';
				COMMENT ON COLUMN enterprise_invite_links.tenant_id IS 'Target tenant ID (reserved for future use)';
				COMMENT ON COLUMN enterprise_invite_links.token IS 'Invite token, high entropy random string';
				COMMENT ON COLUMN enterprise_invite_links.status IS 'Invite link status: enabled/disabled/expired/reset';
				COMMENT ON COLUMN enterprise_invite_links.require_approval IS 'Whether join request requires admin approval';
				COMMENT ON COLUMN enterprise_invite_links.default_group_role IS 'Default enterprise group role for joined member';
				COMMENT ON COLUMN enterprise_invite_links.default_tenant_role IS 'Default tenant role for joined member (reserved)';
				COMMENT ON COLUMN enterprise_invite_links.expires_at IS 'Expire time of the invite link, null means never expires';
			`).Error; err != nil {
				return err
			}

			// 2. Create enterprise_join_requests table
			if err := tx.Exec(`
				CREATE TABLE "public"."enterprise_join_requests" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"group_id" uuid NOT NULL,
					"invite_link_id" uuid,
					"account_id" uuid NOT NULL,
					"department_id" uuid,
					"tenant_id" uuid,
					"default_group_role" varchar(32) NOT NULL,
					"default_tenant_role" varchar(32) NOT NULL,
					"status" varchar(32) NOT NULL,
					"reason" text,
					"reviewer_id" uuid,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"reviewed_at" timestamptz,
					PRIMARY KEY ("id"),
					CONSTRAINT fk_ejr_group FOREIGN KEY (group_id)
						REFERENCES enterprise_groups(id) ON DELETE CASCADE,
					CONSTRAINT fk_ejr_invite_link FOREIGN KEY (invite_link_id)
						REFERENCES enterprise_invite_links(id) ON DELETE SET NULL,
					CONSTRAINT fk_ejr_account FOREIGN KEY (account_id)
						REFERENCES accounts(id) ON DELETE CASCADE,
					CONSTRAINT fk_ejr_department FOREIGN KEY (department_id)
						REFERENCES departments(id) ON DELETE SET NULL,
					CONSTRAINT fk_ejr_tenant FOREIGN KEY (tenant_id)
						REFERENCES tenants(id) ON DELETE SET NULL,
					CONSTRAINT fk_ejr_reviewer FOREIGN KEY (reviewer_id)
						REFERENCES accounts(id) ON DELETE SET NULL
				);

				CREATE INDEX idx_ejr_group_id ON enterprise_join_requests(group_id);
				CREATE INDEX idx_ejr_account_id ON enterprise_join_requests(account_id);
				CREATE INDEX idx_ejr_status ON enterprise_join_requests(status);
				CREATE INDEX idx_ejr_department_id ON enterprise_join_requests(department_id);

				COMMENT ON TABLE enterprise_join_requests IS 'Enterprise join requests created by invite links or admin actions';
				COMMENT ON COLUMN enterprise_join_requests.group_id IS 'Enterprise group ID';
				COMMENT ON COLUMN enterprise_join_requests.invite_link_id IS 'Related invite link ID, null if created directly by admin';
				COMMENT ON COLUMN enterprise_join_requests.account_id IS 'Applicant account ID';
				COMMENT ON COLUMN enterprise_join_requests.department_id IS 'Target department ID';
				COMMENT ON COLUMN enterprise_join_requests.tenant_id IS 'Target tenant ID (reserved for future use)';
				COMMENT ON COLUMN enterprise_join_requests.default_group_role IS 'Default enterprise group role for applicant';
				COMMENT ON COLUMN enterprise_join_requests.default_tenant_role IS 'Default tenant role for applicant (reserved)';
				COMMENT ON COLUMN enterprise_join_requests.status IS 'Join request status: pending/approved/rejected/expired';
				COMMENT ON COLUMN enterprise_join_requests.reason IS 'Additional reason or review comment';
				COMMENT ON COLUMN enterprise_join_requests.reviewer_id IS 'Reviewer account ID';
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE IF EXISTS "public"."enterprise_join_requests";`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`DROP TABLE IF EXISTS "public"."enterprise_invite_links";`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}

