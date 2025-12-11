package usecase

import (
	"ai-agent-task/internal/entity"
	"ai-agent-task/pkg/apperr"
	"ai-agent-task/pkg/logg"
	"ai-agent-task/pkg/tracing"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

func (s *AgentService) executeAction(ctx context.Context, action *entity.BrowserAction) (result string, screenshot []byte, err error) {
	const op = "executeAction"
	logger := s.logger.With(zap.String(logg.Operation, op), zap.String(logg.Action, string(action.Type)))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op,
		attribute.String("action_type", string(action.Type)))
	defer func() {
		step.End(err)
	}()

	switch action.Type {
	case entity.ActionTypeNavigate:
		return s.actionNavigate(ctx, action)
	case entity.ActionTypeClick:
		return s.actionClick(ctx, action)
	case entity.ActionTypeFill:
		return s.actionFill(ctx, action)
	case entity.ActionTypeWait:
		return s.actionWait(ctx, action)
	case entity.ActionTypeScroll:
		return s.actionScroll(ctx, action)
	case entity.ActionTypeClickCoordinates:
		return s.actionClickCoordinates(ctx, action)
	case entity.ActionTypePress:
		return s.actionPress(ctx, action)
	default:
		return "", nil, apperr.WrapErrorWithReason(op, apperr.CodeInvalidArgument, "unknown_action_type")
	}
}

func (s *AgentService) actionNavigate(ctx context.Context, action *entity.BrowserAction) (result string, screenshot []byte, err error) {
	const op = "actionNavigate"
	logger := s.logger.With(zap.String(logg.Operation, op), zap.String(logg.URL, action.URL))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op,
		attribute.String("url", action.URL))
	defer func() {
		step.End(err)
	}()

	if action.URL == "" {
		return "", nil, apperr.InvalidReqError(op, "url", fmt.Errorf("url cannot be empty"))
	}

	step.AddEvent("navigating to URL")

	if err := s.browser.Navigate(ctx, action.URL); err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason: "navigation_failed",
			apperr.MetaStage:  apperr.StageNavigation,
			apperr.MetaURL:    action.URL,
		})
	}

	step.AddEvent("getting page state")

	state, err := s.browser.GetPageState(ctx)
	if err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "page_state_failed",
			apperr.MetaStage:  apperr.StagePageState,
		})
	}

	s.lastURL = state.URL
	screenshot, _ = s.takeScreenshot(ctx)

	return s.optimizePageState(state), screenshot, nil
}

func (s *AgentService) actionClick(ctx context.Context, action *entity.BrowserAction) (result string, screenshot []byte, err error) {
	const op = "actionClick"
	logger := s.logger.With(zap.String(logg.Operation, op), zap.String(logg.Selector, action.Selector))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op,
		attribute.String("selector", action.Selector))
	defer func() {
		step.End(err)
	}()

	if action.Selector == "" {
		return "", nil, apperr.InvalidReqError(op, "selector", fmt.Errorf("selector cannot be empty"))
	}

	oldURL := s.lastURL

	step.AddEvent("clicking element")

	if err := s.browser.Click(ctx, action.Selector); err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason:   "click_failed",
			apperr.MetaStage:    apperr.StageInteraction,
			apperr.MetaSelector: action.Selector,
		})
	}

	step.AddEvent("getting page state")

	state, err := s.browser.GetPageState(ctx)
	if err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "page_state_failed",
			apperr.MetaStage:  apperr.StagePageState,
		})
	}

	newURL := state.URL
	s.lastURL = newURL

	if oldURL != newURL {
		screenshot, _ = s.takeScreenshot(ctx)
	}

	return s.optimizePageState(state), screenshot, nil
}

func (s *AgentService) actionFill(ctx context.Context, action *entity.BrowserAction) (result string, screenshot []byte, err error) {
	const op = "actionFill"
	logger := s.logger.With(zap.String(logg.Operation, op), zap.String(logg.Selector, action.Selector))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op,
		attribute.String("selector", action.Selector))
	defer func() {
		step.End(err)
	}()

	if action.Selector == "" {
		return "", nil, apperr.InvalidReqError(op, "selector", fmt.Errorf("selector cannot be empty"))
	}

	step.AddEvent("filling field")

	if err := s.browser.Fill(ctx, action.Selector, action.Value); err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason:   "fill_failed",
			apperr.MetaStage:    apperr.StageInteraction,
			apperr.MetaSelector: action.Selector,
		})
	}

	isSearchField := strings.Contains(strings.ToLower(action.Selector), "search") ||
		strings.Contains(strings.ToLower(action.Selector), "query") ||
		strings.Contains(strings.ToLower(action.Value), "поиск")

	if isSearchField {
		logger.Info("Auto-pressing Enter for search field")
		step.AddEvent("auto-pressing Enter for search")

		oldURL := s.lastURL

		if err := s.browser.Press(ctx, "Enter"); err != nil {
			logger.Warn("Failed to auto-press Enter", zap.Error(err))

			return "Field filled (Enter press failed).", nil, nil
		}

		time.Sleep(1500 * time.Millisecond)

		state, err := s.browser.GetPageState(ctx)
		if err != nil {
			return "Field filled and Enter pressed.", nil, nil
		}

		newURL := state.URL
		s.lastURL = newURL

		if oldURL != newURL {
			screenshot, _ = s.takeScreenshot(ctx)
		}

		return s.optimizePageState(state), screenshot, nil
	}

	return "Field filled.", nil, nil
}

func (s *AgentService) actionWait(ctx context.Context, action *entity.BrowserAction) (result string, screenshot []byte, err error) {
	const op = "actionWait"

	time.Sleep(time.Duration(action.WaitFor) * time.Millisecond)

	return "Wait completed", nil, nil
}

func (s *AgentService) actionScroll(ctx context.Context, action *entity.BrowserAction) (result string, screenshot []byte, err error) {
	const op = "actionScroll"
	logger := s.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	direction := "down"
	amount := 500

	if action.Value != "" {
		direction = action.Value
	}

	if action.WaitFor > 0 {
		amount = action.WaitFor
	}

	step.AddEvent("scrolling page")

	if err := s.browser.Scroll(ctx, direction, amount); err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason: "scroll_failed",
			apperr.MetaStage:  apperr.StageInteraction,
		})
	}

	step.AddEvent("getting page state")

	state, err := s.browser.GetPageState(ctx)
	if err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "page_state_failed",
			apperr.MetaStage:  apperr.StagePageState,
		})
	}

	return s.optimizePageState(state), nil, nil
}

func (s *AgentService) actionClickCoordinates(ctx context.Context, action *entity.BrowserAction) (result string, screenshot []byte, err error) {
	const op = "actionClickCoordinates"
	logger := s.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op,
		attribute.Float64("x", action.X),
		attribute.Float64("y", action.Y))
	defer func() {
		step.End(err)
	}()

	step.AddEvent("clicking at coordinates")

	if err := s.browser.ClickAtCoordinates(ctx, action.X, action.Y); err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason: "click_coordinates_failed",
			apperr.MetaStage:  apperr.StageInteraction,
		})
	}

	time.Sleep(800 * time.Millisecond)

	step.AddEvent("getting page state")

	state, err := s.browser.GetPageState(ctx)
	if err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "page_state_failed",
			apperr.MetaStage:  apperr.StagePageState,
		})
	}

	s.lastURL = state.URL

	screenshot, _ = s.takeScreenshot(ctx)

	return s.optimizePageState(state), screenshot, nil
}

func (s *AgentService) actionPress(ctx context.Context, action *entity.BrowserAction) (result string, screenshot []byte, err error) {
	const op = "actionPress"
	logger := s.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, s.tracer, logger, op,
		attribute.String("key", action.Value))
	defer func() {
		step.End(err)
	}()

	if action.Value == "" {
		return "", nil, apperr.InvalidReqError(op, "key", fmt.Errorf("key cannot be empty"))
	}

	oldURL := s.lastURL

	step.AddEvent("pressing key")

	if err := s.browser.Press(ctx, action.Value); err != nil {
		return "", nil, apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason: "press_failed",
			apperr.MetaStage:  apperr.StageInteraction,
		})
	}

	// For Enter key, get updated page state
	if action.Value == "Enter" {
		step.AddEvent("getting page state after Enter")

		state, err := s.browser.GetPageState(ctx)
		if err != nil {
			return "", nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
				apperr.MetaReason: "page_state_failed",
				apperr.MetaStage:  apperr.StagePageState,
			})
		}

		newURL := state.URL
		s.lastURL = newURL

		if oldURL != newURL {
			screenshot, _ = s.takeScreenshot(ctx)
		}

		return s.optimizePageState(state), screenshot, nil
	}

	return fmt.Sprintf("Pressed key: %s", action.Value), nil, nil
}

func (s *AgentService) takeScreenshot(ctx context.Context) ([]byte, error) {
	if !s.browser.IsReady() {
		return nil, fmt.Errorf("browser not ready")
	}

	tempPath := "/tmp/agent-screenshot.jpg"

	if err := s.browser.Screenshot(ctx, tempPath); err != nil {
		s.logger.Warn("Failed to take screenshot", zap.Error(err))

		return nil, err
	}

	data, err := os.ReadFile(tempPath)
	if err != nil {
		s.logger.Warn("Failed to read screenshot", zap.Error(err))

		return nil, err
	}

	os.Remove(tempPath)

	return data, nil
}

func (s *AgentService) optimizePageState(state *entity.PageState) string {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("URL: %s\n", state.URL))
	result.WriteString(fmt.Sprintf("Title: %s\n\n", state.Title))

	if len(state.Elements) == 0 {
		return result.String()
	}

	clickableElems := []entity.Element{}
	otherElems := []entity.Element{}

	for _, elem := range state.Elements {
		if elem.Clickable {
			clickableElems = append(clickableElems, elem)
		} else if elem.Text != "" && len(elem.Text) >= 3 {
			otherElems = append(otherElems, elem)
		}
	}

	result.WriteString("Clickable elements:\n")
	count := 0

	for _, elem := range clickableElems {
		if count >= 40 {
			break
		}

		text := elem.Text
		if len(text) > 200 {
			text = text[:200] + "..."
		}

		selector := elem.Selector
		if len(selector) > 100 {
			selector = selector[:100] + "..."
		}

		count++

		result.WriteString(fmt.Sprintf("%d. [%s] %s | selector: %s | coords: (%.0f,%.0f) size: %.0fx%.0f\n", 
			count, elem.Tag, text, selector, elem.BoundingBox.X, elem.BoundingBox.Y, elem.BoundingBox.Width, elem.BoundingBox.Height))
	}

	if len(otherElems) > 0 {
		result.WriteString("\nOther content:\n")
		otherCount := 0

		for _, elem := range otherElems {
			if otherCount >= 10 {
				break
			}

			text := elem.Text
			if len(text) > 200 {
				text = text[:200] + "..."
			}

			otherCount++
			result.WriteString(fmt.Sprintf("%d. [%s] %s\n", otherCount, elem.Tag, text))
		}
	}

	return result.String()
}

func (s *AgentService) createMessageWithScreenshot(role, text string, screenshot []byte) entity.AIMessage {
	if screenshot == nil || len(screenshot) == 0 {
		return entity.AIMessage{
			Role:    role,
			Content: text,
		}
	}

	content := []entity.MessageContent{
		{
			Type: "image",
			Source: &entity.ImageSource{
				Type:      "base64",
				MediaType: "image/jpeg",
				Data:      base64.StdEncoding.EncodeToString(screenshot),
			},
		},
		{
			Type: "text",
			Text: text,
		},
	}

	return entity.AIMessage{
		Role:    role,
		Content: content,
	}
}
