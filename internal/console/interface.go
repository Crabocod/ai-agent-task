package console

import (
	"ai-agent-task/internal/config"
	"ai-agent-task/internal/usecase"
	"ai-agent-task/pkg/logg"
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Interface struct {
	config   *config.Config
	logger   *zap.Logger
	usecase  *usecase.Service
	ctx      context.Context
	cancel   context.CancelFunc
	sigChan  chan os.Signal
	stopping bool
}

type Params struct {
	fx.In

	Config  *config.Config
	Logger  *zap.Logger
	Usecase *usecase.Service
}

func NewInterface(params Params) *Interface {
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)

	return &Interface{
		config:   params.Config,
		logger:   params.Logger.With(zap.String(logg.Layer, "Console")),
		usecase:  params.Usecase,
		ctx:      ctx,
		cancel:   cancel,
		sigChan:  sigChan,
		stopping: false,
	}
}

func (i *Interface) Start() error {
	i.printBanner()
	i.printHelp()

	// Setup signal handler
	signal.Notify(i.sigChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Handle signals in goroutine
	go func() {
		<-i.sigChan
		fmt.Println("\n\n⚠️  Interrupt received, stopping task...")
		i.stopping = true
		i.Stop()
	}()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		if i.stopping {
			break
		}

		fmt.Print("\n> ")

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			continue
		}

		if err := i.handleCommand(input); err != nil {
			if err.Error() == "exit" {
				break
			}

			i.logger.Error("Command error", zap.Error(err))
			fmt.Printf("Error: %v\n", err)
		}
	}

	return nil
}

func (i *Interface) Stop() error {
	if i.stopping {
		return nil
	}

	i.stopping = true
	i.logger.Info("Stopping console interface...")

	// Cancel context first
	i.cancel()

	// Stop agent
	i.usecase.Agent.Stop()

	// Exit program
	fmt.Println("👋 Goodbye!")
	os.Exit(0)

	return nil
}

func (i *Interface) handleCommand(input string) error {
	switch input {
	case "help", "h":
		i.printHelp()

		return nil
	case "exit", "quit", "q":
		fmt.Println("Shutting down...")

		return fmt.Errorf("exit")
	default:
		return i.executeTask(input)
	}
}

func (i *Interface) executeTask(taskDescription string) error {
	fmt.Printf("\n🤖 Starting task: %s\n", taskDescription)
	fmt.Println("───────────────────────────────────────────────────")

	task, err := i.usecase.Agent.Execute(i.ctx, taskDescription)
	if err != nil {
		fmt.Printf("\n❌ Task failed: %v\n", err)

		return nil
	}

	fmt.Println("\n───────────────────────────────────────────────────")

	if task.Status == "completed" {
		fmt.Printf("✅ Task completed successfully!\n\n")
		fmt.Printf("Result: %s\n", task.Result)
		fmt.Printf("Steps taken: %d\n", len(task.Steps))
	} else {
		fmt.Printf("❌ Task failed: %s\n", task.Error)
	}

	return nil
}

func (i *Interface) printBanner() {
	banner := `
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║            🤖  AI Browser Agent  🌐                       ║
║                                                           ║
║  Autonomous web browser automation powered by Claude AI  ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
`
	fmt.Println(banner)
}

func (i *Interface) printHelp() {
	help := `
Available commands:
  help, h       - Show this help message
  exit, quit, q - Exit the application

To start a task, simply type your request in natural language:
  Examples:
    - Read my last 10 emails and delete spam
    - Find 3 AI engineer jobs on hh.ru
    - Order a burger from my favorite restaurant

The agent will autonomously execute the task.
`
	fmt.Println(help)
}
