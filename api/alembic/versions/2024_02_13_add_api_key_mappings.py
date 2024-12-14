"""add api key mappings

Revision ID: 2024_02_13_add_api_key_mappings
Revises: 
Create Date: 2024-02-13 00:00:00.000000

"""
from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

# revision identifiers, used by Alembic.
revision = '2024_02_13_add_api_key_mappings'
down_revision = None
branch_labels = None
depends_on = None

def upgrade():
    # Create api_key_mappings table
    op.create_table(
        'api_key_mappings',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('api_key', sa.String(length=255), nullable=False),
        sa.Column('provider_keys', postgresql.JSON(astext_type=sa.Text()), nullable=False),
        sa.Column('created_at', sa.DateTime(), nullable=True),
        sa.Column('updated_at', sa.DateTime(), nullable=True),
        sa.PrimaryKeyConstraint('id')
    )
    op.create_index(op.f('ix_api_key_mappings_api_key'), 'api_key_mappings', ['api_key'], unique=True)
    op.create_index(op.f('ix_api_key_mappings_id'), 'api_key_mappings', ['id'], unique=False)

def downgrade():
    # Drop api_key_mappings table
    op.drop_index(op.f('ix_api_key_mappings_id'), table_name='api_key_mappings')
    op.drop_index(op.f('ix_api_key_mappings_api_key'), table_name='api_key_mappings')
    op.drop_table('api_key_mappings')
