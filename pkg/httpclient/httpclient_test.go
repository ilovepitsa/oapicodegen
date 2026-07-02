package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_Defaults(t *testing.T) {
	c, err := NewClient("https://api.example.com")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com", c.ServerURL().String())
	assert.NotNil(t, c.httpClient)
	assert.Equal(t, http.DefaultClient, c.httpClient)
	assert.Nil(t, c.interceptor)
}

func TestNewClient_InvalidURL(t *testing.T) {
	_, err := NewClient("://invalid")
	assert.Error(t, err)
}

func TestNewClient_WithHTTPClient(t *testing.T) {
	custom := &http.Client{}
	c, err := NewClient("https://example.com", WithHTTPClient(custom))
	require.NoError(t, err)
	assert.Equal(t, custom, c.httpClient)
}

func TestClient_Do_NoInterceptor(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	resp := doGet(t, c, srv.URL)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestClient_Do_SingleInterceptor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	authIntercept := func(ctx context.Context, req *http.Request, invoker Invoker) (*http.Response, error) {
		req.Header.Set("Authorization", "Bearer token")
		return invoker(ctx, req)
	}

	c, err := NewClient(srv.URL, WithInterceptor(authIntercept))
	require.NoError(t, err)

	resp := doGet(t, c, srv.URL)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestClient_Do_InterceptorChain(t *testing.T) {
	var order []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "server")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	first := func(ctx context.Context, req *http.Request, invoker Invoker) (*http.Response, error) {
		order = append(order, "first-before")
		resp, err := invoker(ctx, req)
		order = append(order, "first-after")
		return resp, err
	}

	second := func(ctx context.Context, req *http.Request, invoker Invoker) (*http.Response, error) {
		order = append(order, "second-before")
		resp, err := invoker(ctx, req)
		order = append(order, "second-after")
		return resp, err
	}

	c, err := NewClient(srv.URL, WithInterceptor(first), WithInterceptor(second))
	require.NoError(t, err)

	resp := doGet(t, c, srv.URL)
	_ = resp.Body.Close()

	assert.Equal(t, []string{
		"first-before",
		"second-before",
		"server",
		"second-after",
		"first-after",
	}, order)
}

func TestClient_Do_InterceptorShortCircuit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer srv.Close()

	blocking := func(ctx context.Context, req *http.Request, _ Invoker) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTeapot}, nil
	}

	c, err := NewClient(srv.URL, WithInterceptor(blocking))
	require.NoError(t, err)

	resp := doGet(t, c, srv.URL)
	assert.Equal(t, http.StatusTeapot, resp.StatusCode)
}

func doGet(t *testing.T, c *Client, baseURL string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/test", nil)
	require.NoError(t, err)
	resp, err := c.Do(context.Background(), req)
	require.NoError(t, err)
	return resp
}
