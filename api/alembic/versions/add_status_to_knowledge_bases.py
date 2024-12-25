"""add status to knowledge bases

Revision ID: add_status_to_knowledge_bases
Revises: 
Create Date: 2024-12-24 23:48:47.000000

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'add_status_to_knowledge_bases'
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Add status column with default value 1 (ACTIVE)
    op.add_column('knowledge_bases', sa.Column('status', sa.Integer(), nullable=False, server_default='1'))


def downgrade() -> None:
    op.drop_column('knowledge_bases', 'status')
