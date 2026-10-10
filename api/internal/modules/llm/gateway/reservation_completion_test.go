package gateway

import "testing"

func TestReservationCompletionTokens(t *testing.T) {
	capped := &ProviderSelection{}
	capped.Model.MaxOutputTokens = 8192

	explicit := 100
	if got := reservationCompletionTokens(capped, &explicit, explicit); got != explicit {
		t.Fatalf("explicit max_tokens reservation = %d, want %d", got, explicit)
	}

	if got := reservationCompletionTokens(capped, nil, 500); got != chatCompletionReservationCap {
		t.Fatalf("catalog-bound reservation = %d, want cap %d", got, chatCompletionReservationCap)
	}

	small := &ProviderSelection{}
	small.Model.MaxOutputTokens = 1500
	if got := reservationCompletionTokens(small, nil, 500); got != 1500 {
		t.Fatalf("small catalog reservation = %d, want 1500", got)
	}

	unknown := &ProviderSelection{}
	if got := reservationCompletionTokens(unknown, nil, 500); got != 500 {
		t.Fatalf("unknown catalog reservation = %d, want 500", got)
	}

	if got := reservationCompletionTokens(nil, nil, 500); got != 500 {
		t.Fatalf("nil selection reservation = %d, want 500", got)
	}
}
