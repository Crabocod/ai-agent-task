package ai

import (
	"ai-agent-task/internal/config"
	"ai-agent-task/internal/entity"
	"ai-agent-task/pkg/apperr"
	"ai-agent-task/pkg/logg"
	"ai-agent-task/pkg/tracing"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	aiClientName = "AIClient"
	aiTracer     = "ai.client"
)

type Client struct {
	config     *config.Config
	logger     *zap.Logger
	tracer     trace.Tracer
	httpClient *http.Client
}

type Params struct {
	fx.In

	Config *config.Config
	Logger *zap.Logger
}

func NewClient(params Params) *Client {
	return &Client{
		config:     params.Config,
		logger:     params.Logger.With(zap.String(logg.Layer, aiClientName)),
		tracer:     otel.Tracer(aiTracer),
		httpClient: &http.Client{},
	}
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
	Tools     []claudeTool    `json:"tools,omitempty"`
}

type claudeMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type claudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type claudeResponse struct {
	Content []struct {
		Type  string                 `json:"type"`
		Text  string                 `json:"text,omitempty"`
		Name  string                 `json:"name,omitempty"`
		Input map[string]interface{} `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

func (c *Client) SendMessage(ctx context.Context, messages []entity.AIMessage) (resp *entity.AIResponse, err error) {
	const op = "SendMessage"
	logger := c.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, c.tracer, logger, op,
		attribute.Int("messages_count", len(messages)))
	defer func() {
		step.End(err)
	}()

	logger.Debug("Sending message to AI", zap.Int("messages_count", len(messages)))

	claudeMessages := make([]claudeMessage, len(messages))
	for i, msg := range messages {
		claudeMessages[i] = claudeMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	reqBody := claudeRequest{
		Model:     c.config.AIConfig.Model,
		MaxTokens: 4096,
		Messages:  claudeMessages,
		Tools:     c.createTools(),
	}

	step.AddEvent("marshaling request")

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "marshal_failed",
			apperr.MetaStage:  apperr.StageAI,
		})
	}

	step.AddEvent("creating HTTP request")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "request_create_failed",
			apperr.MetaStage:  apperr.StageAI,
		})
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.AIConfig.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	step.AddEvent("sending HTTP request")

	resp_http, err := c.httpClient.Do(req)
	if err != nil {
		return nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "http_request_failed",
			apperr.MetaStage:  apperr.StageAI,
		})
	}
	defer resp_http.Body.Close()

	step.AddEvent("reading response")

	body, err := io.ReadAll(resp_http.Body)
	if err != nil {
		return nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "read_body_failed",
			apperr.MetaStage:  apperr.StageAI,
		})
	}

	if resp_http.StatusCode != http.StatusOK {
		return nil, apperr.Wrap(op, apperr.CodeAIError, fmt.Errorf("API error (status %d): %s", resp_http.StatusCode, string(body)), map[string]any{
			apperr.MetaReason: "api_error",
			apperr.MetaStage:  apperr.StageAI,
			"status_code":     resp_http.StatusCode,
		})
	}

	step.AddEvent("unmarshaling response")

	var claudeResp claudeResponse

	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "unmarshal_failed",
			apperr.MetaStage:  apperr.StageAI,
		})
	}

	step.AddEvent("parsing response")

	aiResp, err := c.parseResponse(&claudeResp)
	if err != nil {
		return nil, err
	}

	step.AddEvent("message sent successfully")

	return aiResp, nil
}

func (c *Client) createTools() []claudeTool {
	return []claudeTool{
		{
			Name:        "navigate",
			Description: "Navigate to URL",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "click",
			Description: "Click element. Prefer [data-qa] selectors!",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"selector"},
			},
		},
		{
			Name:        "click_at_coordinates",
			Description: "Click at X,Y when selector fails",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"x": map[string]interface{}{
						"type": "number",
					},
					"y": map[string]interface{}{
						"type": "number",
					},
				},
				"required": []string{"x", "y"},
			},
		},
		{
			Name:        "fill",
			Description: "Fill input",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"selector": map[string]interface{}{
						"type": "string",
					},
					"value": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"selector", "value"},
			},
		},
		{
			Name:        "press",
			Description: "Press keyboard key (e.g. Enter, Escape, Tab)",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"key": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"key"},
			},
		},
		{
			Name:        "scroll",
			Description: "Scroll: down/up/bottom/top",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"direction": map[string]interface{}{
						"type": "string",
						"enum": []string{"down", "up", "bottom", "top"},
					},
					"amount": map[string]interface{}{
						"type":    "number",
						"default": 500,
					},
				},
				"required": []string{"direction"},
			},
		},
		{
			Name:        "complete_task",
			Description: "Complete with result",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"result": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"result"},
			},
		},
	}
}

func (c *Client) parseResponse(resp *claudeResponse) (*entity.AIResponse, error) {
	aiResp := &entity.AIResponse{
		Complete: resp.StopReason == "end_turn",
	}

	for _, content := range resp.Content {
		switch content.Type {
		case "text":
			aiResp.Thought = content.Text
		case "tool_use":
			action, err := c.parseToolUse(content.Name, content.Input)
			if err != nil {
				return nil, err
			}
			aiResp.Action = action

			if content.Name == "complete_task" {
				aiResp.Complete = true

				if result, ok := content.Input["result"].(string); ok {
					aiResp.Result = result
				}
			}
		}
	}

	return aiResp, nil
}

func (c *Client) parseToolUse(toolName string, input map[string]interface{}) (*entity.BrowserAction, error) {
	action := &entity.BrowserAction{}

	switch toolName {
	case "navigate":
		action.Type = entity.ActionTypeNavigate

		if url, ok := input["url"].(string); ok {
			action.URL = url
		}
	case "click":
		action.Type = entity.ActionTypeClick

		if selector, ok := input["selector"].(string); ok {
			action.Selector = selector
		}
	case "click_at_coordinates":
		action.Type = entity.ActionTypeClickCoordinates

		if x, ok := input["x"].(float64); ok {
			action.X = x
		}

		if y, ok := input["y"].(float64); ok {
			action.Y = y
		}
	case "fill":
		action.Type = entity.ActionTypeFill

		if selector, ok := input["selector"].(string); ok {
			action.Selector = selector
		}

		if value, ok := input["value"].(string); ok {
			action.Value = value
		}
	case "press":
		action.Type = entity.ActionTypePress

		if key, ok := input["key"].(string); ok {
			action.Value = key
		}
	case "scroll":
		action.Type = entity.ActionTypeScroll

		if direction, ok := input["direction"].(string); ok {
			action.Value = direction
		}

		if amount, ok := input["amount"].(float64); ok {
			action.WaitFor = int(amount)
		} else {
			action.WaitFor = 500
		}
	case "wait":
		action.Type = entity.ActionTypeWait

		if seconds, ok := input["seconds"].(float64); ok {
			action.WaitFor = int(seconds * 1000)
		}
	case "complete_task":

		return nil, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	return action, nil
}

func (c *Client) CreateTools() []interface{} {
	return []interface{}{
		c.createTools(),
	}
}
