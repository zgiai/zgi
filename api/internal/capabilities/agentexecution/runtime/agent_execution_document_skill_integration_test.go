//go:build agent_runtime_integration

package executionruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// This narrow normal-use gate complements, rather than repeats, the office/media
// matrix. External packages are never rewritten or committed to this repository.
func TestAgentExecutionDocumentSkills(t *testing.T) {
	root := os.Getenv("ZGI_STANDARD_SKILL_TEST_ROOT")
	if root == "" {
		t.Skip("ZGI_STANDARD_SKILL_TEST_ROOT is not set")
	}
	url, key := os.Getenv("ZGI_AGENT_RUNTIME_TEST_URL"), os.Getenv("ZGI_AGENT_RUNTIME_TEST_API_KEY")
	profile, backend := os.Getenv("ZGI_AGENT_RUNTIME_TEST_PROFILE"), os.Getenv("ZGI_AGENT_RUNTIME_TEST_BACKEND")
	if url == "" || key == "" || profile == "" || backend == "" {
		t.Fatal("dedicated Worker configuration is required")
	}
	gateway := &agentExecutionHTTPGateway{baseURL: url, apiKey: key, releaseID: os.Getenv("ZGI_AGENT_RUNTIME_TEST_RELEASE_ID"), client: &http.Client{Timeout: 90 * time.Second}}
	scope := agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	agent, conversation := uuid.NewString(), uuid.NewString()
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	binding, err := gateway.CreateBinding(ctx, scope, agent, conversation, profile, backend, "", agentExecutionPackageNetworkMode)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		latest, err := gateway.GetBinding(ctx, scope, binding.ID)
		if err != nil {
			t.Errorf("find owned test Binding: %v", err)
			return
		}
		var deleted agentExecutionBinding
		if err := gateway.request(ctx, scope, http.MethodDelete, fmt.Sprintf("bindings/%s?generation=%d", latest.ID, latest.Generation), nil, &deleted); err != nil || deleted.Status != "deleted" {
			t.Errorf("clean owned test Binding: status=%s error=%v", deleted.Status, err)
		}
	})
	if err := gateway.UploadFile(ctx, scope, binding.ID, "inputs/values.csv", []byte("item,value\nA,10\nB,20\n")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id              string
		commands, files []string
	}{
		{"docx", []string{documentSkillCreate, `python3 "$ZGI_SKILL_DIR/scripts/office/validate.py" /workspace/outputs/summary.docx`, `python3 "$ZGI_SKILL_DIR/scripts/office/soffice.py" --headless --convert-to pdf --outdir /workspace/outputs /workspace/outputs/summary.docx`, `pdftotext /workspace/outputs/summary.pdf - | grep 'Total: 30'`}, []string{"summary.docx", "summary.pdf"}},
		{"pdf", []string{pdfSkillCreate, `mkdir -p /workspace/outputs/pdf-pages; python3 "$ZGI_SKILL_DIR/scripts/convert_pdf_to_images.py" /workspace/outputs/processed.pdf /workspace/outputs/pdf-pages`, `python3 - <<'PY'
from pypdf import PdfReader
from PIL import Image
r=PdfReader('/workspace/outputs/processed.pdf'); assert len(r.pages)==2
assert all('Total: 30' in p.extract_text() for p in r.pages)
for i in (1,2):
 im=Image.open(f'/workspace/outputs/pdf-pages/page_{i}.png'); im.verify()
print('PDF_CONTENT_VERIFIED')
PY`}, []string{"processed.pdf", "pdf-pages/page_1.png", "pdf-pages/page_2.png"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			packageRoot := filepath.Join(root, tc.id)
			digest, err := skills.SkillPackageSHA256(packageRoot)
			if err != nil {
				t.Fatal(err)
			}
			resolver := func(context.Context, string) (agentSkillPackage, error) {
				return agentSkillPackage{ID: tc.id, Root: packageRoot, ExpectedDigest: digest}, nil
			}
			shell := newAgentExecutionShellToolWithNetworkPolicy(gateway, scope, agent, conversation, uuid.NewString(), profile, backend, agentExecutionPackageNetworkMode, nil, nil, resolver)
			for i, command := range tc.commands {
				args, _ := json.Marshal(map[string]any{"command": "set -eu; " + command, "skill_id": tc.id, "package_sha256": digest, "timeout_seconds": 60})
				result := shell.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(args)}})
				exit, ok := result.ModelResult["exit_code"].(*int)
				if result.Error != nil || !ok || exit == nil || *exit != 0 || result.ModelResult["status"] != "succeeded" {
					t.Fatalf("%s step %d: status=%v error=%v stdout=%v stderr=%v", tc.id, i, result.ModelResult["status"], result.Error, result.ModelResult["stdout"], result.ModelResult["stderr"])
				}
				if duration, ok := result.Result["execution_duration_ms"].(int64); !ok || duration < 0 {
					t.Fatalf("missing measured Worker duration: %v", result.Result["execution_duration_ms"])
				}
			}
			for _, name := range tc.files {
				store := &workerCSVArtifactStore{}
				export := newAgentExecutionExportTool(gateway, store, scope, agent, conversation, uuid.NewString(), profile, backend, agentExecutionPackageNetworkMode)
				args, _ := json.Marshal(map[string]string{"path": "outputs/" + name})
				result := export.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(args)}})
				if result.Error != nil || len(result.Artifacts) != 1 || len(store.data) == 0 {
					t.Fatalf("export %s: error=%v", name, result.Error)
				}
				if target := os.Getenv("ZGI_STANDARD_SKILL_TEST_OUTPUT"); target != "" {
					path := filepath.Join(target, name)
					if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, store.data, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				t.Logf("backend=%s skill=%s package=%s file=%s bytes=%d sha256=%x", backend, tc.id, digest, name, len(store.data), sha256.Sum256(store.data))
			}
		})
		if t.Failed() {
			return
		}
	}
}

const documentSkillCreate = `mkdir -p /workspace/outputs
node <<'JS'
const fs=require('fs'); const {Document,Packer,Paragraph,TextRun}=require('docx');
const rows=fs.readFileSync('/workspace/inputs/values.csv','utf8').trim().split('\n').slice(1);
const total=rows.reduce((sum,row)=>sum+Number(row.split(',')[1]),0);
const doc=new Document({sections:[{children:[new Paragraph({children:[new TextRun('ZGI-DOCUMENT-NORMAL-USE')]}),new Paragraph('Total: '+total)]}]});
Packer.toBuffer(doc).then(data=>fs.writeFileSync('/workspace/outputs/summary.docx',data)).catch(err=>{console.error(err);process.exitCode=1});
JS`

const pdfSkillCreate = `mkdir -p /workspace/outputs
python3 - <<'PY'
import csv,os
from reportlab.pdfgen import canvas
from pypdf import PdfReader,PdfWriter
assert os.getuid()==65532
total=sum(int(r['value']) for r in csv.DictReader(open('/workspace/inputs/values.csv')))
c=canvas.Canvas('/workspace/inputs/source.pdf')
for i in (1,2):
 c.drawString(72,720,f'ZGI-PDF-NORMAL-USE page {i} Total: {total}'); c.showPage()
c.save()
r=PdfReader('/workspace/inputs/source.pdf'); w=PdfWriter()
for p in reversed(r.pages): w.add_page(p)
with open('/workspace/outputs/processed.pdf','wb') as f: w.write(f)
PY`
