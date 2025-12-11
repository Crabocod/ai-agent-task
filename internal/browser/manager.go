package browser

import (
	"ai-agent-task/internal/config"
	"ai-agent-task/internal/entity"
	"ai-agent-task/pkg/apperr"
	"ai-agent-task/pkg/logg"
	"ai-agent-task/pkg/tracing"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	browserManagerName = "BrowserManager"
	browserTracer      = "browser.manager"
	maxRetries         = 3
	retryDelay         = 800 * time.Millisecond
	clickTimeout       = 15000
	waitTimeout        = 12000
)

type Manager struct {
	config         *config.Config
	logger         *zap.Logger
	tracer         trace.Tracer
	playwright     *playwright.Playwright
	browser        playwright.Browser
	browserContext playwright.BrowserContext
	page           playwright.Page
	ready          bool
}

type Params struct {
	fx.In

	Config *config.Config
	Logger *zap.Logger
}

func NewManager(params Params) *Manager {
	return &Manager{
		config: params.Config,
		logger: params.Logger.With(zap.String(logg.Layer, browserManagerName)),
		tracer: otel.Tracer(browserTracer),
		ready:  false,
	}
}

func (m *Manager) Launch(ctx context.Context) (err error) {
	const op = "Launch"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	logger.Info("Launching browser...")
	step.AddEvent("installing playwright")

	err = playwright.Install()
	if err != nil {
		return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "playwright_install_failed",
			apperr.MetaStage:  apperr.StageBrowser,
		})
	}

	step.AddEvent("starting playwright")

	pw, err := playwright.Run()
	if err != nil {
		return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "playwright_start_failed",
			apperr.MetaStage:  apperr.StageBrowser,
		})
	}
	m.playwright = pw

	if m.config.BrowserConfig.UserDataDir != "" {
		return m.launchPersistent(ctx)
	}

	return m.launchNew(ctx)
}

func (m *Manager) launchPersistent(ctx context.Context) (err error) {
	const op = "launchPersistent"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	logger.Info("Launching persistent browser context")

	userDataDir := m.config.BrowserConfig.UserDataDir

	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "mkdir_failed",
			apperr.MetaStage:  apperr.StageBrowser,
		})
	}

	options := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:          playwright.Bool(m.config.BrowserConfig.Headless),
		SlowMo:            playwright.Float(float64(m.config.BrowserConfig.SlowMo)),
		Viewport:          &playwright.Size{Width: 1920, Height: 1080},
		UserAgent:         playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"),
		AcceptDownloads:   playwright.Bool(true),
		JavaScriptEnabled: playwright.Bool(true),
		Locale:            playwright.String("ru-RU"),
		TimezoneId:        playwright.String("Europe/Moscow"),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-dev-shm-usage",
			"--no-sandbox",
			"--window-size=1920,1080",
			"--disable-web-security",
			"--disable-features=IsolateOrigins,site-per-process",
		},
		IgnoreHttpsErrors: playwright.Bool(true),
	}

	browserContext, err := m.playwright.Chromium.LaunchPersistentContext(userDataDir, options)
	if err != nil {
		return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "launch_persistent_failed",
			apperr.MetaStage:  apperr.StageBrowser,
		})
	}

	m.browserContext = browserContext

	pages := browserContext.Pages()

	if len(pages) > 0 {
		m.page = pages[0]
		logger.Info("Using existing page")
	} else {
		page, err := browserContext.NewPage()
		if err != nil {
			return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
				apperr.MetaReason: "new_page_failed",
				apperr.MetaStage:  apperr.StageBrowser,
			})
		}
		m.page = page
		logger.Info("Created new page")
	}

	m.ready = true
	logger.Info("Browser launched successfully")

	return nil
}

func (m *Manager) launchNew(ctx context.Context) (err error) {
	const op = "launchNew"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	logger.Info("Launching new browser")

	browserOptions := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(m.config.BrowserConfig.Headless),
		SlowMo:   playwright.Float(float64(m.config.BrowserConfig.SlowMo)),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
		},
	}

	browser, err := m.playwright.Chromium.Launch(browserOptions)
	if err != nil {
		return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "browser_launch_failed",
			apperr.MetaStage:  apperr.StageBrowser,
		})
	}
	m.browser = browser

	contextOptions := playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{
			Width:  1280,
			Height: 720,
		},
		UserAgent:         playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"),
		AcceptDownloads:   playwright.Bool(true),
		JavaScriptEnabled: playwright.Bool(true),
		Locale:            playwright.String("ru-RU"),
		TimezoneId:        playwright.String("Europe/Moscow"),
	}

	browserContext, err := browser.NewContext(contextOptions)
	if err != nil {
		return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "context_create_failed",
			apperr.MetaStage:  apperr.StageBrowser,
		})
	}

	m.browserContext = browserContext

	page, err := browserContext.NewPage()
	if err != nil {
		return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "page_create_failed",
			apperr.MetaStage:  apperr.StageBrowser,
		})
	}
	m.page = page

	m.ready = true
	logger.Info("Browser launched successfully")

	return nil
}

func (m *Manager) Close(ctx context.Context) (err error) {
	const op = "Close"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	logger.Info("Closing connection to browser...")

	if m.config.BrowserConfig.UserDataDir != "" {
		logger.Info("Persistent browser - keeping it open")
		m.ready = false
		logger.Info("Connection closed, browser still running")

		return nil
	}

	logger.Info("Non-persistent browser - closing completely")

	if m.browserContext != nil {
		if err := m.browserContext.Close(); err != nil {
			logger.Warn("Failed to close context", zap.Error(err))
		}
	}

	if m.browser != nil {
		if err := m.browser.Close(); err != nil {
			logger.Warn("Failed to close browser", zap.Error(err))
		}
	}

	if m.playwright != nil {
		if err := m.playwright.Stop(); err != nil {
			return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
				apperr.MetaReason: "playwright_stop_failed",
			})
		}
	}

	m.ready = false
	logger.Info("Browser closed")

	return nil
}

func (m *Manager) ensurePageActive(ctx context.Context) error {
	if m.browserContext == nil {
		return fmt.Errorf("browser context is nil")
	}

	if m.page != nil && !m.page.IsClosed() {
		return nil
	}

	m.logger.Info("Page closed, reconnecting to active page...")

	pages := m.browserContext.Pages()

	if len(pages) > 0 {
		for _, p := range pages {
			if !p.IsClosed() {
				m.page = p
				m.logger.Info("Reconnected to existing page")

				return nil
			}
		}
	}

	m.logger.Info("No active pages found, creating new page...")

	page, err := m.browserContext.NewPage()
	if err != nil {
		return fmt.Errorf("failed to create new page: %w", err)
	}

	m.page = page
	m.logger.Info("Created new page")

	return nil
}

func (m *Manager) Navigate(ctx context.Context, url string) (err error) {
	const op = "Navigate"
	logger := m.logger.With(zap.String(logg.Operation, op), zap.String(logg.URL, url))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op, attribute.String("url", url))
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	step.AddEvent("navigating to URL")

	_, err = m.page.Goto(url, playwright.PageGotoOptions{
		Timeout:   playwright.Float(float64(m.config.BrowserConfig.Timeout)),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})

	if err != nil {
		return apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason: "goto_failed",
			apperr.MetaStage:  apperr.StageNavigation,
			apperr.MetaURL:    url,
		})
	}

	time.Sleep(500 * time.Millisecond)
	step.AddEvent("navigation completed")

	return nil
}

func (m *Manager) Click(ctx context.Context, selector string) (err error) {
	const op = "Click"
	logger := m.logger.With(zap.String(logg.Operation, op), zap.String(logg.Selector, selector))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op, attribute.String("selector", selector))
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	var lastErr error
	strategies := []struct {
		name string
		fn   func() error
	}{
		{
			name: "wait_and_click",
			fn: func() error {
				result, err := m.page.Evaluate(fmt.Sprintf(`
					(() => {
						const el = document.querySelector('%s');
						if (!el) return {success: false, error: 'element not found'};
						
						const rect = el.getBoundingClientRect();
						const style = window.getComputedStyle(el);
						
						const isVisible = (
							rect.width > 0 && 
							rect.height > 0 && 
							style.display !== 'none' && 
							style.visibility !== 'hidden' &&
							parseFloat(style.opacity) > 0
						);
						
						if (!isVisible) return {success: false, error: 'element not visible'};
						
						el.scrollIntoView({behavior: 'instant', block: 'center'});
						
						return {success: true};
					})()
				`, escapeSelector(selector)))
				
				if err != nil {
					return fmt.Errorf("visibility check failed: %w", err)
				}

				if resultMap, ok := result.(map[string]interface{}); ok {
					if success, ok := resultMap["success"].(bool); ok && !success {
						if errMsg, ok := resultMap["error"].(string); ok {
							return fmt.Errorf("element check failed: %s", errMsg)
						}
					}
				}

				time.Sleep(300 * time.Millisecond)

				err = m.page.Click(selector, playwright.PageClickOptions{
					Timeout: playwright.Float(clickTimeout),
				})
				if err != nil {
					return fmt.Errorf("click failed: %w", err)
				}

				return nil
			},
		},
		{
			name: "force_click",
			fn: func() error {
				_, err := m.page.Evaluate(fmt.Sprintf(`
					(() => {
						const el = document.querySelector('%s');
						if (el) {
							el.scrollIntoView({behavior: 'instant', block: 'center'});
						}
					})()
				`, escapeSelector(selector)))
				
				if err == nil {
					time.Sleep(300 * time.Millisecond)
				}

				err = m.page.Click(selector, playwright.PageClickOptions{
					Timeout: playwright.Float(clickTimeout),
					Force:   playwright.Bool(true),
				})
				if err != nil {
					return fmt.Errorf("force click failed: %w", err)
				}

				return nil
			},
		},
		{
			name: "js_direct_click",
			fn: func() error {
				result, err := m.page.Evaluate(fmt.Sprintf(`
					(() => {
						const el = document.querySelector('%s');
						if (!el) return {success: false, error: 'element not found'};
						
						el.scrollIntoView({behavior: 'instant', block: 'center'});
						
						return new Promise((resolve) => {
							setTimeout(() => {
								try {
									el.click();
									resolve({success: true});
								} catch(e) {
									resolve({success: false, error: e.message});
								}
							}, 200);
						});
					})()
				`, escapeSelector(selector)))
				
				if err != nil {
					return fmt.Errorf("js evaluation failed: %w", err)
				}

				if resultMap, ok := result.(map[string]interface{}); ok {
					if success, ok := resultMap["success"].(bool); ok && !success {
						if errMsg, ok := resultMap["error"].(string); ok {
							return fmt.Errorf("js click failed: %s", errMsg)
						}
					}
				}

				time.Sleep(300 * time.Millisecond)

				return nil
			},
		},
		{
			name: "mouse_click",
			fn: func() error {
				result, err := m.page.Evaluate(fmt.Sprintf(`
					(() => {
						const el = document.querySelector('%s');
						if (!el) return {success: false, error: 'element not found'};
						
						el.scrollIntoView({behavior: 'instant', block: 'center'});
						
						const rect = el.getBoundingClientRect();
						return {
							success: true,
							x: rect.left + rect.width / 2,
							y: rect.top + rect.height / 2
						};
					})()
				`, escapeSelector(selector)))
				
				if err != nil {
					return fmt.Errorf("coordinate calculation failed: %w", err)
				}

				resultMap, ok := result.(map[string]interface{})
				if !ok {
					return fmt.Errorf("invalid result format")
				}

				if success, ok := resultMap["success"].(bool); !ok || !success {
					if errMsg, ok := resultMap["error"].(string); ok {
						return fmt.Errorf("element check failed: %s", errMsg)
					}
					return fmt.Errorf("element check failed")
				}

				x, okX := resultMap["x"].(float64)
				y, okY := resultMap["y"].(float64)
				if !okX || !okY {
					return fmt.Errorf("invalid coordinates")
				}

				time.Sleep(300 * time.Millisecond)

				err = m.page.Mouse().Click(x, y)
				if err != nil {
					return fmt.Errorf("mouse click failed: %w", err)
				}

				return nil
			},
		},
	}

	for attemptNum := 0; attemptNum <= maxRetries; attemptNum++ {
		if attemptNum > 0 {
			logger.Info("Retrying click with different strategy", zap.Int("attempt", attemptNum))
			time.Sleep(retryDelay)
		}

		strategyIndex := attemptNum
		if strategyIndex >= len(strategies) {
			strategyIndex = len(strategies) - 1
		}

		strategy := strategies[strategyIndex]
		step.AddEvent(fmt.Sprintf("trying strategy: %s (attempt %d)", strategy.name, attemptNum+1))

		err = strategy.fn()
		if err == nil {
			time.Sleep(300 * time.Millisecond)
			step.AddEvent("click completed")

			return nil
		}

		lastErr = err
		logger.Warn("Strategy failed", zap.String("strategy", strategy.name), zap.Error(err))
	}

	return apperr.Wrap(op, apperr.CodeActionFailed, lastErr, map[string]any{
		apperr.MetaReason:   "click_failed_all_strategies",
		apperr.MetaStage:    apperr.StageInteraction,
		apperr.MetaSelector: selector,
	})
}

func escapeSelector(selector string) string {
	return strings.ReplaceAll(selector, "'", "\\'")
}

func (m *Manager) ClickAtCoordinates(ctx context.Context, x, y float64) (err error) {
	const op = "ClickAtCoordinates"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op,
		attribute.Float64("x", x),
		attribute.Float64("y", y))
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	step.AddEvent("clicking at coordinates")

	err = m.page.Mouse().Click(x, y)
	if err != nil {
		return apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason: "click_coordinates_failed",
			apperr.MetaStage:  apperr.StageInteraction,
		})
	}

	time.Sleep(300 * time.Millisecond)
	step.AddEvent("click completed")

	return nil
}

func (m *Manager) Fill(ctx context.Context, selector, value string) (err error) {
	const op = "Fill"
	logger := m.logger.With(zap.String(logg.Operation, op), zap.String(logg.Selector, selector))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op, attribute.String("selector", selector))
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logger.Info("Retrying fill", zap.Int("attempt", attempt))
			time.Sleep(retryDelay)
		}

		step.AddEvent(fmt.Sprintf("waiting for element (attempt %d)", attempt+1))

		_, err = m.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{
			Timeout: playwright.Float(5000),
			State:   playwright.WaitForSelectorStateVisible,
		})

		if err != nil {
			lastErr = err
			continue
		}

		step.AddEvent(fmt.Sprintf("filling field (attempt %d)", attempt+1))

		if attempt > 0 {
			m.page.Fill(selector, "", playwright.PageFillOptions{
				Timeout: playwright.Float(5000),
			})
			time.Sleep(200 * time.Millisecond)
		}

		err = m.page.Fill(selector, value, playwright.PageFillOptions{
			Timeout: playwright.Float(5000),
			Force:   playwright.Bool(attempt > 0),
		})

		if err == nil {
			time.Sleep(300 * time.Millisecond)
			step.AddEvent("fill completed")

			return nil
		}

		lastErr = err
	}

	return apperr.Wrap(op, apperr.CodeActionFailed, lastErr, map[string]any{
		apperr.MetaReason:   "fill_failed_after_retries",
		apperr.MetaStage:    apperr.StageInteraction,
		apperr.MetaSelector: selector,
	})
}

func (m *Manager) Press(ctx context.Context, key string) (err error) {
	const op = "Press"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op,
		attribute.String("key", key))
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	step.AddEvent("pressing key")

	err = m.page.Keyboard().Press(key)
	if err != nil {
		return apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason: "press_failed",
			apperr.MetaStage:  apperr.StageInteraction,
		})
	}

	if key == "Enter" {
		time.Sleep(1 * time.Second)
	} else {
		time.Sleep(300 * time.Millisecond)
	}

	step.AddEvent("press completed")

	return nil
}

func (m *Manager) Scroll(ctx context.Context, direction string, amount int) (err error) {
	const op = "Scroll"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op,
		attribute.String("direction", direction),
		attribute.Int("amount", amount))
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	var script string

	if direction == "down" {
		script = fmt.Sprintf("window.scrollBy(0, %d)", amount)
	} else if direction == "up" {
		script = fmt.Sprintf("window.scrollBy(0, -%d)", amount)
	} else if direction == "bottom" {
		script = "window.scrollTo(0, document.body.scrollHeight)"
	} else if direction == "top" {
		script = "window.scrollTo(0, 0)"
	}

	step.AddEvent("scrolling page")

	_, err = m.page.Evaluate(script)
	if err != nil {
		return apperr.Wrap(op, apperr.CodeActionFailed, err, map[string]any{
			apperr.MetaReason: "scroll_failed",
			apperr.MetaStage:  apperr.StageInteraction,
		})
	}

	time.Sleep(500 * time.Millisecond)
	step.AddEvent("scroll completed")

	return nil
}

func (m *Manager) WaitForSelector(ctx context.Context, selector string, timeout int) (err error) {
	const op = "WaitForSelector"
	logger := m.logger.With(zap.String(logg.Operation, op), zap.String(logg.Selector, selector))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op, attribute.String("selector", selector))
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	_, err = m.page.WaitForSelector(selector, playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(float64(timeout)),
	})

	if err != nil {
		return apperr.Wrap(op, apperr.CodeTimeout, err, map[string]any{
			apperr.MetaReason:   "wait_selector_timeout",
			apperr.MetaSelector: selector,
		})
	}

	return nil
}

func (m *Manager) GetElementText(ctx context.Context, selector string) (text string, err error) {
	const op = "GetElementText"
	logger := m.logger.With(zap.String(logg.Operation, op), zap.String(logg.Selector, selector))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op, attribute.String("selector", selector))
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return "", apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return "", apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	element, err := m.page.QuerySelector(selector)
	if err != nil {
		return "", apperr.Wrap(op, apperr.CodeNotFound, err, map[string]any{
			apperr.MetaReason:   "element_not_found",
			apperr.MetaSelector: selector,
		})
	}

	if element == nil {
		return "", apperr.NotFoundError(op, fmt.Errorf("element not found: %s", selector))
	}

	text, err = element.TextContent()
	if err != nil {
		return "", apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "text_content_failed",
		})
	}

	return text, nil
}

func (m *Manager) Screenshot(ctx context.Context, path string) (err error) {
	const op = "Screenshot"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	_, err = m.page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(path),
		FullPage: playwright.Bool(false),
		Type:     playwright.ScreenshotTypeJpeg,
		Quality:  playwright.Int(60),
	})

	if err != nil {
		return apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "screenshot_failed",
			apperr.MetaStage:  apperr.StageScreenshot,
		})
	}

	return nil
}

func (m *Manager) GetPageState(ctx context.Context) (state *entity.PageState, err error) {
	const op = "GetPageState"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return nil, apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return nil, apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	url := m.page.URL()
	title, _ := m.page.Title()

	elements, err := m.GetElements(ctx)
	if err != nil {
		logger.Warn("Failed to get elements", zap.Error(err))
		elements = []entity.Element{}
	}

	return &entity.PageState{
		URL:       url,
		Title:     title,
		Elements:  elements,
		Timestamp: time.Now(),
	}, nil
}

func (m *Manager) GetElements(ctx context.Context) (elements []entity.Element, err error) {
	const op = "GetElements"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return nil, apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return nil, apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	m.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateDomcontentloaded,
		Timeout: playwright.Float(5000),
	})

	script := getElementsScript()

	result, err := m.page.Evaluate(script)
	if err != nil {
		return nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "evaluate_failed",
		})
	}

	elementsList, ok := result.([]interface{})
	if !ok {
		return nil, apperr.WrapErrorWithReason(op, apperr.CodeInternal, "unexpected_result_type")
	}

	elements = make([]entity.Element, 0, len(elementsList))

	for _, item := range elementsList {
		elemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		elem := entity.Element{
			Tag:        getString(elemMap, "tag"),
			Text:       strings.TrimSpace(getString(elemMap, "text")),
			Selector:   getString(elemMap, "selector"),
			Visible:    getBool(elemMap, "visible"),
			Clickable:  getBool(elemMap, "clickable"),
			Attributes: make(map[string]string),
			BoundingBox: entity.BoundingBox{
				X:      getFloat(elemMap, "x"),
				Y:      getFloat(elemMap, "y"),
				Width:  getFloat(elemMap, "width"),
				Height: getFloat(elemMap, "height"),
			},
		}

		if attrs, ok := elemMap["attributes"].(map[string]interface{}); ok {
			for k, v := range attrs {
				if str, ok := v.(string); ok {
					elem.Attributes[k] = str
				}
			}
		}

		elements = append(elements, elem)
	}

	return elements, nil
}

func (m *Manager) EvaluateJS(ctx context.Context, script string) (result interface{}, err error) {
	const op = "EvaluateJS"
	logger := m.logger.With(zap.String(logg.Operation, op))

	ctx, step := tracing.StartSpan(ctx, m.tracer, logger, op)
	defer func() {
		step.End(err)
	}()

	if !m.ready {
		return nil, apperr.WrapErrorWithReason(op, apperr.CodeBrowserNotReady, "browser_not_ready")
	}

	if err := m.ensurePageActive(ctx); err != nil {
		return nil, apperr.Wrap(op, apperr.CodeBrowserNotReady, err, map[string]any{
			apperr.MetaReason: "page_not_active",
		})
	}

	result, err = m.page.Evaluate(script)
	if err != nil {
		return nil, apperr.Wrap(op, apperr.CodeInternal, err, map[string]any{
			apperr.MetaReason: "evaluate_failed",
		})
	}

	return result, nil
}

func (m *Manager) IsReady() bool {
	return m.ready
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}

	return ""
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}

	return false
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}

	if v, ok := m[key].(int); ok {
		return float64(v)
	}

	return 0
}
