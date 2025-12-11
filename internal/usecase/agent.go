package usecase

import (
	"ai-agent-task/internal/config"
	"ai-agent-task/internal/entity"
	"ai-agent-task/internal/ports"
	"ai-agent-task/pkg/apperr"
	"ai-agent-task/pkg/logg"
	"ai-agent-task/pkg/tracing"
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	agentServiceName = "AgentService"
	agentTracer      = "usecase.agent"
	maxIterations    = 16
	maxConsecutiveErrors = 3
)

type AgentService struct {
	config        *config.Config
	logger        *zap.Logger
	browser       ports.BrowserManager
	ai            ports.AIClient
	tracer        trace.Tracer
	stopChan      chan struct{}
	running       bool
	lastURL       string
	lastAction    *entity.BrowserAction
}

type AgentServiceParams struct {
	fx.In

	Config  *config.Config
	Logger  *zap.Logger
	Browser ports.BrowserManager
	AI      ports.AIClient
}

func NewAgentService(params AgentServiceParams) *AgentService {
	return &AgentService{
		config:   params.Config,
		logger:   params.Logger.With(zap.String(logg.Layer, agentServiceName)),
		browser:  params.Browser,
		ai:       params.AI,
		tracer:   otel.Tracer(agentTracer),
		stopChan: make(chan struct{}),
		running:  false,
	}
}

func (s *AgentService) Execute(ctx context.Context, taskDescription string) (resp *entity.Task, err error) {
	const op = "Execute"
	logger := s.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op,
		attribute.String("task_description", taskDescription))
	defer func() {
		step.End(err)
	}()

	if taskDescription == "" {
		return nil, apperr.InvalidReqError(op, "task_description", errors.New("task description cannot be empty"))
	}

	task := &entity.Task{
		ID:          uuid.New(),
		Description: taskDescription,
		Status:      entity.TaskStatusInProgress,
		CreatedAt:   time.Now(),
		Steps:       make([]entity.Step, 0),
	}

	logger = logger.With(zap.String(logg.TaskID, task.ID.String()))
	step.AddEvent("task created")

	if !s.browser.IsReady() {
		task.Status = entity.TaskStatusFailed
		task.Error = "browser is not ready"

		return task, apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	systemPrompt := s.buildSystemPrompt(taskDescription)

	messages := []entity.AIMessage{
		{
			Role:    "user",
			Content: systemPrompt,
		},
	}

	s.running = true
	s.stopChan = make(chan struct{})
	iteration := 0
	consecutiveErrors := 0

	for s.running && iteration < maxIterations {
		// Check for cancellation before each iteration
		select {
		case <-ctx.Done():
			fmt.Println("\n\n⚠️  Task cancelled by user")
			task.Status = entity.TaskStatusFailed
			task.Error = "context cancelled"

			return task, apperr.Wrap(op, apperr.CodeInternal, ctx.Err(), map[string]any{
				apperr.MetaReason: "context_cancelled",
			})
		case <-s.stopChan:
			fmt.Println("\n\n⚠️  Task stopped by user")
			task.Status = entity.TaskStatusFailed
			task.Error = "stopped by user"

			return task, apperr.WrapErrorWithReason(op, apperr.CodeCancelledByUser, "stopped_by_user")
		default:
			// Continue with iteration
		}

		if !s.running {
			fmt.Println("\n\n⚠️  Task stopped by user")
			task.Status = entity.TaskStatusFailed
			task.Error = "stopped by user"

			return task, apperr.WrapErrorWithReason(op, apperr.CodeCancelledByUser, "stopped_by_user")
		}

		iteration++
		fmt.Printf("\n🔄 Iteration %d: ", iteration)

		step.AddEvent("sending message to AI")

		response, err := s.ai.SendMessage(ctx, messages)
		if err != nil {
			logger.Error("AI request failed", zap.Error(err))
			consecutiveErrors++

			if consecutiveErrors >= maxConsecutiveErrors {
				task.Status = entity.TaskStatusFailed
				task.Error = fmt.Sprintf("too many AI errors: %v", err)

				return task, apperr.Wrap(op, apperr.CodeAIError, err, map[string]any{
					apperr.MetaReason: "too_many_ai_errors",
					apperr.MetaStage:  apperr.StageAI,
				})
			}

			time.Sleep(time.Second * 2)

			continue
		}

		consecutiveErrors = 0

		if response.Thought != "" {
			fmt.Printf("%s\n", response.Thought)

			messages = append(messages, entity.AIMessage{
				Role:    "assistant",
				Content: response.Thought,
			})
		}

		if response.Complete {
			fmt.Printf("✅ Task completed: %s\n", response.Result)
			task.Status = entity.TaskStatusCompleted
			task.Result = response.Result
			completedAt := time.Now()
			task.CompletedAt = &completedAt
			step.AddEvent("task completed")

			return task, nil
		}

		if response.Action != nil {
			if err := s.handleAction(ctx, task, response.Action, &messages); err != nil {
				logger.Error("Action failed", zap.Error(err))
				consecutiveErrors++

				if consecutiveErrors >= maxConsecutiveErrors {
					task.Status = entity.TaskStatusFailed
					task.Error = fmt.Sprintf("too many consecutive action errors: %v", err)

					return task, apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
						apperr.MetaReason: "too_many_action_errors",
						apperr.MetaStage:  apperr.StageInteraction,
					})
				}
			} else {
				consecutiveErrors = 0
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	if iteration >= maxIterations {
		task.Status = entity.TaskStatusFailed
		task.Error = "max iterations reached"

		return task, apperr.WrapErrorWithReason(op, apperr.CodeMaxIterations, "max_iterations_reached")
	}

	return task, nil
}

func (s *AgentService) Stop() {
	const op = "Stop"
	logger := s.logger.With(zap.String(logg.Operation, op))
	logger.Info("Stopping agent...")

	s.running = false
	close(s.stopChan)
}

func (s *AgentService) handleAction(
	ctx context.Context,
	task *entity.Task,
	action *entity.BrowserAction,
	messages *[]entity.AIMessage,
) (err error) {
	const op = "handleAction"
	logger := s.logger.With(zap.String(logg.Operation, op), zap.String(logg.Action, string(action.Type)))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op,
		attribute.String("action_type", string(action.Type)))
	defer func() {
		step.End(err)
	}()

	taskStep := entity.Step{
		ID:          uuid.New(),
		Action:      string(action.Type),
		Description: s.formatActionDescription(action),
		Timestamp:   time.Now(),
	}

	fmt.Printf("🎬 Action: %s - %s\n", action.Type, taskStep.Description)

	currentURL := ""

	if state, err := s.browser.GetPageState(ctx); err == nil {
		currentURL = state.URL
	}

	if s.isDuplicateAction(action) {
		taskStep.Success = false
		taskStep.Error = "duplicate action detected"
		task.Steps = append(task.Steps, taskStep)

		*messages = append(*messages, entity.AIMessage{
			Role:    "user",
			Content: "This action failed on the previous attempt. Try a completely different approach.",
		})

		return apperr.WrapErrorWithReason(op, apperr.CodeDuplicateAction, "duplicate_action")
	}

	if s.shouldConfirm(action, currentURL) {
		if !s.requestUserConfirmation(action) {
			taskStep.Success = false
			taskStep.Error = "action cancelled by user"
			task.Steps = append(task.Steps, taskStep)

			*messages = append(*messages, entity.AIMessage{
				Role:    "user",
				Content: "Action was cancelled by user. Try a different approach.",
			})

			return apperr.WrapErrorWithReason(op, apperr.CodeCancelledByUser, "action_cancelled")
		}
	}

	result, screenshot, err := s.executeAction(ctx, action)
		if err != nil {
			logger.Error("Action failed", zap.Error(err))
			taskStep.Success = false
			taskStep.Error = err.Error()
			task.Steps = append(task.Steps, taskStep)

			s.lastAction = action

			errorMsg := fmt.Sprintf("Action '%s' failed: %v.", action.Type, err)
			
			if action.Type == entity.ActionTypeClick {
				errorMsg += " Use click_at_coordinates(x, y) with coordinates from the element list instead."
			}

			*messages = append(*messages, entity.AIMessage{
				Role:    "user",
				Content: errorMsg,
			})

			return err
		}

	s.lastAction = action
	taskStep.Success = true
	task.Steps = append(task.Steps, taskStep)

	if result != "" {
		if screenshot != nil && len(screenshot) > 0 {
			fmt.Printf("📸 Screenshot taken\n")
		}

		msg := s.createMessageWithScreenshot("user", result, screenshot)
		*messages = append(*messages, msg)
	}

	return nil
}

func (s *AgentService) isDuplicateAction(action *entity.BrowserAction) bool {
	if s.lastAction == nil {
		return false
	}

	if s.lastAction.Type != action.Type {
		return false
	}

	switch action.Type {
	case entity.ActionTypeNavigate:
		return s.lastAction.URL == action.URL
	case entity.ActionTypeClick:
		return s.lastAction.Selector == action.Selector
	case entity.ActionTypeFill:
		return s.lastAction.Selector == action.Selector && s.lastAction.Value == action.Value
	case entity.ActionTypeScroll:
		return s.lastAction.Value == action.Value && s.lastAction.WaitFor == action.WaitFor
	case entity.ActionTypeClickCoordinates:
		return s.lastAction.X == action.X && s.lastAction.Y == action.Y
	default:
		return false
	}
}

func (s *AgentService) shouldConfirm(action *entity.BrowserAction, currentURL string) bool {
	switch action.Type {
	case entity.ActionTypeFill:
		lower := strings.ToLower(action.Selector)
		lowerValue := strings.ToLower(action.Value)

		if strings.Contains(lower, "password") || 
		   strings.Contains(lower, "card") || 
		   strings.Contains(lower, "cvv") ||
		   strings.Contains(lower, "pin") ||
		   strings.Contains(lower, "code") && len(action.Value) <= 6 {
			return true
		}

		if strings.Contains(lowerValue, "delete") || 
		   strings.Contains(lowerValue, "remove") ||
		   strings.Contains(lowerValue, "удалить") {
			return true
		}
	case entity.ActionTypeClick:
		lower := strings.ToLower(action.Selector)
		urlLower := strings.ToLower(currentURL)

		if (strings.Contains(lower, "delete") || 
		    strings.Contains(lower, "remove") ||
		    strings.Contains(lower, "удалить") ||
		    strings.Contains(lower, "pay") ||
		    strings.Contains(lower, "оплат") ||
		    strings.Contains(lower, "купить") ||
		    strings.Contains(lower, "buy")) &&
		   (strings.Contains(urlLower, "payment") ||
		    strings.Contains(urlLower, "checkout") ||
		    strings.Contains(urlLower, "cart") ||
		    strings.Contains(urlLower, "оплата")) {
			return true
		}
	}

	return false
}

func (s *AgentService) requestUserConfirmation(action *entity.BrowserAction) bool {
	fmt.Printf("\n⚠️  Security confirmation required\n")
	fmt.Printf("Action: %s %s\n", action.Type, s.formatActionDescription(action))
	fmt.Print("Confirm (yes/no): ")

	scanner := bufio.NewScanner(os.Stdin)

	if scanner.Scan() {
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))

		return response == "yes" || response == "y"
	}

	return false
}

func (s *AgentService) formatActionDescription(action *entity.BrowserAction) string {
	switch action.Type {
	case entity.ActionTypeNavigate:
		return action.URL
	case entity.ActionTypeClick:
		return fmt.Sprintf("selector: %s", action.Selector)
	case entity.ActionTypeFill:
		return fmt.Sprintf("selector: %s, value: %s", action.Selector, action.Value)
	case entity.ActionTypePress:
		return fmt.Sprintf("key: %s", action.Value)
	case entity.ActionTypeWait:
		return fmt.Sprintf("%dms", action.WaitFor)
	case entity.ActionTypeScroll:
		direction := "down"
		amount := 500

		if action.Value != "" {
			direction = action.Value
		}

		if action.WaitFor > 0 {
			amount = action.WaitFor
		}

		return fmt.Sprintf("direction: %s, amount: %d", direction, amount)
	case entity.ActionTypeClickCoordinates:
		return fmt.Sprintf("x: %.0f, y: %.0f", action.X, action.Y)
	default:
		return ""
	}
}

func (s *AgentService) truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	return text[:maxLen] + "..."
}

func (s *AgentService) buildSystemPrompt(taskDescription string) string {
	var prompt strings.Builder

	prompt.WriteString("You are a browser automation agent. Complete tasks efficiently.\n\n")
	prompt.WriteString(fmt.Sprintf("Task: %s\n\n", taskDescription))

	prompt.WriteString(`Available actions:
- navigate(url)
- click_at_coordinates(x, y) - PRIMARY method, click at screen position
- click(selector) - backup method if coordinates not available
- fill(selector, value) - auto-submits search fields
- press(key)
- scroll(direction, amount)
- complete_task(result)

IMPORTANT RULES:
1. Clickable elements show: text | selector | coords (x,y) | size WxH
2. Coordinates are CENTER of element - use these for clicking
3. ALWAYS prefer click_at_coordinates(x,y) - it's more reliable
4. After EVERY click you get screenshot - check what happened
5. Look for [ICON_BUTTON], [*_CARD_*], [*_ITEM_*] patterns
6. Search fields auto-submit with Enter
7. NEVER repeat failed actions
8. Before completing - VERIFY result (check cart, confirmation, new elements)
9. Only complete when you SEE proof of success

Max 16 iterations.`)

	return prompt.String()
}
