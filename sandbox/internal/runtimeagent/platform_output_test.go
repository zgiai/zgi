package runtimeagent

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestPlatformOutputRedactsAcrossWritesAndFlushesBothStreams(t *testing.T) {
	token := strings.Repeat("x", 43)
	for split := 0; split <= len(token); split++ {
		var out, diagnostic strings.Builder
		output := newPlatformOutput(token, func(stream agentexecution.OutputStream, data []byte) error {
			if stream == agentexecution.OutputStreamStderr {
				diagnostic.Write(data)
			} else {
				out.Write(data)
			}
			return nil
		})
		for _, stream := range []agentexecution.OutputStream{agentexecution.OutputStreamStdout, agentexecution.OutputStreamStderr} {
			if err := output.Write(stream, []byte("before"+token[:split])); err != nil {
				t.Fatal(err)
			}
			if err := output.Write(stream, []byte(token[split:]+"after")); err != nil {
				t.Fatal(err)
			}
		}
		if err := output.Flush(); err != nil {
			t.Fatal(err)
		}
		if out.String() != "before[redacted-job-grant]after" || diagnostic.String() != out.String() {
			t.Fatalf("split=%d output=%q diagnostic=%q", split, out.String(), diagnostic.String())
		}
		if err := output.Flush(); err != nil || out.String() != "before[redacted-job-grant]after" {
			t.Fatal("flush replayed output")
		}
	}
}
