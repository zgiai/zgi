"""update chat files

Revision ID: 2023121306
Revises: 2023121304
Create Date: 2023-12-13 20:31:00.000000

"""
from alembic import op
import sqlalchemy as sa

# revision identifiers, used by Alembic.
revision = '2023121306'
down_revision = '2023121304'
branch_labels = None
depends_on = None

def upgrade():
    # Rename file_type to content_type
    op.alter_column('chat_files', 'file_type',
                    new_column_name='content_type',
                    existing_type=sa.String(length=50),
                    nullable=False)

    # Add deleted_at column
    op.add_column('chat_files', sa.Column('deleted_at', sa.DateTime(), nullable=True))

    # Drop updated_at column
    op.drop_column('chat_files', 'updated_at')

    # Update file_path column
    op.alter_column('chat_files', 'file_path',
                    existing_type=sa.String(length=1000),
                    type_=sa.Text(),
                    nullable=False)

    # Update file_size column to be not nullable
    op.alter_column('chat_files', 'file_size',
                    existing_type=sa.Integer(),
                    nullable=False)

def downgrade():
    # Revert file_size to be nullable
    op.alter_column('chat_files', 'file_size',
                    existing_type=sa.Integer(),
                    nullable=True)

    # Revert file_path column
    op.alter_column('chat_files', 'file_path',
                    existing_type=sa.Text(),
                    type_=sa.String(length=1000),
                    nullable=True)

    # Add back updated_at column
    op.add_column('chat_files', sa.Column('updated_at', sa.DateTime(), nullable=False,
                                         server_default=sa.text('CURRENT_TIMESTAMP')))

    # Drop deleted_at column
    op.drop_column('chat_files', 'deleted_at')

    # Rename content_type back to file_type
    op.alter_column('chat_files', 'content_type',
                    new_column_name='file_type',
                    existing_type=sa.String(length=50),
                    nullable=True)
