// Package httpclient — базовый HTTP-клиент с interceptor-цепочкой.
// Замена platform-go/pkg/client: Intercept/Invoker/APIResp не нужны —
// интерсепторы работают на уровне *http.Request → *http.Response.
package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Client оборачивает *http.Client с base URL и цепочкой интерсепторов.
type Client struct {
	httpClient  *http.Client
	serverURL   *url.URL
	interceptor Interceptor
}

// Option настраивает Config.
type Option func(*Config)

// Config — внутренняя конфигурация, собираемая из Option-ов.
type Config struct {
	serverURL    *url.URL
	httpClient   *http.Client
	interceptors []Interceptor
}

// Invoker — финальный HTTP-вызов (без middleware).
type Invoker func(ctx context.Context, req *http.Request) (*http.Response, error)

// Interceptor — middleware, оборачивает Invoker. Может модифицировать
// *http.Request (auth, logging), инспектировать *http.Response (retry),
// или вообще не вызывать invoker (кеширование).
type Interceptor func(ctx context.Context, req *http.Request, invoker Invoker) (*http.Response, error)

// NewClient создаёт Client с base URL и опциями.
func NewClient(serverURL string, opts ...Option) (*Client, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("httpclient: parse server URL: %w", err)
	}

	cfg := &Config{serverURL: u}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.httpClient == nil {
		cfg.httpClient = http.DefaultClient
	}

	return &Client{
		httpClient:  cfg.httpClient,
		serverURL:   cfg.serverURL,
		interceptor: chainInterceptors(cfg.interceptors),
	}, nil
}

// Do прогоняет цепочку интерсепторов (если есть), затем финальный invoker,
// который отправляет запрос через *http.Client.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	final := Invoker(func(ctx context.Context, req *http.Request) (*http.Response, error) {
		return c.httpClient.Do(req.WithContext(ctx))
	})

	if c.interceptor != nil {
		return c.interceptor(ctx, req, final)
	}

	return final(ctx, req)
}

// ServerURL возвращает base URL сервера.
func (c *Client) ServerURL() *url.URL {
	return c.serverURL
}

// WithHTTPClient задаёт кастомный *http.Client (по умолчанию http.DefaultClient).
func WithHTTPClient(h *http.Client) Option {
	return func(cfg *Config) { cfg.httpClient = h }
}

// WithInterceptor добавляет интерсептор в цепочку. Несколько интерсепторов
// складываются: первый в цепочке вызывается первым, каждый вызывает следующий
// через invoker.
func WithInterceptor(i Interceptor) Option {
	return func(cfg *Config) { cfg.interceptors = append(cfg.interceptors, i) }
}

// chainInterceptors собирает слайс интерсепторов в один.
func chainInterceptors(interceptors []Interceptor) Interceptor {
	switch len(interceptors) {
	case 0:
		return nil
	case 1:
		return interceptors[0]
	}

	first := interceptors[0]
	rest := interceptors[1:]

	return func(ctx context.Context, req *http.Request, invoker Invoker) (*http.Response, error) {
		chain := buildChain(rest, invoker)

		return first(ctx, req, chain)
	}
}

// buildChain строит invoker, который прогоняет оставшиеся интерсепторы.
func buildChain(interceptors []Interceptor, final Invoker) Invoker {
	if len(interceptors) == 0 {
		return final
	}

	next := interceptors[0]
	rest := interceptors[1:]

	return func(ctx context.Context, req *http.Request) (*http.Response, error) {
		return next(ctx, req, buildChain(rest, final))
	}
}
