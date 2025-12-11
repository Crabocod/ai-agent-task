package bootstrap

import (
	"ai-agent-task/internal/ai"
	"ai-agent-task/internal/browser"
	"ai-agent-task/internal/config"
	"ai-agent-task/internal/console"
	"ai-agent-task/internal/ports"
	"ai-agent-task/internal/usecase"
	"time"

	"go.uber.org/fx"
)

func NewApp() *fx.App {
	return fx.New(
		fx.Provide(
			config.GetConfig,
			newLogger,
			newTraceProvider,

			fx.Annotate(browser.NewManager, fx.As(new(ports.BrowserManager))),
			fx.Annotate(ai.NewClient, fx.As(new(ports.AIClient))),

			usecase.NewUsecase,

			console.NewInterface,
		),

		fx.Invoke(
			runConsole,
		),

		fx.StartTimeout(10*time.Second),
	)
}
