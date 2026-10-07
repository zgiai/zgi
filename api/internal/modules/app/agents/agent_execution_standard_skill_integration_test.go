//go:build agent_runtime_integration

package agents

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// External packages remain outside Git. The same contract runs against either
// real backend; it exercises original helpers, not a rewritten Skill protocol.
func TestAgentExecutionStandardSkillMatrix(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("ZGI_STANDARD_SKILL_TEST_ROOT"))
	if root == "" {
		t.Skip("ZGI_STANDARD_SKILL_TEST_ROOT is not set")
	}
	baseURL, apiKey := os.Getenv("ZGI_AGENT_RUNTIME_TEST_URL"), os.Getenv("ZGI_AGENT_RUNTIME_TEST_API_KEY")
	profile, backend := os.Getenv("ZGI_AGENT_RUNTIME_TEST_PROFILE"), os.Getenv("ZGI_AGENT_RUNTIME_TEST_BACKEND")
	if baseURL == "" || apiKey == "" || profile == "" || backend == "" {
		t.Fatal("real Worker URL, API key, profile and backend are required")
	}
	gateway := &agentExecutionHTTPGateway{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, client: &http.Client{Timeout: 90 * time.Second}}
	scope := agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	agentID, conversationID := uuid.NewString(), uuid.NewString()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	binding, err := gateway.CreateBinding(ctx, scope, agentID, conversationID, profile, backend, "", agentExecutionPackageNetworkMode)
	if err != nil {
		t.Fatalf("create matrix binding: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cleanupCancel()
		latest, err := gateway.GetBinding(cleanupCtx, scope, binding.ID)
		if err != nil {
			t.Errorf("lookup owned matrix binding for cleanup: %v", err)
			return
		}
		var deleted agentExecutionBinding
		if err := gateway.request(cleanupCtx, scope, http.MethodDelete, fmt.Sprintf("bindings/%s?generation=%d", latest.ID, latest.Generation), nil, &deleted); err != nil {
			t.Errorf("delete owned matrix binding: %v", err)
		} else if deleted.Status != "deleted" {
			t.Errorf("cleanup status = %q, want deleted", deleted.Status)
		}
	})
	otherScope := agentExecutionScope{OrganizationID: uuid.NewString(), SubjectID: uuid.NewString()}
	_, err = gateway.GetBinding(ctx, otherScope, binding.ID)
	var gatewayErr agentExecutionGatewayHTTPError
	if !errors.As(err, &gatewayErr) || gatewayErr.statusCode != http.StatusNotFound {
		t.Fatalf("cross-organization binding access must return 404: %v", err)
	}
	if err := gateway.UploadFile(ctx, scope, binding.ID, "inputs/values.csv", []byte("item,value\nA,10\nB,20\n")); err != nil {
		t.Fatal(err)
	}
	outputs := map[string][32]byte{}
	for _, tc := range []struct {
		id       string
		commands []string
		files    []string
	}{
		{"xlsx", []string{standardMatrixSpreadsheetCreate, `python3 "$ZGI_SKILL_DIR/scripts/recalc.py" /workspace/outputs/matrix.xlsx 30`, standardMatrixSpreadsheetVerify}, []string{"matrix.xlsx"}},
		{"pptx", []string{standardMatrixPresentationCreate, `python3 "$ZGI_SKILL_DIR/scripts/office/validate.py" /workspace/outputs/matrix.pptx`, `python3 "$ZGI_SKILL_DIR/scripts/office/soffice.py" --headless --convert-to pdf --outdir /workspace/outputs /workspace/outputs/matrix.pptx`, `pdftoppm -png -scale-to 900 /workspace/outputs/matrix.pdf /workspace/outputs/slide`, standardMatrixPresentationVerify}, []string{"matrix.pptx", "matrix.pdf", "slide-1.png", "slide-2.png"}},
		{"slack-gif-creator", []string{`python3 -c 'import imageio' || pip install --no-input imageio==2.37.0`, standardMatrixAnimationCreate}, []string{"matrix.gif"}},
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
			shell := newAgentExecutionShellToolWithNetworkPolicy(gateway, scope, agentID, conversationID, uuid.NewString(), profile, backend, agentExecutionPackageNetworkMode, nil, nil, resolver)
			for index, command := range tc.commands {
				args, _ := json.Marshal(map[string]any{"command": "set -eu; " + command, "skill_id": tc.id, "package_sha256": digest, "timeout_seconds": 60})
				result := shell.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(args)}})
				code, ok := result.ModelResult["exit_code"].(*int)
				if result.Error != nil || !ok || code == nil || *code != 0 || result.ModelResult["status"] != "succeeded" {
					t.Fatalf("step %d: status=%v exit=%v error=%v stdout=%v stderr=%v", index, result.ModelResult["status"], code, result.Error, result.ModelResult["stdout"], result.ModelResult["stderr"])
				}
				if tc.id == "xlsx" && index == 1 {
					var recalc struct {
						Status string `json:"status"`
						Errors int    `json:"total_errors"`
					}
					if err := json.Unmarshal([]byte(fmt.Sprint(result.ModelResult["stdout"])), &recalc); err != nil || recalc.Status != "success" || recalc.Errors != 0 {
						t.Fatalf("recalc exit 0 alone is insufficient: output=%v error=%v", result.ModelResult["stdout"], err)
					}
				}
			}
			for _, name := range tc.files {
				store := &workerCSVArtifactStore{}
				export := newAgentExecutionExportTool(gateway, store, scope, agentID, conversationID, uuid.NewString(), profile, backend, agentExecutionPackageNetworkMode)
				args, _ := json.Marshal(map[string]string{"path": "outputs/" + name})
				result := export.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(args)}})
				if result.Error != nil || len(result.Artifacts) != 1 || len(store.data) == 0 {
					t.Fatalf("export %s failed: %#v", name, result)
				}
				outputs[name] = sha256.Sum256(store.data)
				if outputRoot := os.Getenv("ZGI_STANDARD_SKILL_TEST_OUTPUT"); outputRoot != "" {
					if err := os.MkdirAll(outputRoot, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(outputRoot, name), store.data, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				t.Logf("%s package=%s artifact=%s bytes=%d sha256=%x", backend, digest, name, len(store.data), outputs[name])
			}
		})
	}
	if t.Failed() {
		return
	}
	negativeShell := newAgentExecutionShellToolWithNetworkPolicy(gateway, scope, agentID, conversationID, uuid.NewString(), profile, backend, agentExecutionPackageNetworkMode, nil, nil)
	negative := negativeShell.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: `{"command":"set -eu; python3 -c 'raise SystemExit(7)'; echo FALSE_SUCCESS","timeout_seconds":30}`}})
	negativeCode, ok := negative.ModelResult["exit_code"].(*int)
	if negative.Error != nil || !ok || negativeCode == nil || *negativeCode != 7 || negative.ModelResult["status"] != "failed" || strings.Contains(fmt.Sprint(negative.ModelResult["stdout"]), "FALSE_SUCCESS") {
		t.Fatalf("failed script must retain exit 7 without trailing success: %#v", negative)
	}
	for _, tc := range []struct{ name, command string }{
		{"missing package", "pip install --no-input --retries=0 --timeout=5 zgi-negative-fixture-absent-20261008==0.0.1"},
		{"registry override", "pip install --no-input --index-url=https://example.com/simple zgi-negative-fixture"},
		{"global Node install", "npm install --global zod@3.25.76"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, _ := json.Marshal(map[string]any{"command": tc.command, "timeout_seconds": 30})
			result := negativeShell.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(args)}})
			code, ok := result.ModelResult["exit_code"].(*int)
			if result.Error != nil || !ok || code == nil || *code <= 0 || result.ModelResult["status"] != "failed" {
				t.Fatalf("installation must fail visibly, not time out or succeed: %#v", result)
			}
			if strings.TrimSpace(fmt.Sprint(result.ModelResult["stdout"])) == "" && strings.TrimSpace(fmt.Sprint(result.ModelResult["stderr"])) == "" {
				t.Fatal("installation failed without diagnostics")
			}
			t.Logf("%s rejected with exit %d", tc.name, *code)
		})
	}
	for _, cancelJob := range []bool{false, true} {
		t.Run(fmt.Sprintf("termination-canceled-%t", cancelJob), func(t *testing.T) {
			// The existing Worker contract represents timeouts as failed/-1.
			timeout, expected := time.Second, "failed"
			if cancelJob {
				timeout, expected = 30*time.Second, "canceled"
			}
			marker := fmt.Sprintf("outputs/termination-%t.txt", cancelJob)
			job, err := gateway.SubmitJob(ctx, scope, binding.ID, uuid.NewString(), "/bin/sh", []string{"-c", "(sleep 3; echo leaked > " + marker + ") & wait"}, timeout)
			if err != nil {
				t.Fatal(err)
			}
			pollCtx, pollCancel := context.WithTimeout(ctx, 15*time.Second)
			defer pollCancel()
			cancellationRequested := false
			for job.Status == "queued" || job.Status == "running" {
				if cancelJob && !cancellationRequested && job.Status == "running" {
					if err := gateway.CancelJob(pollCtx, scope, job.ID, job.Generation); err != nil {
						t.Fatal(err)
					}
					cancellationRequested = true
				}
				select {
				case <-pollCtx.Done():
					t.Fatal("termination did not reach a terminal state")
				case <-time.After(200 * time.Millisecond):
				}
				job, err = gateway.GetJob(pollCtx, scope, job.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if job.Status != expected {
				t.Fatalf("termination status = %q, want %q", job.Status, expected)
			}
			if !cancelJob && (job.ExitCode == nil || *job.ExitCode != -1 || job.FailureCode != "job_failed") {
				t.Fatalf("timeout lost its infrastructure failure result: %+v", job)
			}
			// A killed parent alone is insufficient: its child must not write later.
			args, _ := json.Marshal(map[string]any{"command": "sleep 4; test ! -e " + marker, "timeout_seconds": 10})
			result := negativeShell.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: string(args)}})
			code, ok := result.ModelResult["exit_code"].(*int)
			if result.Error != nil || !ok || code == nil || *code != 0 || result.ModelResult["status"] != "succeeded" {
				t.Fatalf("terminated descendant wrote a delayed file: %#v", result)
			}
			t.Logf("%s retained with no delayed child write", expected)
		})
	}
	if t.Failed() {
		return
	}
	latest, err := gateway.GetBinding(ctx, scope, binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	var suspended agentExecutionBinding
	if err := gateway.request(ctx, scope, http.MethodPost, "bindings/"+binding.ID+"/suspend", map[string]int64{"generation": latest.Generation}, &suspended); err != nil {
		t.Fatal(err)
	}
	if suspended.Status != "suspended" {
		t.Fatalf("checkpoint status = %q", suspended.Status)
	}
	if _, err := gateway.ResumeBinding(ctx, scope, binding.ID, suspended.Generation); err != nil {
		t.Fatal(err)
	}
	for name, expected := range outputs {
		data, err := gateway.DownloadFile(ctx, scope, binding.ID, "outputs/"+name)
		if err != nil || sha256.Sum256(data) != expected {
			t.Fatalf("resumed %s differs: error=%v", name, err)
		}
	}
	shell := newAgentExecutionShellToolWithNetworkPolicy(gateway, scope, agentID, conversationID, uuid.NewString(), profile, backend, agentExecutionPackageNetworkMode, nil, nil)
	result := shell.Handler(ctx, adapter.ToolCall{ID: uuid.NewString(), Function: adapter.FunctionCall{Arguments: `{"command":"set -eu; test $(id -u) = 65532; python3 -c 'import imageio; print(imageio.__version__)'; echo MATRIX_RETAINED","timeout_seconds":30}`}})
	code, ok := result.ModelResult["exit_code"].(*int)
	if result.Error != nil || !ok || code == nil || *code != 0 || !strings.Contains(fmt.Sprint(result.ModelResult["stdout"]), "MATRIX_RETAINED") {
		t.Fatalf("resumed import: %#v", result)
	}
	t.Logf("%s checkpoint/resume preserved %d output hashes and default Python import", backend, len(outputs))
}

const standardMatrixSpreadsheetCreate = `mkdir -p /workspace/outputs
python3 - <<'PY'
import csv
from openpyxl import Workbook
from openpyxl.styles import Font
w=Workbook();s=w.active;s.title='Values'
s.append(['Item','Value']); rows=list(csv.DictReader(open('/workspace/inputs/values.csv')))
for r in rows:s.append([r['item'],int(r['value'])])
s.append(['Total','=SUM(B2:B3)']);s['D1']='ZGI-STANDARD-REGRESSION';s['D2']='Source: test input values.csv'
for row in s:
 for c in row:c.font=Font(name='Arial',size=11)
s.column_dimensions['A'].width=18;s.column_dimensions['B'].width=14;s.column_dimensions['D'].width=42
w.save('/workspace/outputs/matrix.xlsx')
PY`

const standardMatrixSpreadsheetVerify = `python3 - <<'PY'
from openpyxl import load_workbook
p='/workspace/outputs/matrix.xlsx'
assert load_workbook(p)['Values']['B4'].value=='=SUM(B2:B3)'
assert load_workbook(p,data_only=True)['Values']['B4'].value==30
print('XLSX_FORMULA_AND_CACHE_OK')
PY`

const standardMatrixPresentationCreate = `node - <<'JS'
const fs=require('fs'),PptxGenJS=require('pptxgenjs');
const p=new PptxGenJS();p.layout='LAYOUT_WIDE';p.author='ZGI test';p.subject='Standard Skill regression';
const rows=fs.readFileSync('/workspace/inputs/values.csv','utf8').trim().split('\n').slice(1).map(s=>s.split(','));
let s=p.addSlide();s.addText('ZGI-STANDARD-REGRESSION',{x:0.6,y:0.5,w:12,h:0.7,fontFace:'Arial',fontSize:32});
s.addText('Total: '+rows.reduce((n,r)=>n+Number(r[1]),0),{x:0.6,y:2,w:11,h:1,fontFace:'Arial',fontSize:48});
s=p.addSlide();s.addText('Input values',{x:0.6,y:0.5,w:12,h:0.7,fontFace:'Arial',fontSize:32});
s.addChart(p.ChartType.bar,[{name:'Value',labels:rows.map(r=>r[0]),values:rows.map(r=>Number(r[1]))}],{x:0.8,y:1.5,w:11.6,h:5,catAxisLabelFontFace:'Arial',valAxisLabelFontFace:'Arial',showLegend:false});
p.writeFile({fileName:'/workspace/outputs/matrix.pptx'}).catch(e=>{console.error(e);process.exitCode=1});
JS`

const standardMatrixPresentationVerify = `python3 - <<'PY'
import re
from zipfile import ZipFile
from xml.etree import ElementTree as E
from pypdf import PdfReader
from PIL import Image
with ZipFile('/workspace/outputs/matrix.pptx') as z:
 assert z.testzip() is None
 slides=[n for n in z.namelist() if re.fullmatch(r'ppt/slides/slide\d+\.xml',n)]
 assert len(slides)==2
 chart=E.fromstring(z.read('ppt/charts/chart1.xml'))
 ns={'c':'http://schemas.openxmlformats.org/drawingml/2006/chart'}
 assert chart.find('.//c:barChart',ns) is not None
 assert [v.text for v in chart.findall('.//c:ser/c:val/c:numRef/c:numCache/c:pt/c:v',ns)]==['10','20']
 assert [v.text for v in chart.findall('.//c:ser/c:cat//c:pt/c:v',ns)]==['A','B']
pdf=PdfReader('/workspace/outputs/matrix.pdf');assert len(pdf.pages)==2
assert 'ZGI-STANDARD-REGRESSION' in pdf.pages[0].extract_text()
assert 'Total:30' in re.sub(r'\s+','',pdf.pages[0].extract_text())
for n in (1,2):
 with Image.open('/workspace/outputs/slide-'+str(n)+'.png') as im:
  assert im.format=='PNG' and im.width==900 and im.height>400;im.verify()
print('PPTX_NATIVE_CHART_AND_PDF_OK')
PY`

const standardMatrixAnimationCreate = `cd "$ZGI_SKILL_DIR"
python3 - <<'PY'
from core.gif_builder import GIFBuilder
from core.validators import validate_gif
from PIL import Image,ImageDraw
b=GIFBuilder(width=128,height=128,fps=10)
for i in range(12):
 f=Image.new('RGB',(128,128),(240,248,255));d=ImageDraw.Draw(f)
 d.ellipse([8+i*7,48,30+i*7,70],fill=(2,128,144),outline=(0,40,60),width=3);b.add_frame(f)
path='/workspace/outputs/matrix.gif';b.save(path,num_colors=48,optimize_for_emoji=True)
ok,info=validate_gif(path,is_emoji=True,verbose=False);assert ok,info
with Image.open(path) as g:
 assert g.size==(128,128) and g.n_frames==12
 frames=[]
 for i in range(g.n_frames):
  g.seek(i);assert g.info['duration']==100;frames.append(g.convert('RGB').tobytes())
 assert len(set(frames))==12
print('GIF_ORIGINAL_MODULES_OK')
PY`
