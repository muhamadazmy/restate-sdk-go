package restate

import (
	"context"
	"time"

	"github.com/muhamadazmy/restate-sdk-go/generated/proto/dynrpc"
)

type Call interface {
	// Do makes a call and wait for the response
	Do(key string, body any) ([]byte, error)
	// Send runs a call in the background after delay duration
	Send(key string, body any, delay time.Duration) error
}

type Service interface {
	// Method creates a call to method
	Method(method string) Call
}

type Context interface {
	Ctx() context.Context
	// Set stores state value
	Set(key string, value []byte) error
	// Get a state value associated with key
	Get(key string) ([]byte, error)
	// Clear deletes a key
	Clear(key string) error
	// ClearAll drops all stored state associated with key
	ClearAll() error

	Keys() ([]string, error)

	Sleep(until time.Time) error

	Service(service string) Service
}

// UnKeyedHandlerFn signature of `un-keyed` handler function
type UnKeyedHandlerFn[I any, O any] func(ctx Context, input I) (output O, err error)

// KeyedHandlerFn signature for `keyed` handler function
type KeyedHandlerFn[I any, O any] func(ctx Context, key string, input I) (output O, err error)

// Handler interface.
type Handler interface {
	Call(ctx Context, request *dynrpc.RpcRequest) (output *dynrpc.RpcResponse, err error)
	sealed()
}

type Router interface {
	Keyed() bool
	Handlers() map[string]Handler
}

type UnKeyedRouter struct {
	handlers map[string]Handler
}

func NewUnKeyedRouter() *UnKeyedRouter {
	return &UnKeyedRouter{
		handlers: make(map[string]Handler),
	}
}

func (r *UnKeyedRouter) Handler(name string, handler *UnKeyedHandler) *UnKeyedRouter {
	r.handlers[name] = handler
	return r
}

func (r *UnKeyedRouter) Keyed() bool {
	return false
}

func (r *UnKeyedRouter) Handlers() map[string]Handler {
	return r.handlers
}

type KeyedRouter struct {
	handlers map[string]Handler
}

func NewKeyedRouter() *KeyedRouter {
	return &KeyedRouter{
		handlers: make(map[string]Handler),
	}
}

func (r *KeyedRouter) Handler(name string, handler *KeyedHandler) *KeyedRouter {
	r.handlers[name] = handler
	return r
}

func (r *KeyedRouter) Keyed() bool {
	return true
}

func (r *KeyedRouter) Handlers() map[string]Handler {
	return r.handlers
}
