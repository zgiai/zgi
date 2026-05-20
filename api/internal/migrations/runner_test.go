package migrations

import "testing"

func TestAllMigrationsIncludesCredentialsCompatibility(t *testing.T) {
	targetID := "20260306000124"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesRouteCredentialFKCompatibility(t *testing.T) {
	targetID := "20260306000125"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesSubscriptionPlanSeedBackfill(t *testing.T) {
	targetID := "20260306000126"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesOrganizationAPIKeysCompatibility(t *testing.T) {
	targetID := "20260306000127"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesChannelWalletCompatibility(t *testing.T) {
	targetID := "20260306000128"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesOfficialRouteConstraintCompatibility(t *testing.T) {
	targetID := "20260306000129"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesOfficialModelSnapshot(t *testing.T) {
	targetID := "20260306000130"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesWorkspaceAndGatewayKeyStatisticsCompatibility(t *testing.T) {
	targetID := "20260309000131"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesToolFilesSchemaAlignment(t *testing.T) {
	targetID := "202603150134"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesAccountSSOIdentityFields(t *testing.T) {
	targetID := migration0135ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesSupportedParametersNormalization(t *testing.T) {
	targetID := migration0137ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesCustomModelsRuntimeCompatibilityFix(t *testing.T) {
	targetID := migration0138ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesLLMForeignKeyDrop(t *testing.T) {
	targetID := migration0144ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesAccountSoftDelete(t *testing.T) {
	targetID := migration0145ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesBootstrapLocks(t *testing.T) {
	targetID := migration0146ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsDoesNotRegisterExcelImportJobs(t *testing.T) {
	const targetID = "20260513000149"

	for _, m := range allMigrations() {
		if m.ID == targetID {
			t.Fatalf("legacy migrations must not register Excel import migration %s", targetID)
		}
	}
}

func TestAllMigrationsIncludesContentParseChunkArtifactSets(t *testing.T) {
	targetID := migration0157ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesDataLibraryDocumentAssets(t *testing.T) {
	targetID := migration0158ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesDataLibraryReuseEvents(t *testing.T) {
	targetID := migration0159ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesDataLibraryVectorArtifacts(t *testing.T) {
	targetID := migration0161ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesDataLibraryProcessingExecutionState(t *testing.T) {
	targetID := migration0162ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesDataLibraryKnowledgeBaseAssetRefs(t *testing.T) {
	targetID := migration0163ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesDataLibraryDatabaseAssetRefs(t *testing.T) {
	targetID := migration0164ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}

func TestAllMigrationsIncludesDataLibraryExtractionArtifacts(t *testing.T) {
	targetID := migration0165ID

	for _, m := range allMigrations() {
		if m.ID == targetID {
			return
		}
	}

	t.Fatalf("expected migration %s to be registered in allMigrations()", targetID)
}
