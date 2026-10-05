package agents

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	agentExecutionDependencyPreparationTimeout = 60 * time.Second
	agentExecutionDependencyPreparationProgram = `import ast, hashlib, json, os, pathlib, re, shutil, stat, subprocess, sys, tempfile

package_root = pathlib.Path(sys.argv[1]).resolve()
skill_id = sys.argv[2]
digest = sys.argv[3]
workspace = pathlib.Path('/workspace').resolve()
home_value = os.environ.get('HOME', '').strip()
if not home_value:
    raise ValueError('runtime HOME is required')
home = pathlib.Path(home_value).resolve()
if not package_root.is_relative_to(workspace) or not home.is_relative_to(workspace):
    raise ValueError('invalid dependency preparation root')
if not re.fullmatch(r'[a-z0-9][a-z0-9_.-]{0,127}', skill_id) or not re.fullmatch(r'[0-9a-f]{64}', digest):
    raise ValueError('invalid Skill identity')
if not (package_root / '.zgi-ready').is_file():
    raise ValueError('Skill package is not staged')
if (package_root / '.zgi-ready').read_text().strip() != digest:
    raise ValueError('Skill package integrity marker mismatch')

output_root = (home / '.zgi-dependencies' / skill_id / digest).resolve()
if not output_root.is_relative_to(home):
    raise ValueError('invalid dependency output root')
manifest_names = ('requirements.txt', 'package.json')
manifest_files = [package_root / name for name in manifest_names if (package_root / name).is_file()]
unsupported = [name for name in ('pyproject.toml', 'Pipfile', 'npm-shrinkwrap.json') if (package_root / name).is_file()]
if unsupported:
    raise ValueError('unsupported dependency manifest: ' + ', '.join(unsupported))
if any(path.stat().st_size > 256 * 1024 for path in manifest_files):
    raise ValueError('dependency manifest is too large')

fingerprint_hash = hashlib.sha256()
fingerprint_hash.update(('zgi-agent-dependencies-v2:' + digest + '\n').encode())
for manifest in manifest_files:
    fingerprint_hash.update(manifest.name.encode())
    fingerprint_hash.update(b'\0')
    fingerprint_hash.update(manifest.read_bytes())
fingerprint = fingerprint_hash.hexdigest()
marker = output_root / 'prepare.json'

def cache_is_usable(previous):
    if not isinstance(previous, dict) or previous.get('fingerprint') != fingerprint:
        return False
    if previous.get('status') not in ('ready', 'no_dependencies', 'reused') or not output_root.is_dir() or output_root.is_symlink():
        return False
    if previous.get('source_mode') != 'offline-bundled' or previous.get('preparation_version') != 'v2':
        return False
    if previous.get('dependency_root') != str(output_root.relative_to(workspace)):
        return False
    for key, expected_type in (('python_path', 'file'), ('node_path', 'directory')):
        value = previous.get(key)
        if value is None:
            continue
        raw_candidate = workspace / str(value)
        if raw_candidate.is_symlink():
            return False
        candidate = raw_candidate.resolve()
        exists = candidate.is_file() if expected_type == 'file' else candidate.is_dir()
        if not candidate.is_relative_to(home) or not exists:
            return False
    marker = output_root / 'prepare.json'
    return marker.is_file() and not marker.is_symlink() and marker.stat().st_size <= 64 * 1024

if marker.is_file():
    try:
        previous = json.loads(marker.read_text())
    except Exception:
        previous = None
    if cache_is_usable(previous):
        previous['status'] = 'reused'
        previous['cache_status'] = 'reused'
        print(json.dumps(previous, sort_keys=True))
        raise SystemExit(0)

def fail(reason, code=78, **extra):
    result = {'status': 'blocked', 'fingerprint': fingerprint, 'reason': reason, **extra}
    print(json.dumps(result, sort_keys=True))
    raise SystemExit(code)

def read_requirements(path):
    requirements = []
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith('#'):
            continue
        if line.startswith(('-', 'http:', 'https:', 'git+', 'file:', '.')):
            fail('python_dependency_source_not_allowed', source=line[:120])
        if not re.fullmatch(r'[A-Za-z0-9_.-]+(?:\[[A-Za-z0-9_,.-]+\])?(?:[<>=!~].*)?', line):
            fail('python_dependency_declaration_invalid', source=line[:120])
        requirements.append(line)
    return requirements

python_stdlib = set(getattr(sys, 'stdlib_module_names', ()))
python_stdlib.update(('__future__', 'builtins'))

def detect_python_imports():
    scripts = package_root / 'scripts'
    if not scripts.is_dir():
        return []
    local_modules = set()
    for item in scripts.rglob('*.py'):
        relative = item.relative_to(scripts)
        if relative.name == '__init__.py' and len(relative.parts) > 1:
            local_modules.add(relative.parts[0])
        elif len(relative.parts) == 1:
            local_modules.add(relative.stem)
    detected = set()
    for item in scripts.rglob('*.py'):
        try:
            tree = ast.parse(item.read_text(), filename=str(item))
        except (OSError, SyntaxError):
            continue
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                names = [alias.name for alias in node.names]
            elif isinstance(node, ast.ImportFrom) and node.module:
                names = [node.module]
            else:
                continue
            for name in names:
                top_level = name.split('.')[0]
                if top_level not in python_stdlib and top_level not in local_modules:
                    detected.add(top_level)
    return sorted(detected)

node_builtin_modules = set(('assert', 'async_hooks', 'buffer', 'child_process', 'cluster', 'console', 'constants', 'crypto', 'dgram', 'diagnostics_channel', 'dns', 'domain', 'events', 'fs', 'http', 'http2', 'https', 'inspector', 'module', 'net', 'os', 'path', 'perf_hooks', 'process', 'punycode', 'querystring', 'readline', 'repl', 'stream', 'string_decoder', 'sys', 'test', 'timers', 'tls', 'trace_events', 'tty', 'url', 'util', 'v8', 'vm', 'wasi', 'worker_threads', 'zlib'))

def detect_node_imports():
    scripts = package_root / 'scripts'
    if not scripts.is_dir():
        return []
    local_modules = set()
    node_modules = scripts / 'node_modules'
    if node_modules.is_dir():
        for item in node_modules.iterdir():
            if item.name.startswith('@') and item.is_dir():
                local_modules.update(item.name + '/' + child.name for child in item.iterdir())
            elif not item.name.startswith('.'):
                local_modules.add(item.name)
    detected = set()
    patterns = (re.compile(r"require\s*\(\s*['\"]([^'\"]+)"), re.compile(r"(?:from|import)\s*['\"]([^'\"]+)"), re.compile(r"import\s*\(\s*['\"]([^'\"]+)"))
    for item in list(scripts.rglob('*.js')) + list(scripts.rglob('*.mjs')) + list(scripts.rglob('*.cjs')):
        try:
            source = item.read_text()
        except OSError:
            continue
        for pattern in patterns:
            for name in pattern.findall(source):
                if name.startswith('.') or name.startswith('node:'):
                    continue
                package_name = '/'.join(name.split('/')[:2]) if name.startswith('@') else name.split('/')[0]
                if package_name not in node_builtin_modules and package_name not in local_modules:
                    detected.add(package_name)
    return sorted(detected)

def copy_regular_tree(source, destination):
    if not source.is_dir() or source.is_symlink():
        fail('offline_dependency_source_missing', source=str(source.relative_to(package_root)))
    total = 0
    for item in source.rglob('*'):
        if item.is_symlink():
            fail('offline_dependency_source_symlink', source=str(item.relative_to(package_root)))
        relative = item.relative_to(source)
        target = destination / relative
        if item.is_dir():
            target.mkdir(parents=True, exist_ok=True)
            continue
        if not item.is_file():
            fail('offline_dependency_source_not_regular', source=str(item.relative_to(package_root)))
        size = item.stat().st_size
        if size > 10 * 1024 * 1024:
            fail('offline_dependency_file_too_large', source=str(item.relative_to(package_root)))
        total += size
        if total > 64 * 1024 * 1024:
            fail('offline_dependency_source_too_large')
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(item, target)

def run_checked(command, cwd):
    try:
        result = subprocess.run(command, cwd=str(cwd), capture_output=True, text=True, timeout=45, check=False)
    except (OSError, subprocess.TimeoutExpired) as exc:
        fail('dependency_prepare_command_failed', command=command[0], detail=str(exc)[:240])
    if result.returncode != 0:
        detail = (result.stderr or result.stdout or '').strip()[-2000:]
        fail('dependency_prepare_command_failed', command=command[0], exit_code=result.returncode, detail=detail)

python_requirements = []
if (package_root / 'requirements.txt').is_file():
    python_requirements = read_requirements(package_root / 'requirements.txt')

node_dependencies = {}
if (package_root / 'package.json').is_file():
    try:
        package = json.loads((package_root / 'package.json').read_text())
    except Exception as exc:
        fail('node_package_manifest_invalid', detail=str(exc)[:240])
    for group in ('dependencies', 'optionalDependencies'):
        values = package.get(group, {})
        if values is None:
            continue
        if not isinstance(values, dict):
            fail('node_dependency_declaration_invalid', group=group)
        for name, version in values.items():
            if not re.fullmatch(r'@?[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)?', str(name)) or not isinstance(version, str) or version.startswith(('http:', 'https:', 'git', 'file:', 'workspace:')):
                fail('node_dependency_declaration_invalid', package=str(name)[:120])
            node_dependencies[name] = version

if not python_requirements and not node_dependencies:
    detected_python = detect_python_imports()
    detected_node = detect_node_imports()
    if detected_python or detected_node:
        fail('dependency_manifest_required', python=detected_python, node=detected_node, detail='declare dependencies in requirements.txt or package.json before preparation')
    output_root.parent.mkdir(parents=True, exist_ok=True)
    temporary = pathlib.Path(tempfile.mkdtemp(prefix='.deps-', dir=str(output_root.parent)))
    result = {'status': 'no_dependencies', 'cache_status': 'created', 'preparation_version': 'v2', 'source_mode': 'offline-bundled', 'fingerprint': fingerprint, 'dependency_root': str(output_root.relative_to(workspace)), 'manifests': [path.name for path in manifest_files]}
    (temporary / 'prepare.json').write_text(json.dumps(result, sort_keys=True) + '\n')
    if output_root.is_symlink():
        fail('offline_dependency_cache_invalid', path=str(output_root.relative_to(home)))
    if output_root.exists():
        shutil.rmtree(output_root)
    os.replace(temporary, output_root)
    print(json.dumps(result, sort_keys=True))
    raise SystemExit(0)

output_root.parent.mkdir(parents=True, exist_ok=True)
temporary = pathlib.Path(tempfile.mkdtemp(prefix='.deps-', dir=str(output_root.parent)))
try:
    result = {'status': 'ready', 'cache_status': 'created', 'preparation_version': 'v2', 'source_mode': 'offline-bundled', 'fingerprint': fingerprint, 'dependency_root': str(output_root.relative_to(workspace)), 'manifests': [path.name for path in manifest_files]}
    if python_requirements:
        wheels = package_root / 'vendor' / 'python'
        if not wheels.is_dir():
            fail('offline_dependency_source_missing', language='python3', source='vendor/python', required=python_requirements)
        wheel_files = [item for item in wheels.iterdir() if item.is_file() and not item.is_symlink()]
        if not wheel_files or any(item.suffix.lower() != '.whl' for item in wheel_files):
            fail('offline_python_wheels_required', source='vendor/python')
        python_root = temporary / 'python'
        run_checked(['python3', '-m', 'venv', '--copies', str(python_root)], package_root)
        run_checked([str(python_root / 'bin' / 'python'), '-m', 'pip', 'install', '--disable-pip-version-check', '--no-input', '--no-index', '--only-binary=:all:', '--find-links', str(wheels), '-r', str(package_root / 'requirements.txt')], package_root)
        result['python_path'] = str((output_root / 'python' / 'bin' / 'python').relative_to(workspace))
        result['python_command'] = result['python_path']
    if node_dependencies:
        modules = package_root / 'vendor' / 'node_modules'
        node_root = temporary / 'node'
        copy_regular_tree(modules, node_root / 'node_modules')
        result['node_path'] = str((output_root / 'node' / 'node_modules').relative_to(workspace))
        result['node_env'] = 'NODE_PATH=' + result['node_path']
    (temporary / 'prepare.json').write_text(json.dumps(result, sort_keys=True) + '\n')
    output_root.parent.mkdir(parents=True, exist_ok=True)
    if output_root.is_symlink():
        fail('offline_dependency_cache_invalid', path=str(output_root.relative_to(home)))
    if output_root.exists():
        shutil.rmtree(output_root)
    os.replace(temporary, output_root)
    print(json.dumps(result, sort_keys=True))
finally:
    shutil.rmtree(temporary, ignore_errors=True)
`
)

func newAgentExecutionDependencyPreparationTool(gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend, networkPolicy string, skillResolvers ...agentSkillPackageResolver) skillloop.RuntimeTool {
	return skillloop.RuntimeTool{
		SkillID: "agent_execution",
		Definition: adapter.Tool{Type: "function", Function: adapter.Function{
			Name:        "workspace_prepare_dependencies",
			Description: "Prepare a custom Skill's Python or Node dependencies inside the isolated session Home from offline package sources bundled in the Skill. Results are content-addressed and safely reused only when their prepared paths still exist. This never enables network access or system-package installation; missing offline sources return a diagnostic result.",
			Parameters: map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"skill_id":       map[string]any{"type": "string", "description": "Enabled custom Skill package to prepare"},
					"package_sha256": map[string]any{"type": "string", "description": "Exact package_sha256 supplied in session_scripts for skill_id", "minLength": 64, "maxLength": 64},
				},
				"required": []string{"skill_id", "package_sha256"},
			},
		}},
		Handler: func(ctx context.Context, call adapter.ToolCall) skillloop.RuntimeToolResult {
			return executeAgentExecutionDependencyPreparation(ctx, gateway, scope, agentID, conversationID, messageID, profileID, backend, networkPolicy, call, skillResolvers...)
		},
	}
}

func executeAgentExecutionDependencyPreparation(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, messageID, profileID, backend, networkPolicy string, call adapter.ToolCall, skillResolvers ...agentSkillPackageResolver) skillloop.RuntimeToolResult {
	failed := func(code string, err error, result map[string]interface{}) skillloop.RuntimeToolResult {
		if result == nil {
			result = map[string]interface{}{}
		}
		result["error_code"] = code
		return skillloop.RuntimeToolResult{Status: "failed", Result: result, ModelResult: result, Error: err}
	}
	var args struct {
		SkillID       string `json:"skill_id"`
		PackageSHA256 string `json:"package_sha256"`
	}
	decoder := json.NewDecoder(strings.NewReader(call.Function.Arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return failed("invalid_arguments", err, nil)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return failed("invalid_arguments", fmt.Errorf("unexpected trailing dependency arguments"), nil)
	}
	args.SkillID = strings.ToLower(strings.TrimSpace(args.SkillID))
	if args.SkillID == "" || len(args.PackageSHA256) != 64 || strings.ToLower(args.PackageSHA256) != args.PackageSHA256 {
		return failed("skill_version_required", fmt.Errorf("pass the active Skill's package_sha256 before preparing dependencies"), nil)
	}
	if _, err := hex.DecodeString(args.PackageSHA256); err != nil {
		return failed("skill_version_required", err, nil)
	}
	if len(skillResolvers) == 0 || skillResolvers[0] == nil {
		return failed("skill_package_unavailable", fmt.Errorf("Skill package preparation is unavailable for this caller"), nil)
	}
	operationCtx, cancel := context.WithTimeout(ctx, agentExecutionDependencyPreparationTimeout+120*time.Second)
	defer cancel()
	binding, err := readyAgentExecutionBinding(operationCtx, gateway, scope, agentID, conversationID, profileID, backend, networkPolicy)
	if err != nil {
		return failed("binding_failed", err, nil)
	}
	capabilities, err := gateway.Capabilities(operationCtx, scope, binding.ID)
	if err != nil {
		return failed("capability_check_failed", err, nil)
	}
	dependencyMode := strings.TrimSpace(capabilities.DependencyPreparationMode)
	if dependencyMode == "" && capabilities.DependencyPreparation {
		// Workers predating the explicit mode field are still safe because the
		// only previously advertised preparation mode was offline bundled data.
		dependencyMode = "offline-bundled"
	}
	if !capabilities.Shell || !capabilities.FileTransfer || !capabilities.DependencyPreparation || dependencyMode != "offline-bundled" || capabilities.NetworkMode != "disabled" {
		return failed("dependency_preparation_denied", fmt.Errorf("the approved runtime does not permit offline dependency preparation"), map[string]interface{}{"network_mode": capabilities.NetworkMode})
	}
	pack, err := skillResolvers[0](operationCtx, args.SkillID)
	if err != nil {
		return failed("skill_package_unavailable", err, nil)
	}
	if pack.ExpectedDigest != args.PackageSHA256 {
		return failed("skill_version_changed", fmt.Errorf("Skill package changed since activation"), nil)
	}
	skillPath, err := stageAgentSkillPackage(operationCtx, gateway, scope, binding, capabilities, pack)
	if err != nil {
		return failed("skill_package_staging_failed", err, nil)
	}
	callID := strings.TrimSpace(call.ID)
	if callID == "" {
		callID = uuid.NewString()
	}
	job, err := gateway.SubmitJob(operationCtx, scope, binding.ID, "dependency-prepare:"+messageID+":"+callID+":"+args.SkillID+":"+args.PackageSHA256, "python3", []string{"-c", agentExecutionDependencyPreparationProgram, "/workspace/" + skillPath, args.SkillID, args.PackageSHA256}, agentExecutionDependencyPreparationTimeout)
	if err != nil {
		return failed("dependency_prepare_submission_failed", err, nil)
	}
	for job.Status == "queued" || job.Status == "running" {
		select {
		case <-operationCtx.Done():
			return failed("dependency_prepare_interrupted", cancelInterruptedAgentExecutionJob(operationCtx, gateway, scope, job), map[string]interface{}{"job_id": job.ID})
		case <-time.After(350 * time.Millisecond):
		}
		nextJob, getErr := gateway.GetJob(operationCtx, scope, job.ID)
		if getErr != nil {
			if operationCtx.Err() != nil {
				return failed("dependency_prepare_interrupted", cancelInterruptedAgentExecutionJob(operationCtx, gateway, scope, job), map[string]interface{}{"job_id": job.ID})
			}
			return failed("dependency_prepare_status_failed", getErr, map[string]interface{}{"job_id": job.ID})
		}
		job = nextJob
	}
	stdout, stderr, truncated, err := readAgentExecutionOutput(operationCtx, gateway, scope, job.ID)
	if err != nil {
		return failed("dependency_prepare_output_failed", err, map[string]interface{}{"job_id": job.ID})
	}
	result := map[string]interface{}{"job_id": job.ID, "status": job.Status, "exit_code": job.ExitCode, "truncated": truncated, "skill_workspace_path": skillPath, "network_mode": capabilities.NetworkMode, "dependency_preparation_mode": dependencyMode, "offline_only": true, "source_mode": "offline-bundled", "preparation_version": "v2"}
	modelResult := map[string]interface{}{"job_id": job.ID, "status": job.Status, "exit_code": job.ExitCode, "stdout": stdout, "stderr": stderr, "truncated": truncated, "skill_workspace_path": skillPath, "dependency_preparation_mode": dependencyMode, "offline_only": true, "source_mode": "offline-bundled", "preparation_version": "v2"}
	if job.Status != "succeeded" || job.ExitCode == nil || *job.ExitCode != 0 {
		return failed("dependency_prepare_failed", fmt.Errorf("dependency preparation exited with status %s", job.Status), modelResult)
	}
	var prepared map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &prepared); err != nil || prepared["status"] == nil {
		return failed("dependency_prepare_protocol_error", fmt.Errorf("dependency preparation did not return a valid JSON result"), modelResult)
	}
	for key, value := range prepared {
		result[key] = value
		modelResult[key] = value
	}
	return skillloop.RuntimeToolResult{Status: "success", Arguments: map[string]interface{}{"skill_id": args.SkillID}, Result: result, ModelResult: modelResult}
}
