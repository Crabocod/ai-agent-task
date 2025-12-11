package usecase

import (
	"ai-agent-task/internal/config"
	"ai-agent-task/internal/ports"
	"ai-agent-task/internal/usecase/adapters"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Service struct {
	Agent   adapters.AgentService
	Browser adapters.BrowserService
	AI      adapters.AIService
}

type Params struct {
	fx.In

	Logger  *zap.Logger
	Config  *config.Config
	Browser ports.BrowserManager
	AI      ports.AIClient
}

func NewUsecase(params Params) *Service {
	factory := newServiceFactory(params)

	return &Service{
		Agent:   factory.CreateAgentService(),
		Browser: factory.CreateBrowserService(),
		AI:      factory.CreateAIService(),
	}
}
