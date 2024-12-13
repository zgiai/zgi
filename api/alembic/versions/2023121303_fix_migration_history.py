"""fix migration history

Revision ID: 2023121303
Revises: 
Create Date: 2023-12-13 20:27:00.000000

"""
from alembic import op
import sqlalchemy as sa

# revision identifiers, used by Alembic.
revision = '2023121303'
down_revision = None
branch_labels = None
depends_on = None

def upgrade():
    # Create model_providers table if not exists
    op.create_table(
        'model_providers',
        sa.Column('id', sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column('provider_name', sa.String(length=100), nullable=False),
        sa.Column('enabled', sa.Boolean(), nullable=False, default=True),
        sa.Column('api_key', sa.Text(), nullable=True),
        sa.Column('org_id', sa.String(length=100), nullable=True),
        sa.Column('base_url', sa.String(length=255), nullable=True),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.Column('updated_at', sa.DateTime(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(), nullable=True),
        sa.PrimaryKeyConstraint('id')
    )

    # Create model_provider_models table if not exists
    op.create_table(
        'model_provider_models',
        sa.Column('id', sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column('provider_id', sa.BigInteger(), nullable=False),
        sa.Column('model_name', sa.String(length=100), nullable=False),
        sa.Column('model_version', sa.String(length=50), nullable=True),
        sa.Column('description', sa.String(length=255), nullable=True),
        sa.Column('type', sa.String(length=50), nullable=False),
        sa.Column('modalities', sa.JSON(), nullable=True),
        sa.Column('max_context_length', sa.Integer(), nullable=True),
        sa.Column('supports_streaming', sa.Boolean(), nullable=False, default=False),
        sa.Column('supports_function_calling', sa.Boolean(), nullable=False, default=False),
        sa.Column('supports_roles', sa.Boolean(), nullable=False, default=False),
        sa.Column('supports_functions', sa.Boolean(), nullable=False, default=False),
        sa.Column('embedding_dimensions', sa.Integer(), nullable=True),
        sa.Column('default_temperature', sa.Numeric(precision=3, scale=2), nullable=True),
        sa.Column('default_top_p', sa.Numeric(precision=3, scale=2), nullable=True),
        sa.Column('default_max_tokens', sa.Integer(), nullable=True),
        sa.Column('price_per_1k_tokens', sa.Numeric(precision=10, scale=4), nullable=True),
        sa.Column('input_cost_per_1k_tokens', sa.Numeric(precision=10, scale=4), nullable=True),
        sa.Column('output_cost_per_1k_tokens', sa.Numeric(precision=10, scale=4), nullable=True),
        sa.Column('api_call_name', sa.String(length=100), nullable=True),
        sa.Column('latency_ms_estimate', sa.Integer(), nullable=True),
        sa.Column('rate_limit_per_minute', sa.Integer(), nullable=True),
        sa.Column('fine_tuning_available', sa.Boolean(), nullable=False, default=False),
        sa.Column('multi_lang_support', sa.Boolean(), nullable=False, default=False),
        sa.Column('release_date', sa.Date(), nullable=True),
        sa.Column('developer_name', sa.String(length=100), nullable=True),
        sa.Column('developer_website', sa.String(length=255), nullable=True),
        sa.Column('training_data_sources', sa.Text(), nullable=True),
        sa.Column('parameters_count', sa.String(length=50), nullable=True),
        sa.Column('model_architecture', sa.String(length=100), nullable=True),
        sa.Column('documentation_url', sa.String(length=255), nullable=True),
        sa.Column('demo_url', sa.String(length=255), nullable=True),
        sa.Column('tags', sa.JSON(), nullable=True),
        sa.Column('last_updated', sa.DateTime(), nullable=True),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.Column('updated_at', sa.DateTime(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(), nullable=True),
        sa.ForeignKeyConstraint(['provider_id'], ['model_providers.id']),
        sa.PrimaryKeyConstraint('id')
    )

    # Create model_categories table if not exists
    op.create_table(
        'model_categories',
        sa.Column('id', sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column('category_name', sa.String(length=100), nullable=False),
        sa.Column('parent_id', sa.BigInteger(), nullable=True),
        sa.Column('description', sa.String(length=255), nullable=True),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.Column('updated_at', sa.DateTime(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(), nullable=True),
        sa.ForeignKeyConstraint(['parent_id'], ['model_categories.id']),
        sa.PrimaryKeyConstraint('id')
    )

    # Create model_model_categories table if not exists
    op.create_table(
        'model_model_categories',
        sa.Column('id', sa.BigInteger(), autoincrement=True, nullable=False),
        sa.Column('model_id', sa.BigInteger(), nullable=False),
        sa.Column('category_id', sa.BigInteger(), nullable=False),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.Column('deleted_at', sa.DateTime(), nullable=True),
        sa.ForeignKeyConstraint(['category_id'], ['model_categories.id']),
        sa.ForeignKeyConstraint(['model_id'], ['model_provider_models.id']),
        sa.PrimaryKeyConstraint('id')
    )

def downgrade():
    op.drop_table('model_model_categories')
    op.drop_table('model_categories')
    op.drop_table('model_provider_models')
    op.drop_table('model_providers')
