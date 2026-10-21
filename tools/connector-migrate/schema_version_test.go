package main

import "testing"

func TestHistoricalConverterRejectsSchema2BeforePlanningWrites(t *testing.T) {
	source := fixture(t)
	source["connector_schema_readiness"][0]["schema_version"] = 2
	source["connector_schema_readiness"][0]["state"] = "ready"
	plan, err := makePlan(source, testKeys())
	if err == nil || len(plan.Mutations) != 0 {
		t.Fatal("schema2 accepted by historical converter")
	}
}
