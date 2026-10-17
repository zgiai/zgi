package migrations

// Exact pre-retirement migration sources. These exceptions do not authorize new
// destructive migrations or changes to historical files. Hashes normalize CRLF.
var historicalMigrationSourceDigests = map[string]string{
	// This migration predates the Connector execution repair and already ships
	// its narrowly scoped cleanup transition. Pin the exact source so the
	// safety checker audits future edits instead of treating it as new SQL.
	"20260906120000_messaging_auth_reliability":                   "b92996f68db0c5f7224605ad563a2a72b68913284f6c3b7d3fb4103883ee0743",
	"20260714090000_add_chat_runtime_run_lease":                     "8c87ddcdff2f9ec07a45a57a7e5742aa8975474c910ac512e5496eac24a5d315",
	"20260715100000_add_agent_published_version_name":               "69224ad7532769ff35dc0e18cf1aa354289167ca766d900d231ee87816b82edc",
	"20260715110000_add_agent_memory_slot_name":                     "203adfe4a7f468a66b9064df9060b0466babc413b69458f8b7bb09ccc221964e",
	"20260717090000_add_workflow_runtime_v2":                        "848ef87054b46e3b1d942af227141103386f29be9f47e237621ebcc71f7f1e1d",
	"20260719100000_add_workflow_conversation_runtime":              "5895f0f90a938808c7e22fa0d958870b534f7ddb6ae6f7524551b3091b8c88e4",
	"20260720120000_add_workflow_invocation_context":                "fc5ef83dafb97b53c8327797047e5ddbd346660559a494ba3f6c259229651bf2",
	"20260721123000_extend_agent_resource_bindings_integrations":    "9411d8d7852db85b40fd2013b00f62fda679ecf68b959bacedb841da2f77cd2a",
	"20260722090000_close_integration_connection_security_health":   "75b774dd342fa09f96c00eb4b666c7407242b5563b7ab85421e510a196502d02",
	"20260722090000_restore_knowledge_graph":                        "820b4810797a7dfebd2e07bedcb7e990e00842c239930442fe0d2b47b74da84c",
	"20260723100000_create_integration_oauth_flows_and_clients":     "474e9bd4056b3d366530e29ad265701280e512007d89349c20db3ce4bc01aa70",
	"20260723120000_add_integration_oauth_refresh_token_expiry":     "6bde27cbd66dc86418a5b19694811749d6ab5562711fb8f41ded740510be3a14",
	"20260723130000_add_integration_oauth_browser_binding":          "a659bebfd1391a6e36aaec7a61bdc006b2b465ea6f35f778e6e4b49e7420b2bb",
	"20260723140000_create_integration_oauth_recovery_outbox":       "8d92a9ba4df511b1cfc903d14d3a2d2ecec2347793c105a588ad79c4bec818a9",
	"20260723150000_add_integration_oauth_recovery_acknowledgement": "c4739a25f2a8a97da5901e33dbbcee72d65df3a2802e6121edf6fe217668102f",
	"20260723160000_redact_acknowledged_oauth_recovery_payload":     "104170e9868062c71ef5f101d2bfb9928bfdf273c6de8ecda031659ccb594ddc",
	"20260726100000_reconcile_agent_conversation_dialogue_count":    "a13615eec7b0a45c86a7b8ef4811d77d49d303029ea7995275bedf1b03a4383d",
	"20260804090000_add_graphflow_sync_batches":                     "a76e87cf0f566d4d7679dd078045864af9cd3fd912f310dd3dd32e19d28e5a78",
	"20260804100000_add_integration_connection_setup_completion":    "a3c9053d112f8f0ded6603d758f6a11feece50b27e8715c548878feb925bbee2",
	"20260804120000_serialize_graphflow_runs":                       "5fe3ab657e6f5650f147541707b9d4bb49fe3c24b46cba6e92578b89c791bdc2",
	"20260809123000_fix_workspace_role_template_lifecycle":          "5ceccab087de0ddebf526372406a81175a0cfc276d499f8e7706ddec613eec8d",
	"20260809124500_harden_workspace_role_template_lifecycle":       "70882e5b2cca4aa4c8a79f56a51b0041e72d5d96155ad630ca8362255229ed5e",
	"20260809130000_repair_workspace_role_template_localization":    "9a12426309fda57f9e3f2dd57bf60d9cc9e9d3dbbf289655690d41f50dfa9f23",
	"20260809170000_update_seedance_video_reference_metadata":       "70f0190ab2c779b9b6385ce8c12f59e26786d85a78cc6a0e35260de9ef1ab350",
	"20260810100000_add_seedance_video_resolution_metadata":         "593d0686eb998fd049028cd45378b49d4433f45ec6361a01b4c7a23965dbfe32",
	"20260810153000_add_integration_action_permission_evidence":     "83f5686fe5e6c2607809dfa1ba3c46c348fc66004741d5631093bc0e6ef8c5a0",
	"20260813120000_upgrade_agent_memory_runtime":                   "f5984b1335036a36f9cf11457e404e975538e7bfcb6fd03e28eb2433348096c1",
	"20260816100000_add_provider_install_connections":               "47466efba691392e865ac9feb991c40e1195fca06c8fe4b279a689f39c0c740f",
	"20260823100000_add_llm_cache_usage_billing":                    "7621daf1827bf3d3bc97ff7f96b9ef05d540ab116a8ef32b052c10db2bb79e32",
	"20260825123000_backfill_visible_image_runtime_tasks":           "04fd4b337a3e29e5255ac9b9b1a19fb64bce95df708b7b9707c435b5e6016f0e",
	"20260827120000_extend_agent_resource_binding_consumers":        "60249b3249629d0f6caf82801c9c0ca36703d3c88fea842c8f2af3fa58b67950",
	"20260901090000_create_connector_foundation":                    "d3b3615f983f55a2d2e440643e25772575381bff54e0c9350a4aa8e7d57475aa",
	"20260901100000_harden_connector_agent_binding_projection":      "8965d700f6edddaedd7af01f39173ab8285018cfc14f58f7ff38908d78ef3d87",
	"20260902100000_create_connector_plugin_catalog_state":          "41afd18a67842e98adb623add238c6e52cb36d6a4f2c59ed941f27d92a5a2ef9",
	"20260902110000_create_connector_migration_rollout_state":       "6f3decd61b995a7ebd8fba6f71722cd1e5598f3c4f12cbab50416d8d9b459cee",
	"20260902120000_create_connector_confirmation_continuations":    "ff2237b73b7d17a7c3886a5ac20651fa3cb3b0e7676c7fd2213961d1244aa705",
	"20260902130000_create_connector_platform_lifecycle_events":     "82e3eae54ab725a17a7f681a38279685a464db509779581e4711d0676f34b7d7",
	"20260902140000_add_connector_route_rollout_snapshot":           "796bdcedd26d9ffe39f751e3abbf3568d5f0d06357dc311c1a8d59f9c5b2ab8d",
	"20260902150000_create_connector_reversal_baselines":            "9b9e8c5a88db0c6d32fd45ee7b2cad9d4355b5f4c465cdaeedccf6a2fdfe0426",
	"202609021600000000_create_registration_provisioning_outbox":    "1b1e7b9b4e8e61376be742fbfd7a01ab0b7e195ba5fc209b97cf0ab50cc9508c",
	"20260902170000_guard_connector_migration_sources":              "b4bd0dbf319f029281710bd9779850f3235c1e1b98dcfe937d25167635ffc1f2",
	"20260902190000_detach_connector_runtime":                       "86a174984da3ab731f03ca7cb14d317e943c4db34f7928dbc089f62c5dabfab8",
	"20260903100000_create_connector_chat_preferences":              "0c8454ee099b54f48220d13ed2c0e5547248340e817240dad5fbb5c3cc571b04",
	"20260903110000_create_connector_onboarding":                    "a181bd215fd4ce6eaf66911e51bf9976c3dcaab4af62999ffbbb518076392047",
	"20260903120000_messaging_connector_protocol":                   "ed9b3d60b9d21a5b320a2391579762087544e826bcd46b542f4966b5c438bd52",
}

// Reviewed checkpoint-aware native locking; the fixed retirement DDL is unchanged.
const retirementSchemaSourceDigest = "95da917ea96ee454f26f5fb98a8cbb889dfa0c237721e8c783575b070dec5bd7"
