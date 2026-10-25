package agentmemory

import "testing"

func TestQuotedInIgnoresSpacingPunctuationAndCase(t *testing.T) {
	message := "请记住以下信息：1. 我的车是一辆白色的比亚迪汉；2. I play badminton on Tuesdays."
	for _, quote := range []string{"我的车是一辆白色的比亚迪汉", "我的车 是一辆白色的比亚迪汉。", "1 我的车是一辆白色的比亚迪汉", "i play Badminton, on tuesdays"} {
		if !QuotedIn(quote, "", message) {
			t.Fatalf("%q was not found in the message", quote)
		}
	}
	for _, quote := range []string{"我的车是一辆黑色的比亚迪汉", "", "。", "车"} {
		if QuotedIn(quote, message) {
			t.Fatalf("%q was accepted as a quote", quote)
		}
	}
}
