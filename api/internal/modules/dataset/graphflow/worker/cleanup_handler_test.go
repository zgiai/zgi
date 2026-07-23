package worker

import "testing"

func TestCleanupGarbageCollectionUsesRemainingEvidence(t *testing.T) {
	tests := []struct {
		name                   string
		remainingEvidence      int
		remainingRelationships int
		deleteRelationship     bool
		deleteEntity           bool
	}{
		{name: "shared fact", remainingEvidence: 1, deleteRelationship: false, deleteEntity: false},
		{name: "zero evidence relationship", remainingEvidence: 0, remainingRelationships: 1, deleteRelationship: true, deleteEntity: false},
		{name: "orphan entity", remainingEvidence: 0, remainingRelationships: 0, deleteRelationship: true, deleteEntity: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := planEvidenceGarbageCollection(tt.remainingEvidence, tt.remainingRelationships)
			if plan.DeleteRelationship != tt.deleteRelationship || plan.DeleteEntity != tt.deleteEntity {
				t.Fatalf("plan=%#v", plan)
			}
		})
	}
}
