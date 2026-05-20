package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0013_payment_system creates payment and subscription related tables
func M0013_payment_system() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251122000000",
		Migrate: func(tx *gorm.DB) error {
			// 1. subscription_plans (Subscription Plans Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."subscription_plans" (
					"id" varchar(255) NOT NULL,
					"plan_code" varchar(50) NOT NULL,
					"plan_name" varchar(100) NOT NULL,
					"display_order" int4 NOT NULL DEFAULT 0,
					"pricing" jsonb NOT NULL,
					"currency" varchar(10) NOT NULL DEFAULT 'CNY',
					"quota_config" jsonb NOT NULL,
					"features" jsonb,
					"external_config" jsonb,
					"description" text,
					"is_active" bool NOT NULL DEFAULT true,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE UNIQUE INDEX idx_plan_code ON subscription_plans(plan_code);
				CREATE INDEX idx_plan_is_active ON subscription_plans(is_active);
				
				COMMENT ON TABLE subscription_plans IS 'Subscription plans table';
				COMMENT ON COLUMN subscription_plans.plan_code IS 'Plan code: free, team, business, enterprise, personal_pro';
				COMMENT ON COLUMN subscription_plans.pricing IS 'Pricing configuration (supports multiple billing cycles)';
				COMMENT ON COLUMN subscription_plans.quota_config IS 'Quota configuration JSON';
			`).Error; err != nil {
				return err
			}

			// 2. group_subscriptions (User Subscriptions Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."group_subscriptions" (
					"id" varchar(255) NOT NULL,
					"account_id" uuid NOT NULL,
					"group_id" uuid NOT NULL,
					"plan_id" varchar(255) NOT NULL,
					"plan_code" varchar(50) NOT NULL,
					"billing_cycle" varchar(20) NOT NULL,
					"status" varchar(20) NOT NULL DEFAULT 'active',
					"current_period_start" timestamptz NOT NULL,
					"current_period_end" timestamptz NOT NULL,
					"trial_end" timestamptz,
					"canceled_at" timestamptz,
					"cancel_reason" text,
					"auto_renew" bool NOT NULL DEFAULT true,
					"quota_snapshot" jsonb NOT NULL,
					
					-- External platform binding (for recurring subscription)
					"external_binding" jsonb,
					"next_billing_date" timestamptz,
					"last_payment_at" timestamptz,
					"failed_payment_count" int4 NOT NULL DEFAULT 0,
					
					-- Billing day anchor for monthly reset (1-31)
					"billing_day_of_month" int4 NOT NULL DEFAULT 1,
					
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE INDEX idx_subscription_account ON group_subscriptions(account_id);
				CREATE INDEX idx_subscription_plan ON group_subscriptions(plan_id);
				CREATE INDEX idx_subscription_status ON group_subscriptions(status);
				CREATE INDEX idx_subscription_period_end ON group_subscriptions(current_period_end);
				CREATE INDEX idx_subscription_group ON group_subscriptions(group_id);
				
				-- Index for external subscription ID lookup (provider + subscription_id)
				CREATE INDEX idx_external_subscription_id ON group_subscriptions(
					(external_binding->>'provider'), 
					(external_binding->>'subscription_id')
				) WHERE external_binding IS NOT NULL;
				
				-- Index for merchant-initiated deduction scheduling
				CREATE INDEX idx_next_billing ON group_subscriptions(next_billing_date) 
					WHERE status = 'active' AND next_billing_date IS NOT NULL;
				
				COMMENT ON TABLE group_subscriptions IS 'User subscriptions table';
				COMMENT ON COLUMN group_subscriptions.group_id IS 'Group ID, subscription is for a specific group under the user';
				COMMENT ON COLUMN group_subscriptions.billing_cycle IS 'Billing cycle: monthly, yearly';
				COMMENT ON COLUMN group_subscriptions.status IS 'Status: active, past_due, canceled, expired, trial';
				COMMENT ON COLUMN group_subscriptions.external_binding IS 'External platform binding info (JSONB): provider, subscription_id, customer_id, etc.';
				COMMENT ON COLUMN group_subscriptions.next_billing_date IS 'Next billing date for merchant-initiated deduction (alipay, wechat)';
				COMMENT ON COLUMN group_subscriptions.last_payment_at IS 'Last successful payment time';
				COMMENT ON COLUMN group_subscriptions.failed_payment_count IS 'Consecutive payment failure count for retry strategy';
				COMMENT ON COLUMN group_subscriptions.billing_day_of_month IS 'Day of month (1-31) for billing/reset anchor, default from subscription start date, can be customized';
			`).Error; err != nil {
				return err
			}

			// 3. subscription_history (Subscription History Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."subscription_history" (
					"id" varchar(255) NOT NULL,
					"subscription_id" varchar(255) NOT NULL,
					"account_id" uuid NOT NULL,
					"event_type" varchar(30) NOT NULL,
					"from_plan_code" varchar(50),
					"to_plan_code" varchar(50),
					"from_status" varchar(20),
					"to_status" varchar(20),
					"metadata" jsonb,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE INDEX idx_history_subscription ON subscription_history(subscription_id);
				CREATE INDEX idx_history_account ON subscription_history(account_id);
				CREATE INDEX idx_history_event_type ON subscription_history(event_type);
				CREATE INDEX idx_history_created_at ON subscription_history(created_at);
				
				COMMENT ON TABLE subscription_history IS 'Subscription history table';
				COMMENT ON COLUMN subscription_history.event_type IS 'Event type: created, renewed, upgraded, downgraded, canceled, expired';
			`).Error; err != nil {
				return err
			}

			// 4. group_ai_credit_accounts (Group AI Credit Accounts Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."group_ai_credit_accounts" (
					"id" varchar(255) NOT NULL,
					"account_id" uuid NOT NULL,
					"group_id" uuid NOT NULL,
					"subscription_credits" bigint NOT NULL DEFAULT 0,
					"purchased_credits" bigint NOT NULL DEFAULT 0,
					"total_earned" bigint NOT NULL DEFAULT 0,
					"total_spent" bigint NOT NULL DEFAULT 0,
					"last_reset_at" timestamptz,
					"next_reset_at" timestamptz,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE UNIQUE INDEX idx_account_group_credit ON group_ai_credit_accounts(account_id, group_id);
				CREATE INDEX idx_credit_account ON group_ai_credit_accounts(account_id);
				CREATE INDEX idx_credit_group ON group_ai_credit_accounts(group_id);
				
				COMMENT ON TABLE group_ai_credit_accounts IS 'Group AI credit accounts table';
				COMMENT ON COLUMN group_ai_credit_accounts.account_id IS 'Account ID (redundant for query convenience)';
				COMMENT ON COLUMN group_ai_credit_accounts.group_id IS 'Group ID, credit account is for a specific group';
				COMMENT ON COLUMN group_ai_credit_accounts.subscription_credits IS 'Subscription credits balance (reset monthly)';
				COMMENT ON COLUMN group_ai_credit_accounts.purchased_credits IS 'Purchased credits balance (no reset)';
				COMMENT ON COLUMN group_ai_credit_accounts.total_earned IS 'Total credits earned (cumulative)';
				COMMENT ON COLUMN group_ai_credit_accounts.total_spent IS 'Total credits spent (cumulative)';
				COMMENT ON COLUMN group_ai_credit_accounts.last_reset_at IS 'Last reset time for subscription credits';
				COMMENT ON COLUMN group_ai_credit_accounts.next_reset_at IS 'Next reset time for subscription credits';
			`).Error; err != nil {
				return err
			}

			// 6. orders (Orders Table)
			// Note: ai_credit_transactions table is deprecated, use unified transactions table instead
			if err := tx.Exec(`
				CREATE TABLE "public"."orders" (
					"id" varchar(255) NOT NULL,
					"order_no" varchar(50) NOT NULL,
					"account_id" uuid NOT NULL,
					"group_id" uuid NOT NULL,
					"order_type" varchar(30) NOT NULL,
					"product_code" varchar(50) NOT NULL,
					"product_type" varchar(30) NOT NULL,
					"product_snapshot" jsonb NOT NULL,
					"original_amount" decimal(10,2) NOT NULL,
					"discount_amount" decimal(10,2) NOT NULL DEFAULT 0,
					"final_amount" decimal(10,2) NOT NULL,
					"currency" varchar(10) NOT NULL DEFAULT 'CNY',
					"discount_details" jsonb,
					"status" varchar(30) NOT NULL DEFAULT 'pending',
					"paid_at" timestamptz,
					"completed_at" timestamptz,
					"failed_at" timestamptz,
					"failure_reason" text,
					"canceled_at" timestamptz,
					"cancel_reason" text,
					"refunded_at" timestamptz,
					"refund_reason" text,
					"subscription_id" varchar(255),
					"client_ip" varchar(45),
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE UNIQUE INDEX idx_order_no ON orders(order_no);
				CREATE INDEX idx_order_account ON orders(account_id);
				CREATE INDEX idx_order_group ON orders(group_id);
				CREATE INDEX idx_order_status ON orders(status);
				CREATE INDEX idx_order_type ON orders(order_type);
				CREATE INDEX idx_order_subscription ON orders(subscription_id);
				CREATE INDEX idx_order_created_at ON orders(created_at);
				
				COMMENT ON TABLE orders IS 'Orders table';
				COMMENT ON COLUMN orders.group_id IS 'Group ID, order is for a specific group under the user';
				COMMENT ON COLUMN orders.order_type IS 'Order type: subscription_new, subscription_renew, subscription_upgrade, credit_purchase';
				COMMENT ON COLUMN orders.status IS 'Order status: pending, paid, completed, failed, canceled, refunded';
			`).Error; err != nil {
				return err
			}

			// 8. payment_transactions (Payment Transactions Table)
			// Note: payment_methods table removed - configuration is now managed via config files
			if err := tx.Exec(`
				CREATE TABLE "public"."payment_transactions" (
					"id" varchar(255) NOT NULL,
					"transaction_no" varchar(100) NOT NULL,
					"order_id" varchar(255) NOT NULL,
					"account_id" uuid NOT NULL,
					"payment_method" varchar(30) NOT NULL,
					"amount" decimal(10,2) NOT NULL,
					"currency" varchar(10) NOT NULL DEFAULT 'CNY',
					"status" varchar(20) NOT NULL DEFAULT 'pending',
					"provider_transaction_id" varchar(255),
					"provider_response" jsonb,
					"paid_at" timestamptz,
					"failed_at" timestamptz,
					"failure_reason" text,
					"client_ip" varchar(45),
					"user_agent" text,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE UNIQUE INDEX idx_payment_transaction_no ON payment_transactions(transaction_no);
				CREATE INDEX idx_payment_transaction_order ON payment_transactions(order_id);
				CREATE INDEX idx_payment_transaction_account ON payment_transactions(account_id);
				CREATE INDEX idx_payment_transaction_status ON payment_transactions(status);
				CREATE INDEX idx_payment_transaction_method ON payment_transactions(payment_method);
				CREATE INDEX idx_payment_transaction_created_at ON payment_transactions(created_at);
				
				COMMENT ON TABLE payment_transactions IS 'Payment transactions table';
				COMMENT ON COLUMN payment_transactions.status IS 'Status: pending, processing, success, failed, canceled';
			`).Error; err != nil {
				return err
			}

			// 9. payment_callbacks (Payment Callbacks Log Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."payment_callbacks" (
					"id" varchar(255) NOT NULL,
					"transaction_id" varchar(255) NOT NULL,
					"payment_method" varchar(30) NOT NULL,
					"callback_type" varchar(20) NOT NULL,
					"request_headers" jsonb,
					"request_body" text,
					"response_status" int4,
					"response_body" text,
					"is_verified" bool NOT NULL DEFAULT false,
					"verification_error" text,
					"processed" bool NOT NULL DEFAULT false,
					"processed_at" timestamptz,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE INDEX idx_callback_transaction ON payment_callbacks(transaction_id);
				CREATE INDEX idx_callback_method ON payment_callbacks(payment_method);
				CREATE INDEX idx_callback_processed ON payment_callbacks(processed);
				CREATE INDEX idx_callback_created_at ON payment_callbacks(created_at);
				
				COMMENT ON TABLE payment_callbacks IS 'Payment callbacks log table';
				COMMENT ON COLUMN payment_callbacks.callback_type IS 'Callback type: notify, return';
			`).Error; err != nil {
				return err
			}

			// 10. refund_records (Refund Records Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."refund_records" (
					"id" varchar(255) NOT NULL,
					"refund_no" varchar(100) NOT NULL,
					"order_id" varchar(255) NOT NULL,
					"transaction_id" varchar(255) NOT NULL,
					"account_id" uuid NOT NULL,
					"refund_amount" decimal(10,2) NOT NULL,
					"currency" varchar(10) NOT NULL DEFAULT 'CNY',
					"refund_fee" decimal(10,2) NOT NULL DEFAULT 0,
					"original_transaction_amount" decimal(10,2) NOT NULL,
					"refund_reason" text NOT NULL,
					"status" varchar(20) NOT NULL DEFAULT 'pending',
					"provider_refund_id" varchar(255),
					"provider_response" jsonb,
					"processing_at" timestamptz,
					"success_at" timestamptz,
					"failed_at" timestamptz,
					"failure_reason" text,
					"operator_id" uuid,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE UNIQUE INDEX idx_refund_no ON refund_records(refund_no);
				CREATE INDEX idx_refund_order ON refund_records(order_id);
				CREATE INDEX idx_refund_transaction ON refund_records(transaction_id);
				CREATE INDEX idx_refund_account ON refund_records(account_id);
				CREATE INDEX idx_refund_status ON refund_records(status);
				CREATE INDEX idx_refund_created_at ON refund_records(created_at);
				
				COMMENT ON TABLE refund_records IS 'Refund records table';
				COMMENT ON COLUMN refund_records.status IS 'Status: pending, processing, success, failed';
			`).Error; err != nil {
				return err
			}

			// 11. group_wallets (Group Wallets Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."group_wallets" (
					"id" varchar(255) NOT NULL,
					"account_id" uuid NOT NULL,
					"group_id" uuid NOT NULL,
					"currency" varchar(10) NOT NULL DEFAULT 'CNY',
					"balance" decimal(12,2) NOT NULL DEFAULT 0,
					"frozen_balance" decimal(12,2) NOT NULL DEFAULT 0,
					"total_recharged" decimal(12,2) NOT NULL DEFAULT 0,
					"total_consumed" decimal(12,2) NOT NULL DEFAULT 0,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE UNIQUE INDEX idx_wallet_account_group_currency ON group_wallets(account_id, group_id, currency);
				CREATE INDEX idx_wallet_account ON group_wallets(account_id);
				CREATE INDEX idx_wallet_group ON group_wallets(group_id);
				
				COMMENT ON TABLE group_wallets IS 'Group wallets table for cash balance management';
				COMMENT ON COLUMN group_wallets.account_id IS 'Account ID (redundant for query convenience)';
				COMMENT ON COLUMN group_wallets.currency IS 'Currency: CNY, USD';
				COMMENT ON COLUMN group_wallets.balance IS 'Available balance';
				COMMENT ON COLUMN group_wallets.frozen_balance IS 'Frozen balance (pending review, refund processing, etc.)';
				COMMENT ON COLUMN group_wallets.total_recharged IS 'Total recharged amount (cumulative)';
				COMMENT ON COLUMN group_wallets.total_consumed IS 'Total consumed amount (cumulative)';
			`).Error; err != nil {
				return err
			}

			// 12. transactions (Unified Transaction Records Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."transactions" (
					"id" varchar(255) NOT NULL,
					"batch_id" varchar(255) NOT NULL,
					"group_id" uuid NOT NULL,
					"tenant_id" uuid,
					"currency_type" varchar(20) NOT NULL,
					"transaction_type" varchar(30) NOT NULL,
					"amount" decimal(16,4) NOT NULL,
					"balance_before" decimal(16,4) NOT NULL,
					"balance_after" decimal(16,4) NOT NULL,
					"currency" varchar(10),
					"reference_type" varchar(50),
					"reference_id" varchar(255),
					"description" varchar(500),
					"transaction_detail" jsonb,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE INDEX idx_tx_group ON transactions(group_id);
				CREATE INDEX idx_tx_group_currency ON transactions(group_id, currency_type);
				CREATE INDEX idx_tx_type ON transactions(transaction_type);
				CREATE INDEX idx_tx_reference ON transactions(reference_type, reference_id);
				CREATE INDEX idx_tx_batch ON transactions(batch_id);
				CREATE INDEX idx_tx_tenant ON transactions(tenant_id);
				CREATE INDEX idx_tx_created_at ON transactions(created_at);
				
				COMMENT ON TABLE transactions IS 'Unified transaction records table for cash and credits';
				COMMENT ON COLUMN transactions.batch_id IS 'Batch ID, multiple records from same business operation share this ID';
				COMMENT ON COLUMN transactions.currency_type IS 'Currency type: cash / subscription_credits / purchased_credits';
				COMMENT ON COLUMN transactions.transaction_type IS 'Transaction type: recharge, subscription_payment, ai_usage, etc.';
				COMMENT ON COLUMN transactions.amount IS 'Amount change (positive for income, negative for expense)';
				COMMENT ON COLUMN transactions.reference_type IS 'Reference type: order / subscription / refund';
			`).Error; err != nil {
				return err
			}

			// 13. group_quotas (Group Quotas Table)
			if err := tx.Exec(`
				CREATE TABLE "public"."group_quotas" (
					"id" varchar(255) NOT NULL,
					"group_id" uuid NOT NULL,
					"workflow_executions" int4 NOT NULL DEFAULT 0,
					"last_reset_at" timestamptz,
					"next_reset_at" timestamptz,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);
				
				CREATE UNIQUE INDEX idx_quota_group ON group_quotas(group_id);
				CREATE INDEX idx_quota_next_reset ON group_quotas(next_reset_at);
				
				COMMENT ON TABLE group_quotas IS 'Group quotas table for periodic quota tracking';
				COMMENT ON COLUMN group_quotas.group_id IS 'Group ID (one record per group)';
				COMMENT ON COLUMN group_quotas.workflow_executions IS 'Remaining workflow execution quota (reset monthly)';
				COMMENT ON COLUMN group_quotas.last_reset_at IS 'Last reset time';
				COMMENT ON COLUMN group_quotas.next_reset_at IS 'Next reset time';
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop tables in reverse order
			tables := []string{
				"group_quotas",
				"transactions",
				"group_wallets",
				"refund_records",
				"payment_callbacks",
				"payment_transactions",
				"orders",
				"group_ai_credit_accounts",
				"subscription_history",
				"group_subscriptions",
				"subscription_plans",
			}

			for _, table := range tables {
				if err := tx.Exec("DROP TABLE IF EXISTS " + table + " CASCADE").Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
