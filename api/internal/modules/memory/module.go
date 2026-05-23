package memory

import "gorm.io/gorm"

type Module struct {
	Handler *Handler
	Service *Service
}

func NewModule(db *gorm.DB) *Module {
	service := NewService(db)
	return &Module{
		Handler: NewHandler(service),
		Service: service,
	}
}
