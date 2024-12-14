# coding: utf-8
from sqlalchemy import BigInteger, Column, DECIMAL, Date, DateTime, Float, ForeignKey, Integer, JSON, SmallInteger, String, Table, Text, text
from sqlalchemy.orm import relationship
from sqlalchemy.dialects.mysql import ENUM, TINYINT
from sqlalchemy.ext.declarative import declarative_base

Base = declarative_base()
metadata = Base.metadata


class ApiKeyMapping(Base):
    __tablename__ = 'api_key_mappings'

    id = Column(Integer, primary_key=True, index=True)
    api_key = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False, unique=True)
    provider_keys = Column(JSON, nullable=False)
    created_at = Column(DateTime)
    updated_at = Column(DateTime)


class ApiLog(Base):
    __tablename__ = 'api_logs'

    id = Column(Integer, primary_key=True, index=True)
    request_id = Column(String(36, 'utf8mb4_unicode_ci'), nullable=False, index=True)
    timestamp = Column(DateTime, nullable=False, index=True)
    method = Column(String(10, 'utf8mb4_unicode_ci'), nullable=False)
    path = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    client_ip = Column(String(45, 'utf8mb4_unicode_ci'), nullable=False)
    request_data = Column(Text(collation='utf8mb4_unicode_ci'))
    response_data = Column(Text(collation='utf8mb4_unicode_ci'))
    status_code = Column(Integer)
    api_key_id = Column(String(255, 'utf8mb4_unicode_ci'))
    error_message = Column(Text(collation='utf8mb4_unicode_ci'))
    duration_ms = Column(Float)


class ChatSession(Base):
    __tablename__ = 'chat_sessions'

    id = Column(BigInteger, primary_key=True, index=True)
    user_id = Column(BigInteger, nullable=False)
    conversation_id = Column(String(100, 'utf8mb4_unicode_ci'), nullable=False)
    request_id = Column(String(100, 'utf8mb4_unicode_ci'))
    question = Column(Text(collation='utf8mb4_unicode_ci'))
    answer = Column(Text(collation='utf8mb4_unicode_ci'))
    model = Column(String(100, 'utf8mb4_unicode_ci'), nullable=False)
    prompt_tokens = Column(Integer, nullable=False, server_default=text("'0'"))
    completion_tokens = Column(Integer, nullable=False, server_default=text("'0'"))
    cost = Column(DECIMAL(11, 7), nullable=False, server_default=text("'0.0000000'"))
    api_key = Column(String(200, 'utf8mb4_unicode_ci'))
    interaction_type = Column(SmallInteger, nullable=False, server_default=text("'1'"))
    app_id = Column(Integer)
    source = Column(SmallInteger)
    ip_address = Column(String(45, 'utf8mb4_unicode_ci'), nullable=False, server_default=text("''"))
    is_violation = Column(SmallInteger, server_default=text("'0'"))
    status = Column(SmallInteger, nullable=False, server_default=text("'1'"))
    parameters = Column(JSON)
    openai_response_id = Column(String(100, 'utf8mb4_unicode_ci'))
    openai_system_fingerprint = Column(String(100, 'utf8mb4_unicode_ci'))
    openai_created_at = Column(Integer)
    raw_request = Column(JSON)
    raw_response_chunks = Column(JSON)
    finish_reason = Column(String(50, 'utf8mb4_unicode_ci'))
    created_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"))


class ModelCategory(Base):
    __tablename__ = 'model_categories'

    id = Column(BigInteger, primary_key=True, index=True)
    category_name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False, comment='Category name')
    parent_id = Column(ForeignKey('model_categories.id'), index=True)
    description = Column(String(1000, 'utf8mb4_unicode_ci'), comment='Category description')
    created_at = Column(DateTime, nullable=False)
    updated_at = Column(DateTime, nullable=False)
    deleted_at = Column(DateTime)

    parent = relationship('ModelCategory', remote_side=[id])
    models = relationship('ModelProviderModel', secondary='category_model_association')


class ModelProvider(Base):
    __tablename__ = 'model_providers'

    id = Column(BigInteger, primary_key=True, index=True)
    provider_name = Column(String(100, 'utf8mb4_unicode_ci'), nullable=False)
    enabled = Column(TINYINT(1), comment='Whether the provider is enabled')
    api_key = Column(Text(collation='utf8mb4_unicode_ci'))
    org_id = Column(String(100, 'utf8mb4_unicode_ci'))
    base_url = Column(String(255, 'utf8mb4_unicode_ci'))
    created_at = Column(DateTime, nullable=False)
    updated_at = Column(DateTime, nullable=False)
    deleted_at = Column(DateTime)


class OrganizationsNew(Base):
    __tablename__ = 'organizations_new'

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    description = Column(String(1000, 'utf8mb4_unicode_ci'))
    created_by = Column(Integer)
    is_active = Column(TINYINT(1), nullable=False, server_default=text("'1'"))
    created_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"))


class ResourceUsage(Base):
    __tablename__ = 'resource_usage'

    id = Column(Integer, primary_key=True, index=True)
    application_id = Column(Integer, nullable=False, index=True)
    resource_type = Column(String(50, 'utf8mb4_unicode_ci'), nullable=False)
    quantity = Column(Float, nullable=False)
    endpoint = Column(String(255, 'utf8mb4_unicode_ci'))
    model = Column(String(100, 'utf8mb4_unicode_ci'))
    timestamp = Column(DateTime, nullable=False, index=True, server_default=text("CURRENT_TIMESTAMP"))


class UsageLog(Base):
    __tablename__ = 'usage_logs'

    id = Column(Integer, primary_key=True, index=True)
    api_key = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False, index=True)
    model = Column(String(100, 'utf8mb4_unicode_ci'), nullable=False)
    prompt_tokens = Column(Integer)
    completion_tokens = Column(Integer)
    total_tokens = Column(Integer)
    cost = Column(Float)
    created_at = Column(DateTime)


class UserQuota(Base):
    __tablename__ = 'user_quotas'

    id = Column(Integer, primary_key=True, index=True)
    api_key = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False, unique=True)
    total_tokens = Column(Integer)
    used_tokens = Column(Integer)
    reset_date = Column(DateTime)
    created_at = Column(DateTime)
    updated_at = Column(DateTime)


class User(Base):
    __tablename__ = 'users'

    id = Column(Integer, primary_key=True, index=True)
    email = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False, unique=True)
    username = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False, unique=True)
    full_name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    hashed_password = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    is_active = Column(TINYINT(1), nullable=False)
    is_superuser = Column(TINYINT(1), nullable=False)
    is_verified = Column(TINYINT(1), nullable=False)
    is_admin = Column(TINYINT(1), nullable=False)
    preferences = Column(JSON)
    created_at = Column(DateTime, nullable=False)
    updated_at = Column(DateTime, nullable=False)


class Application(Base):
    __tablename__ = 'applications'

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    description = Column(String(1000, 'utf8mb4_unicode_ci'))
    owner_id = Column(ForeignKey('users.id', ondelete='CASCADE'), ForeignKey('users.id', ondelete='CASCADE'), ForeignKey('users.id', ondelete='CASCADE'), ForeignKey('users.id', ondelete='CASCADE'), nullable=False, index=True)
    is_active = Column(TINYINT(1))
    created_at = Column(DateTime, nullable=False)
    updated_at = Column(DateTime, nullable=False)
    max_tokens = Column(Integer)
    max_requests_per_day = Column(Integer)

    owner = relationship('User', primaryjoin='Application.owner_id == User.id')
    owner1 = relationship('User', primaryjoin='Application.owner_id == User.id')
    owner2 = relationship('User', primaryjoin='Application.owner_id == User.id')
    owner3 = relationship('User', primaryjoin='Application.owner_id == User.id')


class ChatFile(Base):
    __tablename__ = 'chat_files'

    id = Column(Integer, primary_key=True, index=True)
    session_id = Column(ForeignKey('chat_sessions.id', ondelete='CASCADE'), ForeignKey('chat_sessions.id', ondelete='CASCADE'), ForeignKey('chat_sessions.id', ondelete='CASCADE'), nullable=False, index=True)
    filename = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    content_type = Column(String(100, 'utf8mb4_unicode_ci'), nullable=False)
    file_size = Column(Integer, nullable=False)
    file_path = Column(Text(collation='utf8mb4_unicode_ci'), nullable=False)
    created_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))
    deleted_at = Column(DateTime)

    session = relationship('ChatSession', primaryjoin='ChatFile.session_id == ChatSession.id')
    session1 = relationship('ChatSession', primaryjoin='ChatFile.session_id == ChatSession.id')
    session2 = relationship('ChatSession', primaryjoin='ChatFile.session_id == ChatSession.id')


class KnowledgeBase(Base):
    __tablename__ = 'knowledge_bases'

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    description = Column(Text(collation='utf8mb4_unicode_ci'))
    visibility = Column(ENUM('PUBLIC', 'PRIVATE'))
    owner_id = Column(ForeignKey('users.id'), nullable=False, index=True)
    created_at = Column(DateTime)
    updated_at = Column(DateTime)

    owner = relationship('User')


class ModelProviderModel(Base):
    __tablename__ = 'model_provider_models'

    id = Column(BigInteger, primary_key=True, index=True)
    provider_id = Column(ForeignKey('model_providers.id'), nullable=False, index=True)
    model_name = Column(String(100, 'utf8mb4_unicode_ci'), nullable=False)
    model_version = Column(String(50, 'utf8mb4_unicode_ci'))
    description = Column(String(255, 'utf8mb4_unicode_ci'))
    type = Column(String(50, 'utf8mb4_unicode_ci'), nullable=False)
    modalities = Column(JSON)
    max_context_length = Column(Integer)
    supports_streaming = Column(TINYINT(1), nullable=False)
    supports_function_calling = Column(TINYINT(1), nullable=False)
    supports_roles = Column(TINYINT(1), nullable=False)
    supports_functions = Column(TINYINT(1), nullable=False)
    embedding_dimensions = Column(Integer)
    default_temperature = Column(DECIMAL(3, 2))
    default_top_p = Column(DECIMAL(3, 2))
    default_max_tokens = Column(Integer)
    price_per_1k_tokens = Column(DECIMAL(10, 4))
    input_cost_per_1k_tokens = Column(DECIMAL(10, 4))
    output_cost_per_1k_tokens = Column(DECIMAL(10, 4))
    api_call_name = Column(String(100, 'utf8mb4_unicode_ci'))
    latency_ms_estimate = Column(Integer)
    rate_limit_per_minute = Column(Integer)
    fine_tuning_available = Column(TINYINT(1), nullable=False)
    multi_lang_support = Column(TINYINT(1), nullable=False)
    release_date = Column(Date)
    developer_name = Column(String(100, 'utf8mb4_unicode_ci'))
    developer_website = Column(String(255, 'utf8mb4_unicode_ci'))
    training_data_sources = Column(Text(collation='utf8mb4_unicode_ci'))
    parameters_count = Column(String(50, 'utf8mb4_unicode_ci'))
    model_architecture = Column(String(100, 'utf8mb4_unicode_ci'))
    documentation_url = Column(String(255, 'utf8mb4_unicode_ci'))
    demo_url = Column(String(255, 'utf8mb4_unicode_ci'))
    tags = Column(JSON)
    last_updated = Column(DateTime)
    created_at = Column(DateTime, nullable=False)
    updated_at = Column(DateTime, nullable=False)
    deleted_at = Column(DateTime)

    provider = relationship('ModelProvider')


class Organization(Base):
    __tablename__ = 'organizations'

    id = Column(Integer, primary_key=True, index=True)
    uuid = Column(String(36, 'utf8mb4_unicode_ci'), nullable=False, unique=True)
    name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    description = Column(String(1000, 'utf8mb4_unicode_ci'))
    created_by = Column(ForeignKey('users.id', ondelete='SET NULL'), index=True)
    is_active = Column(TINYINT(1))
    created_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))

    user = relationship('User')


class SecurityAuditLog(Base):
    __tablename__ = 'security_audit_logs'

    id = Column(Integer, primary_key=True, index=True)
    timestamp = Column(DateTime, nullable=False, index=True, server_default=text("CURRENT_TIMESTAMP"))
    event_type = Column(String(50, 'utf8mb4_unicode_ci'), nullable=False)
    user_id = Column(ForeignKey('users.id', ondelete='SET NULL'), index=True)
    api_key_id = Column(String(255, 'utf8mb4_unicode_ci'))
    client_ip = Column(String(45, 'utf8mb4_unicode_ci'))
    event_data = Column(JSON)
    description = Column(Text(collation='utf8mb4_unicode_ci'))

    user = relationship('User')


class Team(Base):
    __tablename__ = 'teams'

    id = Column(Integer, primary_key=True)
    name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    description = Column(String(1024, 'utf8mb4_unicode_ci'))
    owner_id = Column(ForeignKey('users.id'), nullable=False, index=True)
    is_active = Column(TINYINT(1))
    created_at = Column(DateTime)
    updated_at = Column(DateTime)
    settings = Column(JSON)

    owner = relationship('User')


class Token(Base):
    __tablename__ = 'tokens'

    id = Column(Integer, primary_key=True, index=True)
    token = Column(String(255, 'utf8mb4_unicode_ci'), unique=True)
    token_type = Column(String(50, 'utf8mb4_unicode_ci'))
    user_id = Column(ForeignKey('users.id', ondelete='CASCADE'), index=True)
    expires_at = Column(DateTime)
    created_at = Column(DateTime, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime, server_default=text("CURRENT_TIMESTAMP"))

    user = relationship('User')


t_category_model_association = Table(
    'category_model_association', metadata,
    Column('category_id', ForeignKey('model_categories.id'), index=True),
    Column('model_id', ForeignKey('model_provider_models.id'), index=True)
)


class KbDocument(Base):
    __tablename__ = 'kb_documents'

    id = Column(Integer, primary_key=True, index=True)
    kb_id = Column(ForeignKey('knowledge_bases.id'), nullable=False, index=True)
    file_name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    file_content = Column(Text(collation='utf8mb4_unicode_ci'))
    vector_id = Column(String(255, 'utf8mb4_unicode_ci'))
    created_at = Column(DateTime)
    updated_at = Column(DateTime)

    kb = relationship('KnowledgeBase')


class ModelModelCategory(Base):
    __tablename__ = 'model_model_categories'

    id = Column(BigInteger, primary_key=True, index=True)
    model_id = Column(ForeignKey('model_provider_models.id'), nullable=False, index=True, comment='Foreign key to model_provider_models.id')
    category_id = Column(ForeignKey('model_categories.id'), nullable=False, index=True, comment='Foreign key to model_categories.id')
    created_at = Column(DateTime, nullable=False, comment='Creation timestamp')
    deleted_at = Column(DateTime, comment='Soft delete timestamp')

    category = relationship('ModelCategory')
    model = relationship('ModelProviderModel')


class OrganizationMember(Base):
    __tablename__ = 'organization_members'

    id = Column(Integer, primary_key=True, index=True)
    organization_id = Column(ForeignKey('organizations.id', ondelete='CASCADE'), nullable=False, index=True)
    user_id = Column(ForeignKey('users.id', ondelete='CASCADE'), nullable=False, index=True)
    role = Column(ENUM('OWNER', 'ADMIN', 'MEMBER'), nullable=False)
    created_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))

    organization = relationship('Organization')
    user = relationship('User')


class Project(Base):
    __tablename__ = 'projects'

    id = Column(Integer, primary_key=True, index=True)
    uuid = Column(String(36, 'utf8mb4_unicode_ci'), nullable=False, unique=True)
    name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    description = Column(String(1000, 'utf8mb4_unicode_ci'))
    organization_id = Column(ForeignKey('organizations.id', ondelete='CASCADE'), nullable=False, index=True)
    created_by = Column(ForeignKey('users.id', ondelete='SET NULL'), index=True)
    status = Column(ENUM('ACTIVE', 'ARCHIVED', 'DELETED'), nullable=False)
    created_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))

    user = relationship('User')
    organization = relationship('Organization')


class PromptTemplate(Base):
    __tablename__ = 'prompt_templates'

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(100, 'utf8mb4_unicode_ci'), nullable=False)
    description = Column(String(500, 'utf8mb4_unicode_ci'))
    content = Column(Text(collation='utf8mb4_unicode_ci'), nullable=False)
    version = Column(String(50, 'utf8mb4_unicode_ci'), nullable=False)
    is_active = Column(TINYINT(1))
    application_id = Column(ForeignKey('applications.id', ondelete='CASCADE'), nullable=False, index=True)
    created_by = Column(ForeignKey('users.id', ondelete='CASCADE'), nullable=False, index=True)
    created_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))

    application = relationship('Application')
    user = relationship('User')


class TeamMember(Base):
    __tablename__ = 'team_members'

    id = Column(Integer, primary_key=True)
    team_id = Column(ForeignKey('teams.id'), nullable=False, index=True)
    user_id = Column(ForeignKey('users.id'), nullable=False, index=True)
    role = Column(String(50, 'utf8mb4_unicode_ci'), nullable=False)
    joined_at = Column(DateTime)
    is_active = Column(TINYINT(1))

    team = relationship('Team')
    user = relationship('User')


class ApiKey(Base):
    __tablename__ = 'api_keys'

    id = Column(Integer, primary_key=True, index=True)
    uuid = Column(String(36, 'utf8mb4_unicode_ci'), unique=True)
    name = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False)
    key = Column(String(255, 'utf8mb4_unicode_ci'), nullable=False, unique=True)
    created_by = Column(ForeignKey('users.id', ondelete='CASCADE'), nullable=False, index=True)
    created_at = Column(DateTime, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime)
    description = Column(Text(collation='utf8mb4_unicode_ci'))
    project_id = Column(ForeignKey('projects.id', ondelete='CASCADE'), nullable=False, index=True)
    status = Column(ENUM('ACTIVE', 'DISABLE', 'DELETED'), nullable=False)

    user = relationship('User')
    project = relationship('Project')


class PromptScenario(Base):
    __tablename__ = 'prompt_scenarios'

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(100, 'utf8mb4_unicode_ci'), nullable=False)
    description = Column(String(500, 'utf8mb4_unicode_ci'))
    content = Column(Text(collation='utf8mb4_unicode_ci'), nullable=False)
    template_id = Column(ForeignKey('prompt_templates.id', ondelete='CASCADE'), nullable=False, index=True)
    created_by = Column(ForeignKey('users.id', ondelete='CASCADE'), nullable=False, index=True)
    created_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))
    updated_at = Column(DateTime, nullable=False, server_default=text("CURRENT_TIMESTAMP"))

    user = relationship('User')
    template = relationship('PromptTemplate')
