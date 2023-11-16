package handlers

type Service string

const (
	Executor    Service = "executor"
	Autofiller  Service = "autofiller"
	AutoCreator Service = "autocreator"
)

type KillSerivce struct {
	ServiceType Service `json:"service" binding:"required"`
	Account     uint    `json:"userAccount"`
}
