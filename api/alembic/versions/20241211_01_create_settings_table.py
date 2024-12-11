"""create settings table

Revision ID: 20241211_01_settings
Revises: 
Create Date: 2024-12-11 21:07:32.000000

"""
from alembic import op
import sqlalchemy as sa


# revision identifiers, used by Alembic.
revision = '20241211_01_settings'
down_revision = None
branch_labels = None
depends_on = None


def upgrade():
    # Create settings table
    op.create_table(
        'settings',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('key', sa.String(length=255), nullable=False),
        sa.Column('value', sa.String(length=255), nullable=False),
        sa.Column('created_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=False),
        sa.Column('updated_at', sa.DateTime(), server_default=sa.text('CURRENT_TIMESTAMP'), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('key')
    )
    
    # Insert initial settings
    op.execute("INSERT INTO settings (key, value) VALUES ('is_initialized', 'false')")


def downgrade():
    op.drop_table('settings')
