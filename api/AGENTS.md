# AGENTS.md

This file provides context and instructions for AI coding agents working on the ZGI API project.

 Every line of code in GraphFlow must meet the standards of the world's leading research labs (Stanford AI Lab, Google DeepMind, Meta FAIR) and top-tier tech companies (OpenAI, Google, Meta). We combine the robustness of production-grade Go with the analytical depth of world-class AI research.

## Project Overview

ZGI API is an enterprise-grade Go backend built on Gin framework, providing:
- LLM Gateway with multi-provider routing and load balancing
- Multi-tenant SaaS architecture (Enterprise Group → Tenant → Account)
- Agent/Workflow orchestration platform
- Knowledge base and dataset management

## AI Model Selection Rule

- Never hardcode AI model names in runtime business logic as fallback defaults.
- Runtime AI calls must resolve models from explicit user/workflow configuration first, then organization/workspace default-model configuration.
- Use the existing default-model service or runtime model resolver when no explicit model is configured.
- If no usable default model exists, return a clear configuration error instead of silently falling back to a literal model such as `qwen...` or `gpt...`.
- Literal model names are acceptable only in tests, static examples, documentation, provider metadata, or seed/template content where they are not used as hidden runtime fallbacks.



## Tech Stack

- **Language**: Go 1.26.2
- **Framework**: Gin
- **Database**: PostgreSQL 12+ with GORM
- **Cache**: Redis 6.0+
- **Auth**: JWT
- **Migrations**: gormigrate

## Setup Commands

```bash
# Install dependencies
go mod download

# Run database migrations
go run cmd/migrate/main.go up

# Start development server (hot reload handled externally)
go run cmd/server/main.go

# Run tests
go test ./...

# Build
go build -o zgi-api cmd/server/main.go
```

## Git Workflow

### Branch Strategy

- `main` - Production branch, stable code
- `develop/v1` - Development branch, latest features
- `fix/*` or `feat/*` - Feature/fix branches

### Development Flow

```bash
# 1. Create feature/fix branch from develop/v1
git checkout develop/v1
git pull origin develop/v1
git checkout -b fix/your-feature-name

# 2. Make changes and commit
git add -A
git commit -m "fix: description of the change"

# 3. Push branch to origin
git push origin fix/your-feature-name

# 4. Create PR to develop/v1 on GitHub
# Go to: https://github.com/zgiai/zgi-api/pull/new/fix/your-feature-name
# Set base branch: develop/v1

# 5. After PR is merged, sync to main
git checkout main
git pull origin main
git merge develop/v1
git push origin main
```

### Commit Message Convention

- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `refactor:` - Code refactoring
- `test:` - Adding tests
- `chore:` - Maintenance tasks

## Project Structure

```
cmd/                    # Entry points
  migrate/              # Database migration tool
  server/               # Main API server
internal/
  container/            # Dependency injection container
  dto/                  # Data Transfer Objects
  migrations/           # Database migrations (gormigrate)
  modules/              # Business modules
    llm/                # LLM Gateway module
      adapter/          # Protocol adapters (OpenAI, Anthropic, etc.)
      channel/          # System channels management
      credential/       # API credentials management
      gateway/          # Request routing and load balancing
      handler/          # HTTP handlers
      llmmodel/         # Model management (v2)
      model/            # Data models
      provider/         # Provider management (v2)
      repository/       # Data access layer
      service/          # Business logic
      shared/           # Shared utilities
    account/            # User account management
    enterprise/         # Enterprise group management
    tenant/             # Tenant/workspace management
    agents/             # AI agents module
    workflow/           # Workflow orchestration
    dataset/            # Knowledge base/dataset
  shared/               # Cross-module shared code
middleware/             # HTTP middleware
routes/                 # Route definitions
pkg/                    # Reusable packages
  logger/               # Zap logger wrapper
  response/             # Standardized API responses
  database/             # Database utilities
tests/                  # Test files (all tests go here, not in business modules)
```

## Go Architecture Guidelines

### Standard Module Structure

```
module/
  ├── dto/           # Data Transfer Objects (request/response)
  ├── handler/       # HTTP handlers (Gin controllers)
  ├── model/         # Data models (GORM entities)
  ├── repository/    # Data access layer (database operations)
  ├── service/       # Business logic layer
  ├── module.go      # Module registration and dependency wiring
  └── router.go      # Route definitions (optional)
```

### File Size Limits

- **Maximum 700 lines per file** - Split large files into smaller, focused units
- **Maximum 50 lines per function** - Extract complex logic into helper functions
- **Maximum 5 parameters per function** - Use struct for multiple parameters

### Layer Responsibilities

| Layer | Responsibility | Dependencies |
|-------|---------------|--------------|
| **Handler** | HTTP request/response, validation, context extraction | Service |
| **Service** | Business logic, orchestration, transactions | Repository |
| **Repository** | Database operations, queries, CRUD | GORM/DB |
| **Model** | Data structures, table definitions | None |
| **DTO** | API contracts, request/response schemas | None |

### Design Patterns

**Dependency Injection:**
```go
// Good: Constructor injection
func NewUserService(repo UserRepository, cache CacheService) *UserService {
    return &UserService{repo: repo, cache: cache}
}

// Bad: Global variables or direct instantiation
var userRepo = &UserRepository{db: globalDB}
```

**Interface-based Design:**
```go
// Define interface in the consumer package
type UserRepository interface {
    GetByID(ctx context.Context, id uuid.UUID) (*User, error)
    Create(ctx context.Context, user *User) error
}

// Implementation in repository package
type userRepository struct {
    db *gorm.DB
}
```

**Error Wrapping:**
```go
// Good: Wrap with context
if err := repo.Create(ctx, user); err != nil {
    return fmt.Errorf("failed to create user %s: %w", user.Email, err)
}

// Bad: Naked error return
return repo.Create(ctx, user)
```

### Code Organization Rules

1. **One struct per file** for models (e.g., `user.go`, `tenant.go`)
2. **Group related functions** in the same file
3. **Separate interfaces** into `interface.go` or define in consumer package
4. **Keep handlers thin** - delegate logic to services
5. **No business logic in repositories** - only data access

## Code Style Guidelines

### General Rules
- **All code comments MUST be in English** - No Chinese in code, comments, or variable names
- Follow Go official style guide and Effective Go
- Use `gofmt` and `golint`
- Prefer functional patterns where possible
- **Keep files under 700 lines** - Split if exceeding

### Naming Conventions
- Use camelCase for variables and functions
- Use PascalCase for exported types and functions
- Use snake_case for database columns and JSON fields
- Prefix interfaces with descriptive names (e.g., `LLMModelService`)
- Use meaningful names: `userService` not `us`, `GetByID` not `Get`

### Error Handling
- Always handle errors explicitly
- Use wrapped errors with context: `fmt.Errorf("failed to create model: %w", err)`
- Return standardized error responses via `pkg/response`
- Define domain errors in `errors.go` within the module

### Project-Local Coding Standard Inspired by `third_party`

These rules are project-local to `/Users/du/wwwdata/zgi-api`. Apply them when coding in this repository only; do not generalize them to unrelated projects.

Use `internal/capabilities/contentparse/engines/hyperparse` as the in-repo runtime mirror for parsing behavior. Keep capability boundaries in front of it, and avoid leaking engine-specific details into business modules.

**Adopt these habits:**
- Keep public APIs thin and explicit. Constructors such as `NewXxxService`, `NewXxxRepository`, or `NewXxxHandler` should only wire dependencies and defaults.
- Prefer small interfaces owned by the consumer package. A narrow interface with 1-5 methods is better than a broad service surface.
- Normalize inputs at boundaries: trim whitespace, lowercase stable enum-like values, normalize extensions/keys, and validate empty values before doing work.
- Use early returns for invalid input, missing dependencies, unsupported formats, and failed authorization checks.
- Keep handlers and adapters shallow. They should translate input/output and delegate business decisions to services.
- Preserve stable public method signatures when reasonable. If a parameter is reserved for future use, assign it explicitly with `_ = value` and leave a short English comment only when the reason is not obvious.
- For configuration helpers, provide deterministic defaults, reject invalid values safely, and clamp risky limits such as concurrency, batch size, timeout, and page count.
- Prefer plain Go tests with inline fixtures, table-driven cases where useful, and names like `TestFunction_Scenario_ExpectedBehavior`.
- Test behavior and edge cases directly: empty input, unsupported values, fallback/default behavior, normalization, and failure paths.

**Do not adopt these `third_party` habits in main project code:**
- Do not write Chinese in code, comments, identifiers, test names, or error messages.
- Do not create files over 700 lines or functions over 50 lines; split focused helpers instead.
- Do not use `map[string]any` for regular API contracts, database models, or service DTOs. Use typed structs unless the data is genuinely dynamic diagnostics or provider-native JSON.
- Do not return raw infrastructure errors from service or handler boundaries. Wrap with context and convert HTTP responses through project response helpers.
- Do not rely on environment variables scattered through business logic. Centralize config in the existing configuration/container patterns unless the module already has a local convention.

### Database Migrations
- **NEVER execute raw SQL directly** - Always use migration files
- Create new migration file in `internal/migrations/`
- Register migration in `internal/migrations/runner.go`
- Use `IF NOT EXISTS` and `IF EXISTS` for idempotency
- Migration ID format: `YYYYMMDD000XXX` (e.g., `20251214000036`)

Example migration:
```go
func M0036_add_system_enabled_fields() *gormigrate.Migration {
    return &gormigrate.Migration{
        ID: "20251214000036",
        Migrate: func(tx *gorm.DB) error {
            return tx.Exec(`
                ALTER TABLE llm_providers 
                ADD COLUMN IF NOT EXISTS is_system_enabled BOOLEAN NOT NULL DEFAULT true
            `).Error
        },
        Rollback: func(tx *gorm.DB) error {
            return tx.Exec(`
                ALTER TABLE llm_providers DROP COLUMN IF EXISTS is_system_enabled
            `).Error
        },
    }
}
```

## Debugging Methodology: Global Analysis Approach

When investigating bugs or issues, **ALWAYS take a global perspective**:

### 1. **Query Database First, Don't Assume**
```sql
-- Check actual values in database
SELECT DISTINCT field_name FROM table_name WHERE condition;

-- Example: Check what protocols are actually used
SELECT DISTINCT protocol FROM llm_tenant_routes WHERE protocol IS NOT NULL;
```

### 2. **Trace Data Flow End-to-End**
```
Frontend Options → API Endpoint → Service Layer → Repository → Database
                ↓
          Where is the mismatch?
```

### 3. **Compare Expectations vs Reality**
- What does frontend show? (options from API)
- What does database contain? (actual data)
- **Are they from the same source?**

### 4. **Understand Field Relationships**
- Is it a Foreign Key constraint?
- Is it free text field (VARCHAR)?
- Check with: `\d table_name` in psql

### 5. **Find Root Cause, Not Symptoms**
**Bad**: "cohere doesn't work, let's add it to llm_protocols table"
**Good**: "Protocol filter options come from llm_protocols, but actual data uses free text. Need to query actual data for filter options."

### 6. **Verify ALL Assumptions**
```bash
# Don't trust variable names - verify actual data
psql $DB_URL -c "SELECT * FROM table LIMIT 5;"

# Check what frontend actually calls
curl -s "https://api.zgi.im/path" | jq
```

### Real Example: Protocol Filter Investigation

**Problem**: Protocol filter doesn't show "cohere" option

**Wrong Approach**: Assume protocols come from code constants

**Correct Global Analysis**:
1. Query llm_protocols table → Found: openai, anthropic, google (3)
2. Query actual channel protocol field → Found: openai, cohere (2)
3. Check frontend API call → GET /llm/protocols (returns only 3)
4. **Root cause**: Frontend gets options from llm_protocols table, but channel.protocol is free text field
5. **Solution**: Create new endpoint returning actual protocols used: `SELECT DISTINCT protocol FROM channels`

## Testing Instructions

- All test files go in `tests/` directory, NOT in business module directories
- Run specific test: `go test -v ./tests/... -run TestName`
- Run all tests: `go test ./...`
- Add tests for new features before implementation when possible

## API Design

### Route Structure

**IMPORTANT:** All routes are registered under `/console/api` prefix in `routes/router.go`.

| Route Pattern | Full Path | Description |
|---------------|-----------|-------------|
| `/llm/*` | `/console/api/llm/*` | Console/Tenant LLM APIs |
| `/v1/*` | `/v1/*` | OpenAI-compatible AI Gateway (uses LLM API key auth) |

**Examples:**
- Provider sync: `POST /console/api/llm/modelmeta/sync-provider-full/:provider`

### Response Format
```json
{
  "code": "0",
  "message": "success",
  "data": { ... }
}
```

### Authentication
- JWT token in `Authorization: Bearer <token>` header
- Tenant context from JWT claims or `X-Tenant-ID` header
- System admin check via `middleware.SystemAdminRequired()`

## LLM Gateway Architecture

### Three-Layer Model
1. **System Level** (Global)
   - `llm_providers` - Global provider definitions
   - `llm_models` - Global model definitions
   - `llm_routes` - Official/private route definitions
   - `llm_official_model_snapshots` - Official cloud model snapshots

2. **Tenant Level** (Configuration)
   - `llm_tenant_providers` - Tenant provider enablement
   - `llm_tenant_models` - Tenant model enablement
   - `llm_tenant_channels` - Tenant channel overrides

3. **Request Level** (Runtime)
   - Channel routing based on priority/weight
   - Load balancing across multiple channels
   - Automatic failover

### Key Fields
- `is_active` - Technical availability (can the provider/model work?)
- `is_system_enabled` - Business control (should tenants see this?)

## Security Considerations

- Never log API keys or passwords
- Use `json:"-"` tag to exclude sensitive fields from JSON
- Encrypt API keys before storing (use `shared.CryptoService`)
- Validate all user inputs
- Use parameterized queries (GORM handles this)

## Common Patterns

### Creating a New Module
1. Create directory under `internal/modules/<module_name>/`
2. Add model.go, repository.go, service.go, handler.go
3. Register routes in `routes/v1/`
4. Add to dependency container if needed

### Adding a Database Column
1. Create migration file: `internal/migrations/m00XX_description.go`
2. Register in `runner.go`
3. Update Go model struct
4. Run migration: `go run cmd/migrate/main.go up`

### Service Hot Reload
- Do NOT start the server yourself - it's hot-reloaded externally
- If restart is truly needed, inform the user first

## Git Development Workflow (SOP)

### Branch Strategy

**Main Branches:**
- `main` - Production-ready code
- `develop/v1` - Development branch for version 1.x

**Feature Development Workflow:**

#### 1. Create Feature Branch
```bash
# Always branch from develop/v1
git checkout develop/v1
git pull origin develop/v1

# Create feature branch with descriptive name
git checkout -b feature/model-availability-check
# or
git checkout -b fix/channel-routing-bug
# or
git checkout -b refactor/llm-gateway-architecture
```

**Branch Naming Conventions:**
- `feature/` - New features
- `fix/` - Bug fixes
- `refactor/` - Code refactoring
- `docs/` - Documentation updates
- `test/` - Test additions or updates

#### 2. Development Process
```bash
# Make changes and commit frequently
git add -A
git commit -m "feat(llm): add model availability checking

- Implement AvailabilityService with channel analysis
- Add HTTP endpoints for availability checks
- Integrate with LLM module routing"

# Push feature branch to remote
git push origin feature/model-availability-check
```

**Commit Message Format:**
```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**
- `feat` - New feature
- `fix` - Bug fix
- `refactor` - Code refactoring
- `docs` - Documentation changes
- `test` - Test updates
- `chore` - Build/tooling changes

**Examples:**
```
feat(llm): implement model availability checking
fix(workflow): correct model_config propagation to LLM node
refactor(gateway): improve channel routing logic
docs(agents): add git workflow SOP
```

#### 3. Pull Request Process

**Before Creating PR:**
```bash
# Ensure code compiles
go build ./...

# Run tests
go test ./...

# Sync with latest develop/v1
git checkout develop/v1
git pull origin develop/v1
git checkout feature/model-availability-check
git merge develop/v1

# Resolve conflicts if any
# Test again after merge
```

**Create Pull Request:**
1. Push final changes to feature branch
2. Create PR on GitHub: `feature/model-availability-check` → `develop/v1`
3. Fill PR template with:
   - **Description**: What does this PR do?
   - **Changes**: List of key changes
   - **Testing**: How was it tested?
   - **Screenshots**: If UI changes

**PR Review Checklist:**
- [ ] Code compiles without errors
- [ ] Tests pass
- [ ] No Chinese in code/comments
- [ ] Follows Go style guidelines
- [ ] API endpoints use semantic naming
- [ ] Database changes use migrations
- [ ] Documentation updated (if needed)

#### 4. Merge and Cleanup
```bash
# After PR approved and merged
git checkout develop/v1
git pull origin develop/v1

# Delete local feature branch
git branch -d feature/model-availability-check

# Delete remote feature branch
git push origin --delete feature/model-availability-check
```

#### 5. Hotfix Workflow (Production Issues)
```bash
# Create hotfix from main
git checkout main
git pull origin main
git checkout -b hotfix/critical-bug-name

# Fix and test
# ...

# Merge to both main and develop/v1
git checkout main
git merge hotfix/critical-bug-name
git push origin main

git checkout develop/v1
git merge hotfix/critical-bug-name
git push origin develop/v1

# Tag release
git tag -a v1.0.1 -m "Hotfix: critical bug"
git push origin v1.0.1
```

### Best Practices

**DO:**
- ✅ Always branch from `develop/v1` for new work
- ✅ Keep feature branches small and focused
- ✅ Commit frequently with descriptive messages
- ✅ Sync with `develop/v1` regularly
- ✅ Test before creating PR
- ✅ Delete branches after merge

**DON'T:**
- ❌ Commit directly to `develop/v1` or `main`
- ❌ Create long-lived feature branches (>1 week)
- ❌ Mix multiple unrelated changes in one branch
- ❌ Force push to shared branches
- ❌ Merge without review (except trivial fixes)

## Do NOT

- Write Chinese in code, comments, or variable names
- Execute raw SQL outside of migration files
- Create random test files or shell scripts
- Delete or modify database data directly
- Use `pkill -f` command
- Create summary markdown files unless explicitly asked
- Put test files in business module directories

## E2E Testing with Live Server

### Prerequisites
- Server running on port 2620 (or configured port)
- Database with migrations applied
- Test account credentials

### Step 1: Login to Get JWT Token

```bash
# Login request
curl -X POST "http://localhost:2620/console/api/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"i@zgi.ai","password":"YOUR_PASSWORD"}'

# Response contains access_token
{
  "code": "0",
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "..."
  }
}
```

### Step 2: Test LLM Management APIs

```bash
# Set token variable
TOKEN="eyJhbGciOiJIUzI1NiIs..."

# Sync providers from ModelMeta
curl -X POST "http://localhost:2620/console/api/llm/modelmeta/sync-providers" \
  -H "Authorization: Bearer $TOKEN"

# Sync one provider with models
curl -X POST "http://localhost:2620/console/api/llm/modelmeta/sync-provider-full/openai" \
  -H "Authorization: Bearer $TOKEN"
```

### Step 3: Test Console/Tenant APIs

```bash
# List Tenant Credentials
curl -X GET "http://localhost:2620/console/api/console/llm/credentials" \
  -H "Authorization: Bearer $TOKEN"

# List Tenant Routes
curl -X GET "http://localhost:2620/console/api/console/llm/routes" \
  -H "Authorization: Bearer $TOKEN"
```

### Step 4: Test Gateway API (OpenAI-compatible)

```bash
# Create LLM API Key first via Console API
# Then use the API key for Gateway calls

LLM_API_KEY="sk-..."

# Chat Completions
curl -X POST "http://localhost:2620/v1/chat/completions" \
  -H "Authorization: Bearer $LLM_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# List Models
curl -X GET "http://localhost:2620/v1/models" \
  -H "Authorization: Bearer $LLM_API_KEY"
```

### Running Automated E2E Tests

```bash
# Run all E2E tests
go test ./tests/llm/e2e/... -v -count=1

```

## Model Availability Feature

### Overview

The Model Availability feature provides intelligent visibility and validation for LLM models, ensuring users know which models are actually usable before enabling them.

### Problem Statement

**Issue:** Users could enable models without knowing if their tenant has configured channels to support them, leading to runtime errors like "no provider available for this model".

**Solution:** Proactive availability checking at multiple levels:
1. **Display Level** - Show availability status in model list
2. **Configuration Level** - Validate when enabling models
3. **Runtime Level** - Pre-check before workflow/app execution

### Architecture

```
┌─────────────────────────────────────────────────────┐
│ Frontend - Model Management UI                      │
│  • Model list with availability indicators          │
│  • Enable/disable with real-time validation         │
│  • Channel configuration wizard                     │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│ Backend - Availability Module                       │
│                                                     │
│  ┌──────────────────────────────────────────┐     │
│  │ AvailabilityHandler (HTTP Layer)         │     │
│  │  • CheckModelAvailability                │     │
│  │  • BatchCheckAvailability                │     │
│  └──────────────────────────────────────────┘     │
│                     ↓                               │
│  ┌──────────────────────────────────────────┐     │
│  │ AvailabilityService (Business Logic)     │     │
│  │  • Filter routes by model/provider       │     │
│  │  • Analyze channel status                │     │
│  │  • Determine availability status         │     │
│  └──────────────────────────────────────────┘     │
│                     ↓                               │
│  ┌──────────────────────────────────────────┐     │
│  │ Data Sources                             │     │
│  │  • TenantRouteRepository                 │     │
│  │  • ModelRepository                       │     │
│  │  • ChannelRepository                     │     │
│  └──────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────┘
```

### API Design

#### 1. Check Single Model Availability
```
GET /console/api/llm/models/{id}/check-availability

Response:
{
  "model_id": "uuid",
  "model_name": "gpt-4o-mini",
  "provider": "openai",
  "status": "available",  // available | partial | unavailable
  "channel_info": {
    "total_count": 3,
    "ready_count": 3,
    "needs_config_count": 0,
    "channels": [
      {
        "id": "uuid",
        "name": "AGICTO OpenAI 1",
        "provider": "openai",
        "status": "ready",
        "priority": 10,
        "weight": 100
      }
    ],
    "warnings": []
  }
}
```

#### 2. Batch Check Model Availability
```
POST /console/api/llm/models/check-availability
Body: { "model_ids": ["uuid1", "uuid2"] }

Response:
{
  "results": [
    { /* ModelAvailability */ },
    { /* ModelAvailability */ }
  ]
}
```

#### 3. Enhanced Model List (with availability)
```
GET /console/api/llm/models?include_availability=true

Response:
{
  "models": [
    {
      "id": "uuid",
      "name": "gpt-4o-mini",
      "provider": "openai",
      "is_enabled": true,
      "availability": {
        "status": "available",
        "ready_channels": 3
      }
    }
  ]
}
```

### Data Structures

```go
// Availability status enumeration
type ModelAvailabilityStatus string

const (
    ModelAvailable   ModelAvailabilityStatus = "available"   // Has working channels
    ModelPartial     ModelAvailabilityStatus = "partial"     // Has channels but need config
    ModelUnavailable ModelAvailabilityStatus = "unavailable" // No channels
)

// Main availability response
type ModelAvailability struct {
    ModelID     uuid.UUID               `json:"model_id"`
    ModelName   string                  `json:"model_name"`
    Provider    string                  `json:"provider"`
    Status      ModelAvailabilityStatus `json:"status"`
    ChannelInfo ChannelAvailabilityInfo `json:"channel_info"`
    UpdatedAt   time.Time               `json:"updated_at"`
}

// Channel information
type ChannelAvailabilityInfo struct {
    TotalCount       int                `json:"total_count"`
    ReadyCount       int                `json:"ready_count"`
    NeedsConfigCount int                `json:"needs_config_count"`
    Channels         []ChannelBriefInfo `json:"channels"`
    Warnings         []string           `json:"warnings,omitempty"`
}
```

### Implementation Guide

#### File Structure
```
internal/modules/llm/availability/
├── dto/
│   └── availability_dto.go          # Data structures
├── service/
│   ├── interface.go                 # Service interface
│   └── availability_service.go      # Core business logic
├── handler/
│   ├── availability_handler.go      # HTTP handlers
│   └── routes.go                    # Route registration
└── module.go                        # Module initialization
```

#### Key Implementation Steps

1. **Create DTO definitions** (`availability_dto.go`)
   - Define `ModelAvailability`, `ChannelAvailabilityInfo`, etc.

2. **Implement service layer** (`availability_service.go`)
   - `CheckModelAvailability` - Check single model
   - `BatchCheckAvailability` - Check multiple models
   - `filterRoutesForModel` - Filter routes supporting the model
   - `analyzeChannelStatus` - Analyze channel readiness
   - `determineStatus` - Determine overall availability

3. **Implement handler layer** (`availability_handler.go`)
   - Handle HTTP requests
   - Extract tenant context
   - Call service methods
   - Return standardized responses

4. **Register routes** (`routes.go`)
   - Register availability endpoints
   - Apply authentication middleware

#### Integration Points

**Enhance Existing APIs:**
- `ListTenantModels` - Add optional `include_availability` parameter
- `ToggleModel` - Call availability check before enabling

**Channel Router Integration:**
- Use existing `filterRoutesForModel` logic from `ChannelRouter`
- Leverage `TenantRouteRepository.GetEnabledRoutes`

### Testing Strategy

```bash
# 1. Unit tests for availability service
go test ./internal/modules/llm/availability/service/...

# 2. Integration tests with real database
go test ./tests/integration/availability/...

# 3. E2E tests with HTTP calls
go test ./tests/e2e/availability/...
```

### Usage Examples

**Frontend - Model List:**
```javascript
// Get models with availability
const response = await fetch('/console/api/llm/models?include_availability=true')
const { models } = await response.json()

// Display with status indicators
models.forEach(model => {
  const indicator = model.availability.status === 'available' ? '✅' 
                  : model.availability.status === 'partial' ? '⚠️' 
                  : '❌'
  console.log(`${indicator} ${model.name} (${model.availability.ready_channels} channels)`)
})
```

**Frontend - Enable Model with Validation:**
```javascript
// Check availability before enabling
const checkResponse = await fetch(`/console/api/llm/models/${modelId}/check-availability`)
const availability = await checkResponse.json()

if (availability.status === 'unavailable') {
  alert('This model requires channel configuration. Please add a channel first.')
  return
}

// Proceed with enabling
await enableModel(modelId)
```

### Future Enhancements

1. **Channel Health Monitoring**
   - Track success rate, latency, error rate
   - Auto-disable unhealthy channels

2. **Intelligent Recommendations**
   - Suggest channel configurations based on usage patterns
   - Recommend backup channels for high-availability

3. **Cost Optimization**
   - Show estimated costs per model
   - Recommend cost-effective alternatives

4. **Caching Strategy**
   - Cache availability results (TTL: 5 minutes)
   - Invalidate on channel/route changes

---

## Release SOP (Standard Operating Procedure)

When completing a feature or fix, follow these steps:

### 1. Create GitHub Issue for Changelog
```bash
gh issue create --title "🚀 Changelog YYYY-MM-DD: [Feature/Fix Description]" --body "[Changelog content]"
```

### 2. Commit and Create PR
```bash
# Create feature branch
git checkout -b feature/[feature-name]-YYYYMMDD

# Commit changes
git add -A
git commit -m "feat/fix: [description]

- [change 1]
- [change 2]

Closes #[issue-number]"

# Push and create PR
git push -u origin feature/[feature-name]-YYYYMMDD
gh pr create --title "[type]: [description]" --body "[PR description]" --base develop/v1
```

### 3. Write Test Cases
All tests should be placed in `tests/` directory:
```bash
tests/
  llm/
    gateway_test.go      # LLM Gateway tests
    channel_test.go      # Channel routing tests
    model_service_test.go # Model service tests
```

Test file naming: `[module]_test.go`

### 4. Run Tests Before Merge
```bash
go test ./tests/... -v
```

### 5. Post-Merge Verification
- Deploy to staging/production
- Run smoke tests via curl or Postman
- Verify in logs for errors

### Example Workflow
```bash
# 1. Create issue
gh issue create --title "🚀 Changelog 2025-12-19: LLM API Fix" --body "..."

# 2. Create branch and commit
git checkout -b feature/llm-fixes-20251219
git add -A && git commit -m "fix: LLM API fixes"
git push -u origin feature/llm-fixes-20251219

# 3. Create PR
gh pr create --title "fix: LLM API fixes" --base develop/v1

# 4. Write tests
# Edit tests/llm/gateway_test.go

# 5. Run tests
go test ./tests/... -v
```

---

## Implementation Plan Writing Standards

When creating technical proposals and implementation plans for new features or architectural changes, follow these standards to ensure consistency, completeness, and actionability.

### Core Principles

1. **Clarity Over Complexity** - Simple, understandable solutions preferred over over-engineered ones
2. **Evidence-Based** - All decisions backed by code analysis, not assumptions
3. **Actionable** - Every step must be executable with clear instructions
4. **Verifiable** - Include concrete testing and validation methods
5. **Maintainable** - Consider long-term maintenance costs

### Required Document Structure

Every implementation plan MUST include these sections:

#### 1. Executive Summary (Required)
- **Problem Statement**: What problem are we solving?
- **Proposed Solution**: High-level approach (2-3 sentences)
- **Key Metrics**: Success criteria (e.g., "API success rate > 99.5%")
- **Timeline**: Estimated implementation time
- **Risk Level**: Low/Medium/High

**Example:**
```markdown
## Executive Summary

**Problem**: Protocol selection is manual and error-prone, leading to failures.
**Solution**: Automatic protocol selection with intelligent failover.
**Metrics**: 99.5% API success rate, <1.5s P95 latency.
**Timeline**: 2-3 weeks.
**Risk**: Medium (database schema changes).
```

#### 2. Current State Analysis (Required)
- **Existing Code Review**: Analyze relevant files with line counts
- **Data Model Analysis**: Current database schema
- **Dependency Mapping**: Module dependencies and call chains
- **Pain Points**: Specific problems with current implementation

**Template:**
```markdown
## Current State Analysis

### Existing Code Structure
- `gateway/service.go` (627 lines) - Main gateway logic
- `channel/service.go` (1205 lines) - Channel management
- Key dependencies: [list]

### Current Data Model
```sql
-- Show relevant tables
```

### Pain Points
1. [Specific problem with evidence]
2. [Specific problem with evidence]
```

#### 3. Proposed Solution (Required)
- **Architecture Diagram**: Visual representation
- **Data Model Changes**: New tables/columns with SQL
- **Core Components**: New services/modules to create
- **Design Decisions**: Why this approach over alternatives

**Template:**
```markdown
## Proposed Solution

### Architecture
```
[ASCII diagram or mermaid]
```

### Data Model Changes
```sql
ALTER TABLE llm_models ADD COLUMN protocol_config JSONB;
```

### Core Components
1. **ProtocolSelector** - Selects optimal protocol
2. **HealthMonitor** - Tracks channel health
3. **CircuitBreaker** - Prevents cascading failures

### Design Rationale
- **Why not ML-based selection?** - Complexity outweighs benefits at current scale
- **Why a circuit breaker?** - Industry-proven pattern for resilience
```

#### 4. Implementation Steps (Required)
- **Phased Approach**: Break into stages (1-2 weeks each)
- **Detailed Tasks**: Specific files to create/modify
- **Code Examples**: Show key implementations
- **Dependencies**: What must be done first

**Template:**
```markdown
## Implementation Steps

### Phase 1: Database Migration (1 day)
**Tasks:**
- [ ] Create migration file `m00XX_add_protocol_fields.go`
- [ ] Add `protocol_config` to `llm_models`
- [ ] Add health fields to active route telemetry tables

**Files to Create:**
- `internal/migrations/m00XX_add_protocol_fields.go`

**Code Example:**
```go
func M00XX_AddProtocolFields() *gormigrate.Migration {
    // ... implementation
}
```

### Phase 2: Core Logic (2-3 days)
[Similar structure]
```

#### 5. Testing & Verification (Required)
- **Unit Tests**: Specific test cases with expected behavior
- **Integration Tests**: End-to-end scenarios
- **Manual Testing**: curl commands or UI steps
- **Performance Tests**: Load testing if applicable

**Template:**
```markdown
## Testing & Verification

### Unit Tests
**File**: `internal/modules/llm/protocol/service/protocol_selector_test.go`
```go
func TestProtocolSelector_SelectProtocolForModel(t *testing.T) {
    // Test implementation
}
```

### Integration Tests
**Scenario**: Protocol failover on primary failure
**Steps:**
1. Create model with primary=cohere, fallback=openai
2. Simulate cohere failure
3. Verify request succeeds via openai

### Manual Testing
```bash
# Test protocol selection
curl -X POST "http://localhost:2620/v1/chat/completions" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"model": "command-r-plus", "messages": [...]}'
```

### Success Criteria
- [ ] All unit tests pass
- [ ] Integration test shows failover works
- [ ] Manual test confirms protocol selection
- [ ] Performance: P95 latency < 1.5s
```

#### 6. Deployment & Rollback (Required)
- **Deployment Steps**: Exact commands to deploy
- **Rollback Plan**: How to revert if issues arise
- **Monitoring**: What to watch after deployment

**Template:**
```markdown
## Deployment & Rollback

### Deployment Steps
1. Backup database: `pg_dump ...`
2. Run migration: `go run cmd/migrate/main.go up`
3. Deploy code: `git pull && go build && systemctl restart`
4. Verify: `curl http://localhost:2620/health`

### Rollback Plan
1. Revert code: `git checkout <previous-commit>`
2. Rollback migration: `go run cmd/migrate/main.go down`
3. Restore backup if needed

### Monitoring
- Watch error rate in logs
- Monitor API success rate dashboard
- Alert if failure rate > 1%
```

#### 7. Risk Assessment (Required)
- **Identified Risks**: What could go wrong
- **Impact**: High/Medium/Low
- **Mitigation**: How to prevent or handle

**Template:**
```markdown
## Risk Assessment

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Database migration fails | High | Low | Test in staging first, have rollback script |
| Performance degradation | Medium | Medium | Use caching, monitor metrics |
| Protocol selection errors | Medium | Low | Extensive testing, manual override option |
```

### Optional Sections (When Applicable)

#### 8. Performance Analysis
- Benchmarks before/after
- Expected improvements
- Resource usage (CPU, memory, DB connections)

#### 9. Security Considerations
- Authentication/authorization changes
- Data encryption requirements
- API key handling

#### 10. Documentation Updates
- API documentation changes
- User guide updates
- Internal wiki updates

### Quality Checklist

Before submitting an implementation plan, verify:

- [ ] **Complete**: All required sections present
- [ ] **Specific**: No vague statements like "improve performance"
- [ ] **Executable**: Every step has clear instructions
- [ ] **Testable**: Concrete verification methods defined
- [ ] **Realistic**: Timeline matches complexity
- [ ] **Code-Based**: References actual files and line numbers
- [ ] **Diagrammed**: Architecture visually explained
- [ ] **Risk-Aware**: Potential issues identified
- [ ] **Rollback-Ready**: Revert plan documented
- [ ] **English-Only**: No Chinese in technical content

### Anti-Patterns to Avoid

❌ **Vague Goals**: "Make it faster" → ✅ "Reduce P95 latency from 2s to 1.5s"
❌ **No Code Analysis**: Assuming structure → ✅ Viewing actual files
❌ **Missing Tests**: "Will test later" → ✅ Specific test cases defined
❌ **No Rollback**: "Hope it works" → ✅ Detailed rollback procedure
❌ **Over-Engineering**: ML for simple rules → ✅ Simple solutions first
❌ **Under-Specification**: "Add caching" → ✅ "Add Redis cache with 5min TTL"

### Document Naming Convention

```
docs/[module]/[ISSUE_NUMBER]_[DESCRIPTIVE_NAME].md

Examples:
- docs/llm/104_PROTOCOL_IMPLEMENTATION_PLAN.md
- docs/workflow/205_NODE_EXECUTION_REFACTOR.md
- docs/dataset/301_VECTOR_SEARCH_OPTIMIZATION.md
```

### Template Prompt for AI Agents

When asked to create an implementation plan, use this prompt:

```
Create a comprehensive implementation plan for [FEATURE/ISSUE] following these requirements:

1. Analyze existing code in [relevant modules]
2. Include all required sections: Executive Summary, Current State, Proposed Solution, Implementation Steps, Testing, Deployment, Risks
3. Provide specific file paths and line numbers
4. Include code examples for key components
5. Define concrete success metrics
6. Create detailed test cases
7. Specify deployment and rollback procedures
8. Keep solutions simple and maintainable
9. Avoid over-engineering
10. Use English only

Output format: Markdown document following ZGI Implementation Plan Standards
```

### Example: Good vs Bad Plan

**❌ Bad Plan:**
```markdown
## Add Caching

We should add caching to improve performance.

### Steps
1. Add cache
2. Test it
3. Deploy

### Timeline
1 week
```

**✅ Good Plan:**
```markdown
## Protocol Configuration Caching Implementation

### Executive Summary
**Problem**: Every API request queries database for protocol config, adding 50ms latency.
**Solution**: In-memory cache with 5-minute TTL, reducing DB queries by 95%.
**Metrics**: P95 latency reduction from 2s to 1.5s.
**Timeline**: 2 days.
**Risk**: Low (cache invalidation is simple).

### Current State Analysis
**File**: `internal/modules/llm/gateway/service.go:245-260`
```go
// Current: DB query on every request
func (g *Gateway) GetProtocol(model string) (string, error) {
    return g.db.Query("SELECT protocol FROM llm_models WHERE name = ?", model)
}
```
**Pain Point**: 50ms DB latency per request, 1000 req/s = 50,000ms wasted.

### Proposed Solution
**Component**: `ProtocolCache` with sync.RWMutex
**Cache Key**: Model name
**TTL**: 5 minutes
**Invalidation**: On model update

```go
type ProtocolCache struct {
    mu    sync.RWMutex
    cache map[string]*CachedProtocol
    ttl   time.Duration
}
```

### Implementation Steps
**Phase 1: Create Cache (4 hours)**
- [ ] Create `internal/modules/llm/protocol/cache/protocol_cache.go`
- [ ] Implement Get/Set/Invalidate methods
- [ ] Add unit tests

**Phase 2: Integrate (4 hours)**
- [ ] Modify `gateway/service.go:245` to use cache
- [ ] Add cache invalidation on model updates
- [ ] Integration test

### Testing
**Unit Test**: `protocol_cache_test.go`
```go
func TestCache_GetSet(t *testing.T) {
    cache := NewProtocolCache(5 * time.Minute)
    cache.Set("gpt-4", "openai", "")
    primary, _, found := cache.Get("gpt-4")
    assert.True(t, found)
    assert.Equal(t, "openai", primary)
}
```

**Performance Test**:
```bash
# Before: 2000ms P95
# After: 1500ms P95 (target)
ab -n 1000 -c 10 http://localhost:2620/v1/chat/completions
```

### Deployment
1. Deploy code (no DB changes needed)
2. Monitor cache hit rate (expect >95%)
3. Verify latency reduction in metrics

### Rollback
Remove cache usage, revert to direct DB queries (1-line change).

### Risks
| Risk | Impact | Mitigation |
|------|--------|------------|
| Stale cache | Low | 5min TTL + invalidation on updates |
| Memory usage | Low | Max 10MB for 10k models |
```

---

**Following these standards ensures all implementation plans are consistent, complete, and actionable.**
