package skills

import "testing"

func TestTimeSkillDefaultsToTheUsersTimezoneAndNamesIt(t *testing.T) {
	arguments := withTimeSkillDefaults("current_time", map[string]interface{}{}, "Asia/Shanghai")
	if arguments["timezone"] != "Asia/Shanghai" || arguments["format"] != timeSkillDefaultFormat {
		t.Fatalf("defaults = %#v", arguments)
	}
	kept := withTimeSkillDefaults("current_time", map[string]interface{}{"timezone": "Europe/Paris", "format": "%H:%M"}, "Asia/Shanghai")
	if kept["timezone"] != "Europe/Paris" || kept["format"] != "%H:%M" {
		t.Fatalf("model arguments were replaced: %#v", kept)
	}
	if unknown := withTimeSkillDefaults("date_calculate", map[string]interface{}{}, "Not/AZone"); unknown["timezone"] != nil || unknown["format"] != nil {
		t.Fatalf("an unknown timezone or a date tool format was set: %#v", unknown)
	}
}
