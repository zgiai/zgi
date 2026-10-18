package approval

import "testing"

func TestSubmittedDataNeverReturnsNilMap(t *testing.T) {
	null, empty := "null", ""
	for name, raw := range map[string]*string{"missing": nil, "empty": &empty, "json null": &null} {
		t.Run(name, func(t *testing.T) {
			data, err := submittedData(raw)
			if err != nil {
				t.Fatalf("submittedData() error = %v", err)
			}
			if data == nil {
				t.Fatal("submittedData() returned a nil map; the node writes its action outputs into it")
			}
			data["action_id"] = "approve"
		})
	}
	values := `{"comment":"ok"}`
	data, err := submittedData(&values)
	if err != nil || data["comment"] != "ok" {
		t.Fatalf("submittedData(values) = %v, %v", data, err)
	}
}
