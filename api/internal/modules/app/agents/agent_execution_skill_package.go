package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm"
)

const agentSkillPackageSetupTimeout = 60 * time.Second

type agentSkillPackage struct {
	ID             string
	Root           string
	ExpectedDigest string
}

type agentSkillPackageResolver func(context.Context, string) (agentSkillPackage, error)

func newAgentSkillPackageResolver(db *gorm.DB, organizationID uuid.UUID, enabledIDs []string) agentSkillPackageResolver {
	allowed := make(map[string]struct{}, len(enabledIDs))
	for _, id := range enabledIDs {
		allowed[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
	}
	return func(ctx context.Context, skillID string) (agentSkillPackage, error) {
		skillID = strings.ToLower(strings.TrimSpace(skillID))
		if _, ok := allowed[skillID]; !ok || db == nil {
			return agentSkillPackage{}, fmt.Errorf("skill is not enabled for this Agent")
		}
		var record runtimemodel.CustomSkill
		err := db.WithContext(ctx).Where("organization_id = ? AND skill_id = ? AND status = ?", organizationID, skillID, runtimemodel.CustomSkillStatusActive).Take(&record).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return agentSkillPackage{}, fmt.Errorf("enabled custom Skill package is unavailable")
			}
			return agentSkillPackage{}, fmt.Errorf("load enabled custom Skill package: %w", err)
		}
		if record.StoragePath == "" {
			return agentSkillPackage{}, fmt.Errorf("enabled custom Skill package has no storage path")
		}
		info, err := os.Stat(filepath.Join(record.StoragePath, "scripts"))
		if err != nil || !info.IsDir() {
			return agentSkillPackage{}, fmt.Errorf("enabled custom Skill has no scripts directory")
		}
		digest, _ := record.Manifest["package_sha256"].(string)
		if digest == "" {
			digest, err = skills.SkillPackageSHA256(record.StoragePath)
			if err != nil {
				return agentSkillPackage{}, fmt.Errorf("hash enabled custom Skill package: %w", err)
			}
		}
		return agentSkillPackage{ID: skillID, Root: record.StoragePath, ExpectedDigest: digest}, nil
	}
}

func stageAgentSkillPackage(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, binding agentExecutionBinding, capabilities agentExecutionCapabilities, skill agentSkillPackage) (string, error) {
	if !capabilities.FileTransfer || capabilities.MaxUploadBytes <= 0 {
		return "", fmt.Errorf("agent execution profile does not permit Skill package transfer")
	}
	if skill.ExpectedDigest != "" {
		workspacePath, err := skills.PortableSkillWorkspacePath(skill.ID, skill.ExpectedDigest)
		if err != nil {
			return "", err
		}
		markerPath := path.Join(workspacePath, ".zgi-ready")
		if marker, err := gateway.DownloadFile(ctx, scope, binding.ID, markerPath); err == nil && strings.TrimSpace(string(marker)) == skill.ExpectedDigest {
			return workspacePath, nil
		}
	}
	archive, digest, err := skills.BuildPortableSkillArchive(skill.Root)
	if err != nil {
		return "", err
	}
	if skill.ExpectedDigest != "" && digest != skill.ExpectedDigest {
		return "", fmt.Errorf("stored Skill package failed version integrity verification")
	}
	workspacePath, err := skills.PortableSkillWorkspacePath(skill.ID, digest)
	if err != nil {
		return "", err
	}
	markerPath := path.Join(workspacePath, ".zgi-ready")
	if marker, err := gateway.DownloadFile(ctx, scope, binding.ID, markerPath); err == nil && strings.TrimSpace(string(marker)) == digest {
		return workspacePath, nil
	}
	archiveDigest := sha256.Sum256(archive)
	archivePath := path.Join(".zgi", "skill-archives", skill.ID, uuid.NewString()+".zip")
	chunkSize := capabilities.MaxUploadBytes
	chunkCount := int64(1) + (int64(len(archive))-1)/chunkSize
	if chunkCount > 64 {
		return "", fmt.Errorf("Skill package requires too many upload chunks for this runtime")
	}
	for index := int64(0); index < chunkCount; index++ {
		start := index * chunkSize
		end := min(start+chunkSize, int64(len(archive)))
		uploadPath := archivePath
		if chunkCount > 1 {
			uploadPath += ".part." + strconv.FormatInt(index, 10)
		}
		if err := gateway.UploadFile(ctx, scope, binding.ID, uploadPath, archive[start:end]); err != nil {
			return "", fmt.Errorf("upload Skill package archive chunk %d/%d: %w", index+1, chunkCount, err)
		}
	}
	job, err := gateway.SubmitJob(ctx, scope, binding.ID, "skill-setup:"+binding.ID+":"+uuid.NewString(), "python3", []string{
		"-c", agentSkillPackageUnpackProgram,
		path.Join("/workspace", archivePath), path.Join("/workspace", workspacePath), hex.EncodeToString(archiveDigest[:]), digest, strconv.FormatInt(chunkCount, 10),
	}, agentSkillPackageSetupTimeout)
	if err != nil {
		return "", fmt.Errorf("submit Skill package setup job: %w", err)
	}
	for job.Status == "queued" || job.Status == "running" {
		select {
		case <-ctx.Done():
			return "", cancelInterruptedAgentExecutionJob(ctx, gateway, scope, job)
		case <-time.After(350 * time.Millisecond):
		}
		nextJob, err := gateway.GetJob(ctx, scope, job.ID)
		if err != nil {
			if ctx.Err() != nil {
				return "", cancelInterruptedAgentExecutionJob(ctx, gateway, scope, job)
			}
			return "", fmt.Errorf("read Skill package setup job: %w", err)
		}
		job = nextJob
	}
	if job.Status != "succeeded" || job.ExitCode == nil || *job.ExitCode != 0 {
		return "", fmt.Errorf("Skill package setup failed with status %s", job.Status)
	}
	marker, err := gateway.DownloadFile(ctx, scope, binding.ID, markerPath)
	if err != nil || strings.TrimSpace(string(marker)) != digest {
		return "", fmt.Errorf("Skill package setup did not create a valid completion marker")
	}
	return workspacePath, nil
}

// The setup program receives only server-generated paths and checks archive
// bytes, member types, member sizes, duplicates, and traversal before writing.
const agentSkillPackageUnpackProgram = `import hashlib, os, pathlib, shutil, stat, sys, tempfile, zipfile
archive, target, archive_digest, package_digest, part_count = sys.argv[1:]
part_count = int(part_count)
if part_count < 1 or part_count > 64:
    raise ValueError('invalid archive part count')
root = pathlib.Path('/workspace').resolve()
archive_path, target_path = pathlib.Path(archive).resolve(), pathlib.Path(target).resolve()
if not archive_path.is_relative_to(root) or not target_path.is_relative_to(root):
    raise ValueError('invalid workspace path')
if part_count > 1:
    with archive_path.open('xb') as combined:
        for index in range(part_count):
            part_path = pathlib.Path(str(archive_path) + '.part.' + str(index))
            if not part_path.resolve().is_relative_to(root):
                raise ValueError('invalid archive part path')
            with part_path.open('rb') as part:
                shutil.copyfileobj(part, combined, 65536)
            part_path.unlink()
with archive_path.open('rb') as source:
    if hashlib.file_digest(source, 'sha256').hexdigest() != archive_digest:
        raise ValueError('archive checksum mismatch')
target_path.parent.mkdir(parents=True, exist_ok=True)
temp = pathlib.Path(tempfile.mkdtemp(prefix='.skill-setup-', dir=target_path.parent))
try:
    seen, total = set(), 0
    with zipfile.ZipFile(archive_path) as bundle:
        files = [entry for entry in bundle.infolist() if not entry.is_dir()]
        if len(files) > 600:
            raise ValueError('too many package files')
        for entry in files:
            name = entry.filename
            parts = pathlib.PurePosixPath(name).parts
            mode = (entry.external_attr >> 16) & 0o170000
            if not name or '\\' in name or name.startswith('/') or any(part in ('', '.', '..') for part in parts) or mode not in (0, stat.S_IFREG) or name in seen:
                raise ValueError('invalid package member')
            seen.add(name)
            if entry.file_size > 10 * 1024 * 1024:
                raise ValueError('package file is too large')
            total += entry.file_size
            if total > 64 * 1024 * 1024:
                raise ValueError('package is too large')
            destination = temp.joinpath(*parts)
            destination.parent.mkdir(parents=True, exist_ok=True)
            written = 0
            with bundle.open(entry) as source, destination.open('xb') as output:
                while True:
                    chunk = source.read(65536)
                    if not chunk:
                        break
                    written += len(chunk)
                    if written > 10 * 1024 * 1024:
                        raise ValueError('expanded package file is too large')
                    output.write(chunk)
    if not (temp / 'SKILL.md').is_file():
        raise ValueError('SKILL.md is missing')
    (temp / '.zgi-ready').write_text(package_digest + '\n')
    if target_path.exists():
        if (target_path / '.zgi-ready').read_text().strip() != package_digest:
            raise ValueError('different package already staged at destination')
    else:
        os.rename(temp, target_path)
finally:
    shutil.rmtree(temp, ignore_errors=True)
    archive_path.unlink(missing_ok=True)
`
