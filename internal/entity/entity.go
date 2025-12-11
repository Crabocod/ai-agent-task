package entity

import (
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID          uuid.UUID
	Description string
	Status      TaskStatus
	CreatedAt   time.Time
	CompletedAt *time.Time
	Steps       []Step
	Result      string
	Error       string
}

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

type Step struct {
	ID          uuid.UUID
	Action      string
	Description string
	Timestamp   time.Time
	Success     bool
	Error       string
	Screenshot  string
}

type BrowserAction struct {
	Type       ActionType
	Selector   string
	Value      string
	URL        string
	WaitFor    int
	X          float64
	Y          float64
	Screenshot bool
}

type ActionType string

const (
	ActionTypeNavigate         ActionType = "navigate"
	ActionTypeClick            ActionType = "click"
	ActionTypeClickCoordinates ActionType = "click_coordinates"
	ActionTypeFill             ActionType = "fill"
	ActionTypeSelect           ActionType = "select"
	ActionTypeWait             ActionType = "wait"
	ActionTypeScreenshot       ActionType = "screenshot"
	ActionTypeGetAttribute     ActionType = "get_attribute"
	ActionTypeScroll           ActionType = "scroll"
	ActionTypeHover            ActionType = "hover"
	ActionTypePress            ActionType = "press"
)

type PageState struct {
	URL        string
	Title      string
	HTML       string
	Screenshot string
	Elements   []Element
	Timestamp  time.Time
}

type Element struct {
	Tag         string
	Text        string
	Selector    string
	Type        string
	Attributes  map[string]string
	Visible     bool
	Clickable   bool
	BoundingBox BoundingBox
}

type BoundingBox struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type MessageContent struct {
	Type   string        `json:"type"`
	Text   string        `json:"text,omitempty"`
	Source *ImageSource  `json:"source,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type AIMessage struct {
	Role    string
	Content interface{}
}

type AIResponse struct {
	Action   *BrowserAction
	Thought  string
	NextStep string
	Complete bool
	Result   string
}

type PageContext struct {
	URL         string
	Title       string
	Content     string
	VisibleText string
	Elements    []Element
	Screenshot  []byte
}
