package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var (
	modelName   = flag.String("model", "", "model name (required)")
	tableName   = flag.String("table", "", "database table name (optional; defaults to lower-case plural model name)")
	packageName = flag.String("package", "", "package name (optional; defaults to lower-case model name)")
	outputDir   = flag.String("output", "internal", "output directory (optional; defaults to internal)")
	force       = flag.Bool("force", false, "whether to overwrite existing files")
)

type TemplateData struct {
	ModelName    string
	TableName    string
	PackageName  string
	StructFields []Field
}

type Field struct {
	Name       string
	Type       string
	Tag        string
	Comment    string
	IsRequired bool
}

func main() {
	flag.Parse()

	if *modelName == "" {
		fmt.Println("error: model name is required (use -model)")
		flag.Usage()
		os.Exit(1)
	}

	if *tableName == "" {
		*tableName = toSnakeCase(*modelName) + "s"
	}
	if *packageName == "" {
		*packageName = strings.ToLower(*modelName)
	}

	data := &TemplateData{
		ModelName:   *modelName,
		TableName:   *tableName,
		PackageName: *packageName,
		StructFields: []Field{
			{Name: "ID", Type: "uint", Tag: "gorm:\"primarykey\"", Comment: "primary key ID", IsRequired: true},
			{Name: "CreatedAt", Type: "time.Time", Tag: "gorm:\"autoCreateTime\"", Comment: "created time", IsRequired: true},
			{Name: "UpdatedAt", Type: "time.Time", Tag: "gorm:\"autoUpdateTime\"", Comment: "updated time", IsRequired: true},
		},
	}

	files := map[string]string{
		"model":      "model.go",
		"service":    "service.go",
		"handler":    "handler.go",
		"repository": "repository.go",
	}

	for fileType, filename := range files {
		if err := generateFile(fileType, filename, data); err != nil {
			fmt.Printf("failed to generate %s: %v\n", filename, err)
			os.Exit(1)
		}
	}

	fmt.Println("code generation complete")
}

func generateFile(fileType, filename string, data *TemplateData) error {
	dir := filepath.Join(*outputDir, data.PackageName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	filePath := filepath.Join(dir, filename)
	if _, err := os.Stat(filePath); err == nil && !*force {
		return fmt.Errorf("file %s already exists; use -force to overwrite", filePath)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	tmpl, err := getTemplate(fileType)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	fmt.Printf("generated file: %s\n", filePath)
	return nil
}

func getTemplate(fileType string) (*template.Template, error) {
	var tmplStr string
	switch fileType {
	case "model":
		tmplStr = modelTemplate
	case "service":
		tmplStr = serviceTemplate
	case "handler":
		tmplStr = handlerTemplate
	case "repository":
		tmplStr = repositoryTemplate
	default:
		return nil, fmt.Errorf("unknown file type: %s", fileType)
	}

	return template.New(fileType).Parse(tmplStr)
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

const modelTemplate = `package {{.PackageName}}

import (
	"time"
	"gorm.io/gorm"
)

type {{.ModelName}} struct {
	{{range .StructFields}}
	{{.Name}} {{.Type}} {{.Tag}} {{.Comment}}
	{{end}}
}

func ({{.ModelName}}) TableName() string {
	return "{{.TableName}}"
}
`

const serviceTemplate = `package {{.PackageName}}

import (
	"context"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type {{.ModelName}}Service interface {
	Create(ctx context.Context, model *{{.ModelName}}) error
	Update(ctx context.Context, model *{{.ModelName}}) error
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*{{.ModelName}}, error)
	List(ctx context.Context, page, pageSize int) ([]*{{.ModelName}}, int64, error)
}

type {{.ModelName}}ServiceImpl struct {
	repo {{.ModelName}}Repository
}

func New{{.ModelName}}Service(repo {{.ModelName}}Repository) {{.ModelName}}Service {
	return &{{.ModelName}}ServiceImpl{repo: repo}
}

func (s *{{.ModelName}}ServiceImpl) Create(ctx context.Context, model *{{.ModelName}}) error {
	return s.repo.Create(ctx, model)
}

func (s *{{.ModelName}}ServiceImpl) Update(ctx context.Context, model *{{.ModelName}}) error {
	return s.repo.Update(ctx, model)
}

func (s *{{.ModelName}}ServiceImpl) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

func (s *{{.ModelName}}ServiceImpl) Get(ctx context.Context, id uint) (*{{.ModelName}}, error) {
	return s.repo.Get(ctx, id)
}

func (s *{{.ModelName}}ServiceImpl) List(ctx context.Context, page, pageSize int) ([]*{{.ModelName}}, int64, error) {
	return s.repo.List(ctx, page, pageSize)
}
`

const handlerTemplate = `package {{.PackageName}}

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

type {{.ModelName}}Handler struct {
	service {{.ModelName}}Service
}

func New{{.ModelName}}Handler(service {{.ModelName}}Service) *{{.ModelName}}Handler {
	return &{{.ModelName}}Handler{service: service}
}

func (h *{{.ModelName}}Handler) Create(c *gin.Context) {
	var model {{.ModelName}}
	if err := c.ShouldBindJSON(&model); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request data")
		return
	}

	if err := h.service.Create(c.Request.Context(), &model); err != nil {
		logger.Error("failed to create {{.ModelName}}", "error", err)
		response.Error(c, http.StatusInternalServerError, "create failed")
		return
	}

	response.Success(c, model)
}

func (h *{{.ModelName}}Handler) Update(c *gin.Context) {
	var model {{.ModelName}}
	if err := c.ShouldBindJSON(&model); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid request data")
		return
	}

	if err := h.service.Update(c.Request.Context(), &model); err != nil {
		logger.Error("failed to update {{.ModelName}}", "error", err)
		response.Error(c, http.StatusInternalServerError, "update failed")
		return
	}

	response.Success(c, model)
}

func (h *{{.ModelName}}Handler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), uint(id)); err != nil {
		logger.Error("failed to delete {{.ModelName}}", "error", err)
		response.Error(c, http.StatusInternalServerError, "delete failed")
		return
	}

	response.Success(c, nil)
}

func (h *{{.ModelName}}Handler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid ID")
		return
	}

	model, err := h.service.Get(c.Request.Context(), uint(id))
	if err != nil {
		logger.Error("failed to get {{.ModelName}}", "error", err)
		response.Error(c, http.StatusInternalServerError, "get failed")
		return
	}

	response.Success(c, model)
}

func (h *{{.ModelName}}Handler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	models, total, err := h.service.List(c.Request.Context(), page, pageSize)
	if err != nil {
		logger.Error("failed to list {{.ModelName}}", "error", err)
		response.Error(c, http.StatusInternalServerError, "list failed")
		return
	}

	response.Success(c, gin.H{
		"items": models,
		"total": total,
	})
}
`

const repositoryTemplate = `package {{.PackageName}}

import (
	"context"
	"gorm.io/gorm"
	"github.com/zgiai/zgi/api/pkg/database"
)

type {{.ModelName}}Repository interface {
	Create(ctx context.Context, model *{{.ModelName}}) error
	Update(ctx context.Context, model *{{.ModelName}}) error
	Delete(ctx context.Context, id uint) error
	Get(ctx context.Context, id uint) (*{{.ModelName}}, error)
	List(ctx context.Context, page, pageSize int) ([]*{{.ModelName}}, int64, error)
}

type {{.ModelName}}RepositoryImpl struct {
	db *gorm.DB
}

func New{{.ModelName}}Repository() {{.ModelName}}Repository {
	return &{{.ModelName}}RepositoryImpl{
		db: database.GetDB(),
	}
}

func (r *{{.ModelName}}RepositoryImpl) Create(ctx context.Context, model *{{.ModelName}}) error {
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *{{.ModelName}}RepositoryImpl) Update(ctx context.Context, model *{{.ModelName}}) error {
	return r.db.WithContext(ctx).Save(model).Error
}

func (r *{{.ModelName}}RepositoryImpl) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&{{.ModelName}}{}, id).Error
}

func (r *{{.ModelName}}RepositoryImpl) Get(ctx context.Context, id uint) (*{{.ModelName}}, error) {
	var model {{.ModelName}}
	err := r.db.WithContext(ctx).First(&model, id).Error
	if err != nil {
		return nil, err
	}
	return &model, nil
}

func (r *{{.ModelName}}RepositoryImpl) List(ctx context.Context, page, pageSize int) ([]*{{.ModelName}}, int64, error) {
	var models []*{{.ModelName}}
	var total int64

	offset := (page - 1) * pageSize
	err := r.db.WithContext(ctx).Model(&{{.ModelName}}{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = r.db.WithContext(ctx).Offset(offset).Limit(pageSize).Find(&models).Error
	if err != nil {
		return nil, 0, err
	}

	return models, total, nil
}
`
