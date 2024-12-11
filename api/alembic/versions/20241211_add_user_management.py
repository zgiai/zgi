"""add user management tables

Revision ID: 20241211_01
Revises: previous_revision
Create Date: 2024-12-11 01:32:49.000000

"""
from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

# revision identifiers, used by Alembic.
revision = '20241211_01'
down_revision = 'previous_revision'  # Update this with your previous revision
branch_labels = None
depends_on = None


def upgrade():
    # Update users table with new fields
    op.add_column('users', sa.Column('is_verified', sa.Boolean(), nullable=False, server_default='false'))
    op.add_column('users', sa.Column('preferences', postgresql.JSONB(astext_type=sa.Text()), nullable=True))
    op.add_column('users', sa.Column('last_login', sa.DateTime(), nullable=True))

    # Create user activity logs table
    op.create_table(
        'user_activity_logs',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('user_id', sa.Integer(), nullable=False),
        sa.Column('action', sa.String(length=255), nullable=False),
        sa.Column('resource_type', sa.String(length=255), nullable=False),
        sa.Column('resource_id', sa.String(length=255), nullable=False),
        sa.Column('ip_address', sa.String(length=45), nullable=True),
        sa.Column('user_agent', sa.String(length=255), nullable=True),
        sa.Column('status', sa.String(length=50), nullable=False),
        sa.Column('details', postgresql.JSONB(astext_type=sa.Text()), nullable=True),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.ForeignKeyConstraint(['user_id'], ['users.id'], ondelete='CASCADE'),
        sa.PrimaryKeyConstraint('id')
    )
    op.create_index('ix_user_activity_logs_user_id', 'user_activity_logs', ['user_id'])
    op.create_index('ix_user_activity_logs_created_at', 'user_activity_logs', ['created_at'])

    # Create API metrics table
    op.create_table(
        'api_metrics',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('endpoint', sa.String(length=255), nullable=False),
        sa.Column('method', sa.String(length=10), nullable=False),
        sa.Column('status_code', sa.Integer(), nullable=False),
        sa.Column('response_time', sa.Float(), nullable=False),
        sa.Column('user_id', sa.Integer(), nullable=True),
        sa.Column('ip_address', sa.String(length=45), nullable=True),
        sa.Column('request_size', sa.Integer(), nullable=True),
        sa.Column('response_size', sa.Integer(), nullable=True),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.ForeignKeyConstraint(['user_id'], ['users.id'], ondelete='SET NULL'),
        sa.PrimaryKeyConstraint('id')
    )
    op.create_index('ix_api_metrics_endpoint', 'api_metrics', ['endpoint'])
    op.create_index('ix_api_metrics_created_at', 'api_metrics', ['created_at'])
    op.create_index('ix_api_metrics_user_id', 'api_metrics', ['user_id'])

    # Create system metrics table
    op.create_table(
        'system_metrics',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('cpu_usage', sa.Float(), nullable=False),
        sa.Column('memory_usage', sa.Float(), nullable=False),
        sa.Column('disk_usage', sa.Float(), nullable=False),
        sa.Column('api_latency', sa.Float(), nullable=False),
        sa.Column('error_rate', sa.Float(), nullable=False),
        sa.Column('created_at', sa.DateTime(), nullable=False),
        sa.PrimaryKeyConstraint('id')
    )
    op.create_index('ix_system_metrics_created_at', 'system_metrics', ['created_at'])


def downgrade():
    # Drop system metrics table
    op.drop_table('system_metrics')

    # Drop API metrics table
    op.drop_table('api_metrics')

    # Drop user activity logs table
    op.drop_table('user_activity_logs')

    # Remove new columns from users table
    op.drop_column('users', 'last_login')
    op.drop_column('users', 'preferences')
    op.drop_column('users', 'is_verified')
