package bootstrap

import (
	"ai-agent-task/internal/console"
	"ai-agent-task/internal/ports"
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

func runConsole(lc fx.Lifecycle, consoleInterface *console.Interface, browser ports.BrowserManager, logger *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting AI Agent Console Interface...")

			logger.Info("Launching browser...")

			if err := browser.Launch(ctx); err != nil {
				logger.Error("Failed to launch browser", zap.Error(err))

				return err
			}

			logger.Info("Browser launched successfully")

			go func() {
				if err := consoleInterface.Start(); err != nil {
					logger.Error("Console interface error", zap.Error(err))
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Shutting down AI Agent...")

			if err := consoleInterface.Stop(); err != nil {
				logger.Error("Failed to stop console", zap.Error(err))
			}

			if err := browser.Close(ctx); err != nil {
				logger.Error("Failed to close browser", zap.Error(err))
			}

			return nil
		},
	})
}
