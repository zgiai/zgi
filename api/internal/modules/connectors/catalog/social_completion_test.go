package catalog

import (
	"github.com/santhosh-tekuri/jsonschema/v6"
	"testing"
)

func TestSocialCompletionOperations(t *testing.T) {
	for _, d := range SocialMCPDefinitions() {
		found := map[string]Operation{}
		for _, op := range d.Operations {
			found[op.ID] = op
		}
		expected := map[string][]string{"x-social": {"comment.create", "post.search", "mention.list", "post.repost", "post.quote"}, "linkedin-social": {"comment.create", "notification.list", "post.repost"}, "facebook-social": {"comment.create", "mention.list", "webhook.subscribe"}}
		for _, id := range expected[d.ID] {
			op, ok := found[d.ID+"."+id]
			if !ok {
				t.Errorf("missing %s.%s", d.ID, id)
				continue
			}
			if id == "comment.create" && (op.Effect != OperationEffectExternalSend || op.RiskHint != OperationRiskHintHigh || op.Idempotent) {
				t.Errorf("unsafe comment contract: %+v", op)
			}
			if id == "post.repost" || id == "post.quote" {
				if op.Effect != OperationEffectPublish || op.Idempotent {
					t.Errorf("unsafe publishing contract: %+v", op)
				}
			}
		}
		if d.PluginVersion != "1.1.0" {
			t.Errorf("version %s", d.PluginVersion)
		}
	}
}

func TestSocialCompletionInputBoundaries(t *testing.T) {
	cases := []struct {
		provider, action string
		valid, invalid   map[string]any
	}{
		{"x-social", "post.search", map[string]any{"query": "zgi", "until_id": "123", "since": "2026-10-01T00:00:00Z"}, map[string]any{"query": "zgi", "since_id": "https://host/"}},
		{"linkedin-social", "notification.list", map[string]any{"account_id": "7", "since": "2026-10-01T00:00:00Z"}, map[string]any{"account_id": "7", "since_id": "123"}},
		{"facebook-social", "mention.list", map[string]any{"cursor": "next"}, map[string]any{"since": "2026-10-01T00:00:00Z"}},
		{"facebook-social", "webhook.subscribe", map[string]any{}, map[string]any{"subscribed_fields": []any{"arbitrary"}}},
		{"facebook-social", "post.create", map[string]any{"text": "hi", "image_data": "aGVsbG8=", "mime_type": "image/png"}, map[string]any{"text": "hi", "image_data": "aGVsbG8=", "mime_type": "image/png", "image_url": "https://cdn.public.org/a.png"}},
		{"facebook-social", "post.create", map[string]any{"text": "hi", "image_url": "https://cdn.public.org/a.png"}, map[string]any{"text": "hi", "mime_type": "image/png"}},
	}
	for _, c := range cases {
		t.Run(c.provider+"."+c.action, func(t *testing.T) {
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("urn:test", socialInput(c.provider, c.action)); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile("urn:test")
			if err != nil {
				t.Fatal(err)
			}
			if err = schema.Validate(c.valid); err != nil {
				t.Errorf("valid input: %v", err)
			}
			if schema.Validate(c.invalid) == nil {
				t.Error("invalid input accepted")
			}
		})
	}
	for _, d := range SocialMCPDefinitions() {
		for _, op := range d.Operations {
			if d.ID != "x-social" && (op.ID == d.ID+".post.search" || op.ID == d.ID+".post.quote") {
				t.Errorf("unsupported operation %s", op.ID)
			}
		}
	}
}
