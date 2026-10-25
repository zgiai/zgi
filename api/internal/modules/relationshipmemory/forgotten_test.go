package relationshipmemory

import "testing"

func TestRepeatsForgottenMatchesTheLineOrMostOfItsWords(t *testing.T) {
	forgotten := []string{"住在上海浦东。", "Drinks oat milk."}
	for text, want := range map[string]bool{
		"请记住：我住在上海浦东。":                    true,
		"好的，我记下了：你住在上海浦东新区。":              true,
		"Remember that I drink oat milk.": true,
		"我住在杭州。":                          false,
		"Do you like soy milk?":           false,
		"":                                false,
	} {
		if got := RepeatsForgotten(text, forgotten); got != want {
			t.Fatalf("RepeatsForgotten(%q) = %v, want %v", text, got, want)
		}
	}
}
