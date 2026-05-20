package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/file_process/model"
	fileProcessRepo "github.com/zgiai/ginext/internal/modules/file_process/repository"
	file_service "github.com/zgiai/ginext/internal/modules/file_process/service"
	"github.com/zgiai/ginext/pkg/database"
	"github.com/zgiai/ginext/pkg/image"
	"github.com/zgiai/ginext/pkg/storage"
)

type agent struct {
	ID       string  `gorm:"column:id"`
	Name     string  `gorm:"column:name"`
	Icon     *string `gorm:"column:icon"`
	IconType *string `gorm:"column:icon_type"`
}

func (agent) TableName() string {
	return "agents"
}

type uploadFile struct {
	ID             string     `gorm:"column:id;primaryKey"`
	OrganizationID string     `gorm:"column:organization_id;type:varchar(255);not null"`
	WorkspaceID    *string    `gorm:"column:workspace_id;type:varchar(255)"`
	IsTemporary    bool       `gorm:"column:is_temporary;default:false"`
	StorageType    string     `gorm:"column:storage_type;type:varchar(255);not null"`
	Key            string     `gorm:"column:key;type:varchar(255);not null"`
	Name           string     `gorm:"column:name;type:varchar(255);not null"`
	Size           int64      `gorm:"column:size;not null"`
	Extension      string     `gorm:"column:extension;type:varchar(255);not null"`
	MimeType       string     `gorm:"column:mime_type;type:varchar(255)"`
	CreatedByRole  string     `gorm:"column:created_by_role;type:varchar(255);not null"`
	CreatedBy      string     `gorm:"column:created_by;type:varchar(255);not null"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	Used           bool       `gorm:"column:used;default:false"`
	UsedBy         *string    `gorm:"column:used_by;type:varchar(255)"`
	UsedAt         *time.Time `gorm:"column:used_at"`
	Hash           string     `gorm:"column:hash;type:varchar(255)"`
	SourceURL      string     `gorm:"column:source_url;type:text"`
}

func (uploadFile) TableName() string {
	return "upload_files"
}

func getExtensionFromMimeType(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func main() {
	statsOnly := flag.Bool("stats", false, "Only show statistics, don't migrate")
	batchSize := flag.Int("batch", 100, "Batch size for migration")
	dryRun := flag.Bool("dry-run", false, "Show what would be migrated without actually migrating")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	dbInstance, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}

	storageClient := storage.GetStorage()
	storageType := cfg.Storage.Type
	fmt.Printf("Using storage type: %s\n", storageType)

	fileRepo := fileProcessRepo.NewFileRepository(dbInstance)
	fileService := file_service.NewFileService(
		fileRepo,
		storageClient,
		dbInstance,
		nil,
		nil,
	)

	fmt.Println("================================================")
	fmt.Println("  Base64 Icon Migration Tool")
	fmt.Println("  (Using fileService.UploadFile)")
	fmt.Println("================================================")
	fmt.Println()

	var totalCount int64
	if err := dbInstance.Model(&agent{}).Where("icon_type = ?", "base64").Count(&totalCount).Error; err != nil {
		log.Fatalf("Failed to count base64 icons: %v", err)
	}

	fmt.Printf("Total agents with base64 icons: %d\n", totalCount)

	if totalCount == 0 {
		fmt.Println("\nNo base64 icons to migrate. Exiting.")
		return
	}

	fmt.Printf("Batch size: %d\n", *batchSize)
	fmt.Println()

	if *statsOnly {
		fmt.Println("Stats mode: showing statistics only.")
		return
	}

	if *dryRun {
		fmt.Println("Dry-run mode: showing what would be migrated without actually migrating.")
		fmt.Println()
	}

	batches := (int(totalCount) + *batchSize - 1) / *batchSize
	fmt.Printf("Will migrate in %d batches\n\n", batches)

	migratedCount := 0
	failedCount := 0
	ctx := context.Background()

	for i := 0; i < batches; i++ {
		start := time.Now()

		var agents []agent
		if err := dbInstance.Where("icon_type = ?", "base64").
			Limit(*batchSize).
			Find(&agents).Error; err != nil {
			log.Printf("Failed to query batch %d: %v", i+1, err)
			continue
		}

		if len(agents) == 0 {
			break
		}

		batchMigrated := 0
		batchFailed := 0

		for _, a := range agents {
			if a.Icon == nil || *a.Icon == "" {
				continue
			}

			iconStr := *a.Icon
			if !strings.HasPrefix(iconStr, "data:image") {
				continue
			}

			if *dryRun {
				fmt.Printf("[DRY-RUN] Would migrate: %s (%s)\n", a.ID, a.Name)
				batchMigrated++
				continue
			}

			parts := strings.SplitN(iconStr, ",", 2)
			if len(parts) != 2 {
				log.Printf("Invalid base64 format for agent %s", a.ID)
				batchFailed++
				continue
			}

			mimePart := strings.TrimPrefix(parts[0], "data:")
			mimeType := strings.TrimSuffix(mimePart, ";base64")

			imageData, err := base64.StdEncoding.DecodeString(parts[1])
			if err != nil {
				log.Printf("Failed to decode base64 for agent %s: %v", a.ID, err)
				batchFailed++
				continue
			}

			processedData, err := image.ProcessIconImage(imageData)
			if err != nil {
				log.Printf("Failed to process image for agent %s: %v", a.ID, err)
				batchFailed++
				continue
			}

			ext := getExtensionFromMimeType(mimeType)
			filename := fmt.Sprintf("icon_%s%s", a.ID, ext)

			uploadedFile, err := fileService.UploadFile(
				ctx,
				filename,
				processedData,
				mimeType,
				config.TempFileTenantID,
				config.TempFileTenantID,
				model.CreatedByRoleAccount,
				nil,
				nil,
				true,
				true,
			)
			if err != nil {
				log.Printf("Failed to upload file for agent %s: %v", a.ID, err)
				batchFailed++
				continue
			}

			if err := dbInstance.Model(&agent{}).
				Where("id = ?", a.ID).
				Updates(map[string]interface{}{
					"icon":      uploadedFile.ID,
					"icon_type": "image",
				}).Error; err != nil {
				log.Printf("Failed to update agent %s: %v", a.ID, err)
				batchFailed++
				continue
			}

			batchMigrated++
		}

		migratedCount += batchMigrated
		failedCount += batchFailed

		elapsed := time.Since(start)
		fmt.Printf("Batch %d/%d completed in %v (%d migrated, %d failed, %d/%d total)\n",
			i+1, batches, elapsed, batchMigrated, batchFailed, migratedCount, totalCount)
	}

	fmt.Println()
	fmt.Println("================================================")

	if *dryRun {
		fmt.Printf("Dry-run complete. Would migrate %d agents.\n", migratedCount)
	} else {
		fmt.Printf("Migration complete. Migrated %d agents, %d failed.\n", migratedCount, failedCount)
	}

	fmt.Println("================================================")
}
