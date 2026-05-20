package llm_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zgiai/ginext/internal/modules/llm/shared/types"
)

func TestJSONArray_Value(t *testing.T) {
	t.Run("nil array returns empty array", func(t *testing.T) {
		var arr types.JSONArray
		val, err := arr.Value()
		assert.NoError(t, err)
		assert.Equal(t, "[]", val)
	})

	t.Run("non-empty array returns JSON", func(t *testing.T) {
		arr := types.JSONArray{"a", "b", "c"}
		val, err := arr.Value()
		assert.NoError(t, err)

		var result []string
		err = json.Unmarshal(val.([]byte), &result)
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})
}

func TestJSONArray_Scan(t *testing.T) {
	t.Run("nil value returns nil array", func(t *testing.T) {
		var arr types.JSONArray
		err := arr.Scan(nil)
		assert.NoError(t, err)
		assert.Nil(t, arr)
	})

	t.Run("valid JSON bytes returns array", func(t *testing.T) {
		var arr types.JSONArray
		err := arr.Scan([]byte(`["x", "y", "z"]`))
		assert.NoError(t, err)
		assert.Equal(t, types.JSONArray{"x", "y", "z"}, arr)
	})
}

func TestJSONObject_Value(t *testing.T) {
	t.Run("nil object returns empty object", func(t *testing.T) {
		var obj types.JSONObject
		val, err := obj.Value()
		assert.NoError(t, err)
		assert.Equal(t, "{}", val)
	})

	t.Run("non-empty object returns JSON", func(t *testing.T) {
		obj := types.JSONObject{"key": "value", "num": 42}
		val, err := obj.Value()
		assert.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(val.([]byte), &result)
		assert.NoError(t, err)
		assert.Equal(t, "value", result["key"])
	})
}

func TestJSONObject_Scan(t *testing.T) {
	t.Run("nil value returns nil object", func(t *testing.T) {
		var obj types.JSONObject
		err := obj.Scan(nil)
		assert.NoError(t, err)
		assert.Nil(t, obj)
	})

	t.Run("valid JSON bytes returns object", func(t *testing.T) {
		var obj types.JSONObject
		err := obj.Scan([]byte(`{"foo": "bar"}`))
		assert.NoError(t, err)
		assert.Equal(t, "bar", obj["foo"])
	})
}

func TestLocalizedString_Get(t *testing.T) {
	t.Run("returns value for exact locale", func(t *testing.T) {
		ls := types.LocalizedString{
			"en_US":   "Hello",
			"zh_Hans": "你好",
		}
		assert.Equal(t, "Hello", ls.Get("en_US"))
		assert.Equal(t, "你好", ls.Get("zh_Hans"))
	})

	t.Run("falls back to en_US if locale not found", func(t *testing.T) {
		ls := types.LocalizedString{
			"en_US": "Hello",
		}
		assert.Equal(t, "Hello", ls.Get("ja_JP"))
	})

	t.Run("returns empty string for nil map", func(t *testing.T) {
		var ls types.LocalizedString
		assert.Equal(t, "", ls.Get("en_US"))
	})
}

func TestLocalizedString_Set(t *testing.T) {
	t.Run("sets value for locale", func(t *testing.T) {
		var ls types.LocalizedString
		ls.Set("en_US", "Hello")
		assert.Equal(t, "Hello", ls["en_US"])
	})

	t.Run("overwrites existing value", func(t *testing.T) {
		ls := types.LocalizedString{"en_US": "Hi"}
		ls.Set("en_US", "Hello")
		assert.Equal(t, "Hello", ls["en_US"])
	})
}

func TestLocalizedString_Value(t *testing.T) {
	t.Run("nil returns empty object", func(t *testing.T) {
		var ls types.LocalizedString
		val, err := ls.Value()
		assert.NoError(t, err)
		assert.Equal(t, "{}", val)
	})

	t.Run("non-empty returns JSON", func(t *testing.T) {
		ls := types.LocalizedString{"en_US": "Hello"}
		val, err := ls.Value()
		assert.NoError(t, err)

		var result map[string]string
		err = json.Unmarshal(val.([]byte), &result)
		assert.NoError(t, err)
		assert.Equal(t, "Hello", result["en_US"])
	})
}

func TestLocalizedString_Scan(t *testing.T) {
	t.Run("nil value returns nil", func(t *testing.T) {
		var ls types.LocalizedString
		err := ls.Scan(nil)
		assert.NoError(t, err)
		assert.Nil(t, ls)
	})

	t.Run("valid JSON bytes returns map", func(t *testing.T) {
		var ls types.LocalizedString
		err := ls.Scan([]byte(`{"en_US": "Hello", "zh_Hans": "你好"}`))
		assert.NoError(t, err)
		assert.Equal(t, "Hello", ls["en_US"])
		assert.Equal(t, "你好", ls["zh_Hans"])
	})
}

func TestEnums(t *testing.T) {
	t.Run("Scope values", func(t *testing.T) {
		assert.Equal(t, types.Scope("system"), types.ScopeSystem)
		assert.Equal(t, types.Scope("tenant"), types.ScopeTenant)
	})

	t.Run("ModelType values", func(t *testing.T) {
		assert.Equal(t, types.ModelType("llm"), types.ModelTypeLLM)
		assert.Equal(t, types.ModelType("text-embedding"), types.ModelTypeEmbedding)
		assert.Equal(t, types.ModelType("image"), types.ModelTypeImage)
	})

	t.Run("RouteType values", func(t *testing.T) {
		assert.Equal(t, types.RouteType("ZGI_CLOUD"), types.RouteTypeZGICloud)
		assert.Equal(t, types.RouteType("PRIVATE"), types.RouteTypePrivate)
	})

	t.Run("LoadBalanceStrategy values", func(t *testing.T) {
		assert.Equal(t, types.LoadBalanceStrategy("round_robin"), types.LoadBalanceRoundRobin)
		assert.Equal(t, types.LoadBalanceStrategy("random"), types.LoadBalanceRandom)
		assert.Equal(t, types.LoadBalanceStrategy("weighted"), types.LoadBalanceWeighted)
	})
}
