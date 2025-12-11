package adapters

import (
	"ai-agent-task/internal/entity"
	"context"
)

type BrowserService interface {
	Launch(ctx context.Context) error
	Close(ctx context.Context) error
	Navigate(ctx context.Context, url string) error
	Click(ctx context.Context, selector string) error
	ClickAtCoordinates(ctx context.Context, x, y float64) error
	Fill(ctx context.Context, selector, value string) error
	Scroll(ctx context.Context, direction string, amount int) error
	WaitForSelector(ctx context.Context, selector string, timeout int) error
	GetElementText(ctx context.Context, selector string) (string, error)
	Screenshot(ctx context.Context, path string) error
	GetPageState(ctx context.Context) (*entity.PageState, error)
	GetElements(ctx context.Context) ([]entity.Element, error)
	EvaluateJS(ctx context.Context, script string) (interface{}, error)
	IsReady() bool
}

type AIService interface {
	SendMessage(ctx context.Context, messages []entity.AIMessage) (*entity.AIResponse, error)
	CreateTools() []interface{}
}

type AgentService interface {
	Execute(ctx context.Context, taskDescription string) (*entity.Task, error)
	Stop()
}
