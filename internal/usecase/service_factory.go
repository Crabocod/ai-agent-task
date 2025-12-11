package usecase

import (
	"ai-agent-task/internal/usecase/adapters"
)

type serviceFactory struct {
	deps Params
}

func newServiceFactory(deps Params) *serviceFactory {
	return &serviceFactory{
		deps: deps,
	}
}

func (f *serviceFactory) CreateAgentService() adapters.AgentService {
	return NewAgentService(AgentServiceParams{
		Browser: f.deps.Browser,
		AI:      f.deps.AI,
		Config:  f.deps.Config,
		Logger:  f.deps.Logger,
	})
}

func (f *serviceFactory) CreateBrowserService() adapters.BrowserService {
	return f.deps.Browser
}

func (f *serviceFactory) CreateAIService() adapters.AIService {
	return f.deps.AI
}
