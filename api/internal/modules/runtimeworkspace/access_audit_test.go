package runtimeworkspace

import "testing"

func TestAccessEventResultHonorsExplicitStreamFailure(t *testing.T) {
	result, err := accessEventResult(AccessEventInput{StatusCode: 200, Result: AccessEventResultFailure})
	if err != nil {
		t.Fatal(err)
	}
	if result != AccessEventResultFailure {
		t.Fatalf("result = %q, want failure", result)
	}
}

func TestAccessEventResultFallsBackToHTTPStatus(t *testing.T) {
	for _, test := range []struct {
		status int
		want   string
	}{
		{status: 200, want: AccessEventResultSuccess},
		{status: 429, want: AccessEventResultFailure},
	} {
		result, err := accessEventResult(AccessEventInput{StatusCode: test.status})
		if err != nil {
			t.Fatal(err)
		}
		if result != test.want {
			t.Fatalf("status %d result = %q, want %q", test.status, result, test.want)
		}
	}
}
