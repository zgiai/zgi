package fxapp

import (
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

var imageRuntimeModule = fx.Module("imageruntime",
	fx.Provide(
		provideImageRuntimeTaskPoller,
	),
)

func provideImageRuntimeTaskPoller(db *gorm.DB) *imageservice.TaskPoller {
	return imageservice.NewTaskPoller(db)
}
