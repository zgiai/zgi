package parser

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestParsePluginFromZipFullInlineProviderTool(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipFile(t, zw, "manifest.yaml", `author: zgi
description:
  en_US: Fetch realtime hot topics.
label:
  en_US: Baidu Realtime Hot
name: baidu-realtime-hot
plugins:
  tools:
    - provider/baidu_top.yaml
type: plugin
version: 0.0.1
meta:
  runner:
    entrypoint: main_runner
    language: python
  version: 0.0.1
`)
	writeZipFile(t, zw, "provider/baidu_top.yaml", `identity:
  author: zgi
  name: baidu_top
  label:
    en_US: Baidu Top
    zh_Hans: 百度热榜
  description:
    en_US: Tools for fetching Baidu Top realtime board data.
    zh_Hans: 用于获取百度热搜实时榜数据的工具。
tools:
  - name: baidu_realtime_hot
    label:
      en_US: Baidu Realtime Hot
      zh_Hans: 百度实时热榜
    description:
      en_US: Fetch realtime hot topics.
      zh_Hans: 获取实时热搜榜。
    parameters:
      - name: limit
        type: number
        required: false
        label:
          en_US: Limit
          zh_Hans: 返回数量
        human_description:
          en_US: Number of topics to return.
          zh_Hans: 返回的热搜条数。
        form: llm
`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := ParsePluginFromZipFull(buf.Bytes())
	if err != nil {
		t.Fatalf("ParsePluginFromZipFull() error = %v", err)
	}
	if result.Manifest.Name != "baidu-realtime-hot" {
		t.Fatalf("manifest name = %q", result.Manifest.Name)
	}
	if got := len(result.Declaration.Tools); got != 1 {
		t.Fatalf("tools length = %d", got)
	}
	tool := result.Declaration.Tools[0]
	if tool.Name != "baidu_realtime_hot" {
		t.Fatalf("tool name = %q", tool.Name)
	}
	if tool.Label["zh_Hans"] != "百度实时热榜" {
		t.Fatalf("zh label = %q", tool.Label["zh_Hans"])
	}
	if tool.Description.LLM != "Baidu Realtime Hot" && tool.Description.LLM != "Fetch realtime hot topics." {
		t.Fatalf("llm description = %q", tool.Description.LLM)
	}
	if tool.Parameters[0].HumanDescription["zh_Hans"] != "返回的热搜条数。" {
		t.Fatalf("param human description = %q", tool.Parameters[0].HumanDescription["zh_Hans"])
	}
}

func writeZipFile(t *testing.T, zw *zip.Writer, name string, content string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}
