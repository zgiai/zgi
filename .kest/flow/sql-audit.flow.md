# SQL Audit End-to-End Contract

This flow logs in with a supplied test account, creates a datasource, creates a
table and writable column, writes one record, then verifies SQL audit behavior
from both datasource and workspace audit APIs.

Required variables:

- Kest `--base-url`:
  - use `http://localhost:3000` when running through the Next.js web dev server
  - use `http://127.0.0.1:2679` only when running directly against the API server
- `run_id`, a unique value for the test run, for example `sql_audit_001`
- `login_email`, an existing account email
- `login_password`, the account password

API test environment prerequisites:

- latest API migrations have been applied; this flow requires the SQL audit
  migration that extends `data_source_sql_operations`
- the supplied account can access at least one workspace with database
  management permission
- when using the web dev server as `--base-url`, `LOCAL_API_PROXY_TARGET` must point to the API server

```flow
@flow id=zgi-sql-audit
@name ZGI SQL audit datasource contract
```

```step
@id login
@name Login and capture auth context

POST /console/api/login
Content-Type: application/json
Accept: application/json

{
  "email": "{{login_email}}",
  "password": "{{login_password}}",
  "remember_me": true,
  "language": "en-US"
}

[Captures]
auth_token = data.data.access_token
account_id = data.data.account.id

[Asserts]
status == 200
code == "0"
data.result == "success"
data.data.access_token != ""
data.data.account.id != ""
```

```step
@id get-current-organization
@name Get current organization

GET /console/api/organizations/current
Accept: application/json
Authorization: Bearer {{auth_token}}

[Captures]
organization_id = data.id

[Asserts]
status == 200
code == "0"
data.id != ""
```

```step
@id get-managed-workspace
@name Get a workspace with database management permission

GET /console/api/organizations/{{organization_id}}/managed-workspaces?page=1&limit=1
Accept: application/json
Authorization: Bearer {{auth_token}}

[Captures]
workspace_id = data.data.0.id

[Asserts]
status == 200
code == "0"
data.data.0.id != ""
```

```step
@id create-datasource
@name Create datasource for SQL audit test

POST /console/api/data-dbs
Content-Type: application/json
Accept: application/json
Authorization: Bearer {{auth_token}}

{
  "name": "kest_sql_audit_ds_{{run_id}}",
  "description": "Kest SQL Audit E2E datasource",
  "permission": "only_me",
  "workspace_id": "{{workspace_id}}"
}

[Captures]
data_source_id = data.id

[Asserts]
status == 200
code == "0"
data.id != ""
data.name == "kest_sql_audit_ds_{{run_id}}"
```

```step
@id create-table
@name Create table and generate DDL audit rows

POST /console/api/data-dbs/{{data_source_id}}/tables
Content-Type: application/json
Accept: application/json
Authorization: Bearer {{auth_token}}

{
  "name": "kest_sql_audit_table_{{run_id}}",
  "description": "Kest SQL Audit E2E table"
}

[Captures]
table_id = data.id

[Asserts]
status == 200
code == "0"
data.id != ""
data.name == "kest_sql_audit_table_{{run_id}}"
```

```step
@id create-test-column
@name Add writable text column and generate DDL audit row

PUT /console/api/data-dbs/{{data_source_id}}/tables/{{table_id}}/columns
Content-Type: application/json
Accept: application/json
Authorization: Bearer {{auth_token}}

{
  "columns": [
    {
      "name": "audit_value",
      "type": "varchar",
      "is_required": false,
      "description": "Writable column for SQL Audit Kest flow"
    }
  ]
}

[Asserts]
status == 200
code == "0"
```

```step
@id add-table-record
@name Add table record and generate DML audit row

POST /console/api/data-dbs/{{data_source_id}}/tables/{{table_id}}/records
Content-Type: application/json
Accept: application/json
Authorization: Bearer {{auth_token}}

{
  "records": [
    {
      "audit_value": "kest_sql_audit_{{run_id}}"
    }
  ]
}

[Asserts]
status == 200
code == "0"
data.affected_rows >= 0
```

```step
@id wait-for-async-audit-recorder
@name Wait for async SQL audit flush
@type exec

sleep 1
```

```step
@id list-datasource-create-audit
@name List datasource SQL operation logs filtered to inserted row

GET /console/api/data-dbs/{{data_source_id}}/sql-operations?page=1&limit=10&table_id={{table_id}}&created_by={{account_id}}&operation_type=create&status=success
Accept: application/json
Authorization: Bearer {{auth_token}}

[Asserts]
status == 200
code == "0"
data.page == 1
data.limit == 10
data.total >= 1
data.data.0.data_source_id == "{{data_source_id}}"
data.data.0.table_id == "{{table_id}}"
data.data.0.operation_type == "create"
data.data.0.status == "success"
data.data.0.created_by == "{{account_id}}"
data.data.0.sql_statement != ""
```

```step
@id list-workspace-sql-audit
@name List workspace SQL audit rows filtered to inserted row

GET /console/api/workspaces/{{workspace_id}}/sql-audit?page=1&limit=10&data_source_id={{data_source_id}}&table_id={{table_id}}&client_type=api&created_by={{account_id}}&operation_type=create&status=success
Accept: application/json
Authorization: Bearer {{auth_token}}

[Captures]
sql_audit_operation_id = data.data.0.id

[Asserts]
status == 200
code == "0"
data.page == 1
data.limit == 10
data.total >= 1
data.data.0.id != ""
data.data.0.organization_id == "{{organization_id}}"
data.data.0.workspace_id == "{{workspace_id}}"
data.data.0.data_source_id == "{{data_source_id}}"
data.data.0.table_id == "{{table_id}}"
data.data.0.client_type == "api"
data.data.0.operation_type == "create"
data.data.0.status == "success"
data.data.0.row_count >= 0
data.data.0.duration_ms >= 0
data.data.0.created_by == "{{account_id}}"
data.data.0.executed_at != ""
```

```step
@id get-workspace-sql-audit-detail
@name Get workspace SQL audit detail

GET /console/api/workspaces/{{workspace_id}}/sql-audit/{{sql_audit_operation_id}}
Accept: application/json
Authorization: Bearer {{auth_token}}

[Asserts]
status == 200
code == "0"
data.id == "{{sql_audit_operation_id}}"
data.organization_id == "{{organization_id}}"
data.workspace_id == "{{workspace_id}}"
data.data_source_id == "{{data_source_id}}"
data.table_id == "{{table_id}}"
data.client_type == "api"
data.operation_type == "create"
data.status == "success"
data.row_count >= 0
data.duration_ms >= 0
data.sql_statement != ""
data.params_json != null
data.start_time != ""
data.end_time != ""
```

```step
@id list-workspace-update-audit
@name List workspace SQL audit rows for column update

GET /console/api/workspaces/{{workspace_id}}/sql-audit?page=1&limit=10&data_source_id={{data_source_id}}&table_id={{table_id}}&client_type=api&created_by={{account_id}}&operation_type=update&status=success
Accept: application/json
Authorization: Bearer {{auth_token}}

[Asserts]
status == 200
code == "0"
data.total >= 1
data.data.0.workspace_id == "{{workspace_id}}"
data.data.0.data_source_id == "{{data_source_id}}"
data.data.0.table_id == "{{table_id}}"
data.data.0.client_type == "api"
data.data.0.operation_type == "update"
data.data.0.status == "success"
data.data.0.duration_ms >= 0
data.data.0.created_by == "{{account_id}}"
```
