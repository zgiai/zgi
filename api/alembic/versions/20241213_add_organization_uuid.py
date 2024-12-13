"""add organization uuid

Revision ID: 20241213_add_org_uuid
Revises: 
Create Date: 2024-12-13 01:15:24.000000

"""
from alembic import op
import sqlalchemy as sa
import uuid

# revision identifiers, used by Alembic.
revision = '20241213_add_org_uuid'
down_revision = None
branch_labels = None
depends_on = None

def generate_uuid():
    return str(uuid.uuid4())

def upgrade() -> None:
    # Add UUID column
    op.add_column('organizations', 
        sa.Column('uuid', sa.String(36), nullable=True)
    )
    
    # Generate UUIDs for existing records
    connection = op.get_bind()
    organizations = connection.execute(sa.text('SELECT id FROM organizations')).fetchall()
    for org in organizations:
        connection.execute(
            sa.text('UPDATE organizations SET uuid = :uuid WHERE id = :id'),
            {'uuid': generate_uuid(), 'id': org[0]}
        )
    
    # Make UUID column not nullable and unique
    op.alter_column('organizations', 'uuid',
        existing_type=sa.String(36),
        nullable=False
    )
    op.create_unique_constraint('uq_organizations_uuid', 'organizations', ['uuid'])
    op.create_index('ix_organizations_uuid', 'organizations', ['uuid'])

def downgrade() -> None:
    op.drop_constraint('uq_organizations_uuid', 'organizations', type_='unique')
    op.drop_index('ix_organizations_uuid', 'organizations')
    op.drop_column('organizations', 'uuid')
