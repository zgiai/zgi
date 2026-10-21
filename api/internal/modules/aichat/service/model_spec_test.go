//go:build legacy_aichat_service
// +build legacy_aichat_service

package service

import "testing"

func TestModelSpecSupportsVisionFromFlag(t *testing.T) {
	spec := ModelSpec{Vision: true}

	if !spec.SupportsVision() {
		t.Fatal("SupportsVision() = false, want true when Vision is true")
	}
}

func TestModelSpecSupportsVisionFromUseCase(t *testing.T) {
	spec := ModelSpec{UseCases: []string{"vision"}}

	if !spec.SupportsVision() {
		t.Fatal("SupportsVision() = false, want true when use_cases contains vision")
	}
}

func TestModelSpecSupportsVisionFalse(t *testing.T) {
	spec := ModelSpec{}

	if spec.SupportsVision() {
		t.Fatal("SupportsVision() = true, want false")
	}
}
