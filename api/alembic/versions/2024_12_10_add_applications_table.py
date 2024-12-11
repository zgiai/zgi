"""add applications table

Revision ID: 2024_12_10_applications
Revises: 7f227e4abe3b
Create Date: 2024-12-10 23:33:55.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import mysql

# revision identifiers, used by Alembic.
revision: str = '2024_12_10_applications'
down_revision: Union[str, None] = '7f227e4abe3b'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # 创建applications表
    op.create_table(
        'applications',
        sa.Column('id', sa.Integer(), autoincrement=True, nullable=False),
        sa.Column('name', sa.String(length=255), nullable=False),
        sa.Column('description', sa.String(length=1000), nullable=True),
        sa.Column('type', sa.String(length=50), nullable=False),
        sa.Column('access_level', sa.String(length=20), nullable=False, server_default='private'),
        sa.Column('team_id', sa.Integer(), nullable=True),
        sa.Column('created_by', sa.Integer(), nullable=False),
        sa.Column('is_active', sa.Boolean(), server_default=sa.text('1'), nullable=False),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=False),
        sa.Column('updated_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP'), nullable=False),
        sa.ForeignKeyConstraint(['created_by'], ['users.id'], ondelete='CASCADE'),
        sa.ForeignKeyConstraint(['team_id'], ['teams.id'], ondelete='SET NULL'),
        sa.PrimaryKeyConstraint('id')
    )
    
    # 创建索引
    op.create_index('ix_applications_name', 'applications', ['name'])
    op.create_index('ix_applications_team_id', 'applications', ['team_id'])
    op.create_index('ix_applications_created_by', 'applications', ['created_by'])


def downgrade() -> None:
    # 删除索引
    op.drop_index('ix_applications_created_by', 'applications')
    op.drop_index('ix_applications_team_id', 'applications')
    op.drop_index('ix_applications_name', 'applications')
    
    # 删除表
    op.drop_table('applications')
