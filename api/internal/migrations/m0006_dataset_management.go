package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0006_dataset_management creates dataset management related tables
func M0006_dataset_management() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100500",
		Migrate: func(tx *gorm.DB) error {
			// Create batch_hit_testing_tasks table
			if err := tx.Exec(`
				CREATE TABLE batch_hit_testing_tasks (
					task_id VARCHAR(36) NOT NULL,
					dataset_id UUID NOT NULL,
					account_id UUID NOT NULL,
					tenant_id UUID NOT NULL,
					status VARCHAR(20) NOT NULL DEFAULT 'pending',
					progress INTEGER NOT NULL DEFAULT 0,
					total INTEGER NOT NULL,
					completed INTEGER NOT NULL DEFAULT 0,
					failed INTEGER NOT NULL DEFAULT 0,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					started_at TIMESTAMPTZ,
					finished_at TIMESTAMPTZ,
					queries JSONB,
					PRIMARY KEY (task_id)
				)
			`).Error; err != nil {
				return err
			}

			// Create child_chunks table
			if err := tx.Exec(`
				CREATE TABLE child_chunks (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					dataset_id UUID NOT NULL,
					document_id UUID NOT NULL,
					segment_id UUID NOT NULL,
					position INTEGER NOT NULL,
					content TEXT NOT NULL,
					word_count INTEGER NOT NULL,
					index_node_id VARCHAR(255),
					index_node_hash VARCHAR(255),
					type VARCHAR(255) NOT NULL DEFAULT 'automatic',
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					indexing_at TIMESTAMPTZ,
					completed_at TIMESTAMPTZ,
					error TEXT,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_collection_bindings table
			if err := tx.Exec(`
				CREATE TABLE dataset_collection_bindings (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					provider_name VARCHAR(255) NOT NULL,
					model_name VARCHAR(255) NOT NULL,
					collection_name VARCHAR(64) NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					type VARCHAR(40) NOT NULL DEFAULT 'dataset',
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_folder_joins table
			if err := tx.Exec(`
				CREATE TABLE dataset_folder_joins (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					dataset_id UUID NOT NULL,
					folder_id UUID NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_folders table
			if err := tx.Exec(`
				CREATE TABLE dataset_folders (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					name VARCHAR(255) NOT NULL,
					description TEXT,
					parent_id UUID,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					icon_type VARCHAR(255),
					icon VARCHAR(255),
					position INTEGER NOT NULL DEFAULT 0,
					permission VARCHAR(255) NOT NULL DEFAULT 'only_me',
					icon_background VARCHAR(255),
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_metadata_bindings table
			if err := tx.Exec(`
				CREATE TABLE dataset_metadata_bindings (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					dataset_id UUID NOT NULL,
					metadata_id UUID NOT NULL,
					document_id UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					created_by UUID NOT NULL,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_metadatas table
			if err := tx.Exec(`
				CREATE TABLE dataset_metadatas (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					dataset_id UUID NOT NULL,
					type VARCHAR(255) NOT NULL,
					name VARCHAR(255) NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					created_by UUID NOT NULL,
					updated_by UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_permissions table
			if err := tx.Exec(`
				CREATE TABLE dataset_permissions (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					dataset_id UUID NOT NULL,
					account_id UUID NOT NULL,
					has_permission BOOLEAN NOT NULL DEFAULT true,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					tenant_id UUID NOT NULL,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_process_rules table
			if err := tx.Exec(`
				CREATE TABLE dataset_process_rules (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					dataset_id UUID NOT NULL,
					mode VARCHAR(255) NOT NULL DEFAULT 'automatic',
					rules TEXT,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_queries table
			if err := tx.Exec(`
				CREATE TABLE dataset_queries (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					dataset_id UUID NOT NULL,
					content TEXT NOT NULL,
					source VARCHAR(255) NOT NULL,
					source_app_id UUID,
					created_by_role VARCHAR NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					results JSONB,
					elapsed_time NUMERIC,
					hit_count INTEGER,
					query_type VARCHAR(50) NOT NULL DEFAULT 'single',
					batch_task_id UUID,
					batch_name VARCHAR(255),
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create dataset_retriever_resources table
			if err := tx.Exec(`
				CREATE TABLE dataset_retriever_resources (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					message_id UUID NOT NULL,
					position INTEGER NOT NULL,
					dataset_id UUID NOT NULL,
					dataset_name TEXT NOT NULL,
					document_id UUID,
					document_name TEXT NOT NULL,
					data_source_type TEXT,
					segment_id UUID,
					score FLOAT8,
					content TEXT NOT NULL,
					hit_count INTEGER,
					word_count INTEGER,
					segment_position INTEGER,
					index_node_hash TEXT,
					retriever_from TEXT NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create datasets table
			if err := tx.Exec(`
				CREATE TABLE datasets (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					name VARCHAR(255) NOT NULL,
					description TEXT,
					provider VARCHAR(255) NOT NULL DEFAULT 'vendor',
					permission VARCHAR(255) NOT NULL DEFAULT 'only_me',
					data_source_type VARCHAR(255),
					indexing_technique VARCHAR(255),
					index_struct TEXT,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					embedding_model VARCHAR(255) DEFAULT 'text-embedding-ada-002',
					embedding_model_provider VARCHAR(255) DEFAULT 'openai',
					collection_binding_id UUID,
					retrieval_model JSONB,
					owner UUID,
					icon TEXT,
					icon_background VARCHAR(255),
					built_in_field_enabled BOOLEAN NOT NULL DEFAULT false,
					icon_type VARCHAR(255),
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create document_segment_questions table
			if err := tx.Exec(`
				CREATE TABLE document_segment_questions (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					dataset_id UUID NOT NULL,
					document_id UUID NOT NULL,
					segment_id UUID NOT NULL,
					question TEXT NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					status VARCHAR(255) NOT NULL DEFAULT 'waiting',
					indexing_at TIMESTAMPTZ,
					completed_at TIMESTAMPTZ,
					error TEXT,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create document_segments table
			if err := tx.Exec(`
				CREATE TABLE document_segments (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					dataset_id UUID NOT NULL,
					document_id UUID NOT NULL,
					position INTEGER NOT NULL,
					content TEXT NOT NULL,
					word_count INTEGER NOT NULL,
					tokens INTEGER NOT NULL,
					keywords JSON,
					index_node_id VARCHAR(255),
					index_node_hash VARCHAR(255),
					hit_count INTEGER NOT NULL,
					enabled BOOLEAN NOT NULL DEFAULT true,
					disabled_at TIMESTAMPTZ,
					disabled_by UUID,
					status VARCHAR(255) NOT NULL DEFAULT 'waiting',
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					indexing_at TIMESTAMPTZ,
					completed_at TIMESTAMPTZ,
					error TEXT,
					stopped_at TIMESTAMPTZ,
					answer TEXT,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create documents table
			if err := tx.Exec(`
				CREATE TABLE documents (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					dataset_id UUID NOT NULL,
					position INTEGER NOT NULL,
					data_source_type VARCHAR(255) NOT NULL,
					data_source_info TEXT,
					dataset_process_rule_id UUID,
					batch VARCHAR(255) NOT NULL,
					name VARCHAR(255) NOT NULL,
					created_from VARCHAR(255) NOT NULL,
					created_by UUID NOT NULL,
					created_api_request_id UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					processing_started_at TIMESTAMPTZ,
					file_id TEXT,
					word_count INTEGER,
					parsing_completed_at TIMESTAMPTZ,
					cleaning_completed_at TIMESTAMPTZ,
					splitting_completed_at TIMESTAMPTZ,
					tokens INTEGER,
					indexing_latency FLOAT8,
					completed_at TIMESTAMPTZ,
					is_paused BOOLEAN DEFAULT false,
					paused_by UUID,
					paused_at TIMESTAMPTZ,
					error TEXT,
					stopped_at TIMESTAMPTZ,
					indexing_status VARCHAR(255) NOT NULL DEFAULT 'waiting',
					enabled BOOLEAN NOT NULL DEFAULT true,
					disabled_at TIMESTAMPTZ,
					disabled_by UUID,
					archived BOOLEAN NOT NULL DEFAULT false,
					archived_reason VARCHAR(255),
					archived_by UUID,
					archived_at TIMESTAMPTZ,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					doc_type VARCHAR(40),
					doc_metadata JSONB,
					doc_form VARCHAR(255) NOT NULL DEFAULT 'text_model',
					doc_language VARCHAR(255),
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create embeddings table
			if err := tx.Exec(`
				CREATE TABLE embeddings (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					hash VARCHAR(64) NOT NULL,
					embedding BYTEA NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					model_name VARCHAR(255) NOT NULL DEFAULT 'text-embedding-ada-002',
					provider_name VARCHAR(255) NOT NULL DEFAULT '',
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes
			indexes := []string{
				`CREATE INDEX batch_hit_testing_tasks_tenant_id_idx ON batch_hit_testing_tasks(tenant_id)`,
				`CREATE INDEX batch_hit_testing_tasks_dataset_id_idx ON batch_hit_testing_tasks(dataset_id)`,
				`CREATE INDEX child_chunks_tenant_id_dataset_id_idx ON child_chunks(tenant_id, dataset_id)`,
				`CREATE INDEX child_chunks_document_id_idx ON child_chunks(document_id)`,
				`CREATE INDEX child_chunks_segment_id_idx ON child_chunks(segment_id)`,
				`CREATE INDEX dataset_folder_joins_dataset_id_idx ON dataset_folder_joins(dataset_id)`,
				`CREATE INDEX dataset_folder_joins_folder_id_idx ON dataset_folder_joins(folder_id)`,
				`CREATE INDEX dataset_folders_tenant_id_idx ON dataset_folders(tenant_id)`,
				`CREATE INDEX dataset_metadata_bindings_dataset_id_idx ON dataset_metadata_bindings(dataset_id)`,
				`CREATE INDEX dataset_metadata_bindings_metadata_id_idx ON dataset_metadata_bindings(metadata_id)`,
				`CREATE INDEX dataset_metadatas_dataset_id_idx ON dataset_metadatas(dataset_id)`,
				`CREATE INDEX dataset_metadatas_tenant_id_idx ON dataset_metadatas(tenant_id)`,
				`CREATE INDEX dataset_permissions_dataset_id_idx ON dataset_permissions(dataset_id)`,
				`CREATE INDEX dataset_permissions_account_id_idx ON dataset_permissions(account_id)`,
				`CREATE INDEX dataset_process_rules_dataset_id_idx ON dataset_process_rules(dataset_id)`,
				`CREATE INDEX dataset_queries_dataset_id_idx ON dataset_queries(dataset_id)`,
				`CREATE INDEX dataset_queries_batch_task_id_idx ON dataset_queries(batch_task_id)`,
				`CREATE INDEX dataset_retriever_resources_message_id_idx ON dataset_retriever_resources(message_id)`,
				`CREATE INDEX dataset_tenant_id_idx ON datasets(tenant_id)`,
				`CREATE INDEX document_segment_questions_dataset_id_idx ON document_segment_questions(dataset_id)`,
				`CREATE INDEX document_segment_questions_document_id_idx ON document_segment_questions(document_id)`,
				`CREATE INDEX document_segment_questions_segment_id_idx ON document_segment_questions(segment_id)`,
				`CREATE INDEX document_segment_questions_status_idx ON document_segment_questions(status)`,
				`CREATE INDEX document_segment_questions_tenant_id_idx ON document_segment_questions(tenant_id)`,
				`CREATE INDEX document_segments_dataset_id_idx ON document_segments(dataset_id)`,
				`CREATE INDEX document_segments_document_id_idx ON document_segments(document_id)`,
				`CREATE INDEX document_segments_tenant_id_idx ON document_segments(tenant_id)`,
				`CREATE INDEX documents_dataset_id_idx ON documents(dataset_id)`,
				`CREATE INDEX documents_tenant_id_idx ON documents(tenant_id)`,
				`CREATE INDEX embeddings_hash_model_provider_idx ON embeddings(hash, model_name, provider_name)`,
			}

			for _, indexSQL := range indexes {
				if err := tx.Exec(indexSQL).Error; err != nil {
					return err
				}
			}

			// Add constraints
			constraints := []string{
				`ALTER TABLE dataset_collection_bindings ADD CONSTRAINT unique_collection_name UNIQUE (collection_name)`,
				`ALTER TABLE embeddings ADD CONSTRAINT embedding_hash_idx UNIQUE (model_name, hash, provider_name)`,
			}

			for _, constraintSQL := range constraints {
				if err := tx.Exec(constraintSQL).Error; err != nil {
					return err
				}
			}

			// Add foreign key constraints
			foreignKeyConstraints := []string{
				// batch_hit_testing_tasks foreign keys
				`ALTER TABLE batch_hit_testing_tasks ADD CONSTRAINT fk_batch_hit_testing_task_dataset FOREIGN KEY (dataset_id) REFERENCES datasets(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
				`ALTER TABLE batch_hit_testing_tasks ADD CONSTRAINT fk_batch_hit_testing_task_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
				
				// dataset_folder_joins foreign keys
				`ALTER TABLE dataset_folder_joins ADD CONSTRAINT fk_dataset_folder_join_dataset FOREIGN KEY (dataset_id) REFERENCES datasets(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
				`ALTER TABLE dataset_folder_joins ADD CONSTRAINT fk_dataset_folder_join_folder FOREIGN KEY (folder_id) REFERENCES dataset_folders(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
				
				// dataset_folders foreign keys
				`ALTER TABLE dataset_folders ADD CONSTRAINT fk_dataset_folder_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
				
				// document_segment_questions foreign keys
				`ALTER TABLE document_segment_questions ADD CONSTRAINT fk_document_segment_question_dataset FOREIGN KEY (dataset_id) REFERENCES datasets(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
				`ALTER TABLE document_segment_questions ADD CONSTRAINT fk_document_segment_question_document FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
				`ALTER TABLE document_segment_questions ADD CONSTRAINT fk_document_segment_question_segment FOREIGN KEY (segment_id) REFERENCES document_segments(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
				`ALTER TABLE document_segment_questions ADD CONSTRAINT fk_document_segment_question_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE NO ACTION`,
			}

			for _, constraintSQL := range foreignKeyConstraints {
				if err := tx.Exec(constraintSQL).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(
				"batch_hit_testing_tasks",
				"child_chunks",
				"dataset_collection_bindings",
				"dataset_folder_joins",
				"dataset_folders",
				"dataset_metadata_bindings",
				"dataset_metadatas",
				"dataset_permissions",
				"dataset_process_rules",
				"dataset_queries",
				"dataset_retriever_resources",
				"datasets",
				"document_segment_questions",
				"document_segments",
				"documents",
				"embeddings",
			)
		},
	}
}