package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0028_bank_transfer creates the bank transfer requests table
func M0028_bank_transfer() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251209000000",
		Migrate: func(tx *gorm.DB) error {
			// Create bank_transfer_requests table
			if err := tx.Exec(`
				CREATE TABLE "public"."bank_transfer_requests" (
					"id" varchar(255) NOT NULL,
					"request_no" varchar(50) NOT NULL,
					"account_id" uuid NOT NULL,
					"group_id" uuid NOT NULL,
					"amount" decimal(12,2) NOT NULL,
					"currency" varchar(10) NOT NULL DEFAULT 'CNY',
					"voucher_key" varchar(255) NOT NULL,
					"remark" text,
					"status" varchar(30) NOT NULL DEFAULT 'pending',
					"reviewed_by" uuid,
					"reviewed_at" timestamptz,
					"reject_reason" text,
					"completed_at" timestamptz,
					"canceled_at" timestamptz,
					"cancel_reason" text,
					"client_ip" varchar(45),
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id"),
					CONSTRAINT chk_btr_amount CHECK (amount > 0)
				);

				-- Indexes
				CREATE UNIQUE INDEX idx_btr_request_no ON bank_transfer_requests(request_no);
				CREATE INDEX idx_btr_account ON bank_transfer_requests(account_id);
				CREATE INDEX idx_btr_group ON bank_transfer_requests(group_id);
				CREATE INDEX idx_btr_status ON bank_transfer_requests(status);
				CREATE INDEX idx_btr_created_at ON bank_transfer_requests(created_at);
				CREATE INDEX idx_btr_reviewed_by ON bank_transfer_requests(reviewed_by);

				-- Comments
				COMMENT ON TABLE bank_transfer_requests IS 'Bank transfer recharge requests table';
				COMMENT ON COLUMN bank_transfer_requests.request_no IS 'User-visible request number';
				COMMENT ON COLUMN bank_transfer_requests.voucher_key IS 'Voucher storage key (object storage path)';
				COMMENT ON COLUMN bank_transfer_requests.status IS 'Request status: pending/approved/rejected/canceled';
				COMMENT ON COLUMN bank_transfer_requests.reviewed_by IS 'Reviewer account ID';
				COMMENT ON COLUMN bank_transfer_requests.reviewed_at IS 'Review timestamp';
				COMMENT ON COLUMN bank_transfer_requests.completed_at IS 'Completion timestamp (when balance is credited)';
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop table
			if err := tx.Exec(`
				DROP TABLE IF EXISTS "public"."bank_transfer_requests";
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
