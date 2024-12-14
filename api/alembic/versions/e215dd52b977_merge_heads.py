"""merge heads

Revision ID: e215dd52b977
Revises: 2023121306, 2024_02_13_add_api_key_mappings
Create Date: 2024-12-13 23:56:26.109095

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'e215dd52b977'
down_revision: Union[str, None] = ('2023121306', '2024_02_13_add_api_key_mappings')
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    pass


def downgrade() -> None:
    pass
