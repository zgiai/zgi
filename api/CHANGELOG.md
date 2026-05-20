# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added (2026-01-27)

#### Provider API Enhancement - ModelMeta Integration
- **Migration M0094**: Align `llm_providers` table with ModelMeta API schema
  - Added 5 new fields: `website`, `pricing_url`, `tagline`, `country_code`, `founded_year`
  - Created index on `country_code` for optimized filtering
  - All new fields are nullable for backward compatibility

- **Data Synchronization**
  - Implemented ModelMeta API sync script (`cmd/sync_modelmeta/main.go`)
  - Smart incremental updates (only non-empty values)
  - Synced 19 providers with 95% data completeness
  - Support for multi-language taglines and descriptions via `metadata.i18n`

- **Code Updates**
  - Updated `Provider` model with new ModelMeta fields
  - Enhanced `CreateProviderRequest` and `UpdateProviderRequest` DTOs
  - Updated service layer to handle new fields in create/update operations
  - Optimized config validation to allow utility scripts to run

- **Documentation**
  - Complete Provider API documentation with v1.1 updates
  - API use case guide with frontend integration examples
  - Migration impact assessment and verification report
  - Sync script optimization guide

#### New Features
- Provider website and pricing page links in API responses
- Country/region identification for geographic filtering
- Founded year for timeline displays
- Brand taglines with internationalization support
- Enhanced frontend capabilities:
  - Provider marketplace with country filters
  - Geographic distribution visualization
  - Timeline displays by founded year
  - Multi-language tagline support

### Changed
- Config validation made more flexible for utility scripts
  - R2 config now optional (only validates when credentials provided)
  - DB_PASSWORD validation removed for development mode
  - JWT secret validation only in production mode

### Technical Details
- **Database**: PostgreSQL migration executed successfully
- **Data Quality**: 95% field completion rate (19/20 providers)
- **Backward Compatibility**: 100% (all new fields use `omitempty`)
- **Test Coverage**: Database schema, data integrity, API responses verified
- **Risk Level**: Low (zero breaking changes)

### Migration Notes
- Migration ID: `20260127000094`
- Execution Date: 2026-01-27
- Rollback: Simple DROP COLUMN operations available
- Impact: Zero downtime, no frontend updates required

---

## [Previous Versions]

(Add previous changelog entries here)
