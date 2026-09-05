package engine

import (
	"context"
	"fmt"
	"time"
)

type IOErrorCode string

const (
	ErrNotFound    IOErrorCode = "not-found"
	ErrDenied      IOErrorCode = "denied"
	ErrIOFailure   IOErrorCode = "io-failure"
	ErrUnavailable IOErrorCode = "unavailable"
	ErrTimeout     IOErrorCode = "timeout"
)

type IOError struct {
	Code    IOErrorCode `json:"code"`
	Target  string      `json:"target"`
	Message string      `json:"message"`
}

func (e *IOError) Error() string {
	return fmt.Sprintf("%s %s: %s", e.Code, e.Target, e.Message)
}

type ServiceState string

const (
	ServiceRunning ServiceState = "running"
	ServiceStopped ServiceState = "stopped"
	ServiceFailed  ServiceState = "failed"
)

type FileSystem interface {
	Read(ctx context.Context, path string) (string, error)
	Write(ctx context.Context, path, contents string) error
	Remove(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) (bool, error)
}

type ServiceHost interface {
	Restart(ctx context.Context, service string) error
	Status(ctx context.Context, service string) (ServiceState, error)
}

type Clock interface {
	Now() time.Time
	NextID() string
}

type LogEvent struct {
	TransactionID string    `json:"transactionId"`
	At            time.Time `json:"at"`
	Kind          string    `json:"kind"`
	Detail        string    `json:"detail"`
}

type Logger interface {
	Log(event LogEvent)
}

type Ports struct {
	FS       FileSystem
	Services ServiceHost
	Clock    Clock
	Logger   Logger
}

func (p Ports) log(transactionID, kind, detail string) {
	if p.Logger == nil {
		return
	}
	p.Logger.Log(LogEvent{
		TransactionID: transactionID,
		At:            p.Clock.Now(),
		Kind:          kind,
		Detail:        detail,
	})
}
