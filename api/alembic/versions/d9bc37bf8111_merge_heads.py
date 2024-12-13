"""merge heads

Revision ID: d9bc37bf8111
Revises: 20241213_add_org_uuid, 20241213_add_projects, remove_api_keys
Create Date: 2024-12-13 01:46:38.800554

"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision: str = 'd9bc37bf8111'
down_revision: Union[str, None] = ('20241213_add_org_uuid', '20241213_add_projects', 'remove_api_keys')
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    pass


def downgrade() -> None:
    pass
