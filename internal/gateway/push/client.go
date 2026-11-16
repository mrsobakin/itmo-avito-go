package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/mrsobakin/itmo-avito-go/internal/model"
)

const (
	pushPath            = "/api/v1/push"
	kindRequestPosition = "REQUEST_POSITION"
	maxDrainedBody      = 64 << 10
)

type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("push service responded with status %d", e.StatusCode)
}

type pushRequest struct {
	RequestID   string            `json:"request_id"`
	RecipientID uuid.UUID         `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data"`
}

type Client struct {
	http *http.Client
	url  string
}

func New(baseURL string, timeout time.Duration, tracerProvider trace.TracerProvider) *Client {
	return &Client{
		http: &http.Client{
			Timeout: timeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport,
				otelhttp.WithTracerProvider(tracerProvider),
				otelhttp.WithMeterProvider(noop.NewMeterProvider()),
				otelhttp.WithPropagators(propagation.TraceContext{}),
				otelhttp.WithSpanNameFormatter(func(string, *http.Request) string { return "push.RequestPosition" }),
			),
		},
		url: baseURL + pushPath,
	}
}

func (c *Client) RequestPosition(ctx context.Context, value model.StaleTrip) error {
	body, err := json.Marshal(pushRequest{
		RequestID:   uuid.NewString(),
		RecipientID: value.DriverID,
		Kind:        kindRequestPosition,
		Data:        map[string]string{"trip_id": value.ID.String()},
	})
	if err != nil {
		return fmt.Errorf("encode push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send push request: %w", err)
	}

	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainedBody))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &StatusError{StatusCode: resp.StatusCode}
	}

	return nil
}

func (c *Client) Close() {
	c.http.CloseIdleConnections()
}
