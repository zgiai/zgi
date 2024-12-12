"""remove api_keys table

Revision ID: remove_api_keys
Revises: 90ea5bbbf626
Create Date: 2024-12-12 18:40:00.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'remove_api_keys'
down_revision: Union[str, None] = '90ea5bbbf626'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Drop api_keys table
    op.drop_index(op.f('ix_api_keys_key'), table_name='api_keys')
    op.drop_index(op.f('ix_api_keys_id'), table_name='api_keys')
    op.drop_table('api_keys')


def downgrade() -> None:
    # Re-create api_keys table
    op.create_table('api_keys',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('key', sa.String(length=255), nullable=False),
        sa.Column('name', sa.String(length=255), nullable=False),
        sa.Column('description', sa.String(length=1000), nullable=True),
        sa.Column('user_id', sa.Integer(), nullable=False),
        sa.Column('is_active', sa.Boolean(), nullable=False),
        sa.Column('expires_at', sa.DateTime(), nullable=True),
        sa.Column('last_used_at', sa.DateTime(), nullable=True),
        sa.Column('permissions', sa.JSON(), nullable=True),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('now()'), nullable=False),
        sa.Column('updated_at', sa.DateTime(), server_default=sa.text('now()'), nullable=False),
        sa.ForeignKeyConstraint(['user_id'], ['users.id'], ondelete='CASCADE'),
        sa.PrimaryKeyConstraint('id')
    )
    op.create_index(op.f('ix_api_keys_id'), 'api_keys', ['id'], unique=False)
    op.create_index(op.f('ix_api_keys_key'), 'api_keys', ['key'], unique=True)
