package statistics

import (
	"github.com/zgiai/ginext/internal/modules/llm/statistics/handler"
	"github.com/zgiai/ginext/internal/modules/llm/statistics/repository"
	"github.com/zgiai/ginext/internal/modules/llm/statistics/service"
	"gorm.io/gorm"
)

// Module provides statistics functionality
type Module struct {
	Repository repository.StatisticsRepository
	Service    service.StatisticsService
	Handler    *handler.StatisticsHandler
}

// NewModule creates a new statistics module
func NewModule(db *gorm.DB) *Module {
	repo := repository.NewStatisticsRepository(db)
	svc := service.NewStatisticsService(repo)
	h := handler.NewStatisticsHandler(svc)

	return &Module{
		Repository: repo,
		Service:    svc,
		Handler:    h,
	}
}
