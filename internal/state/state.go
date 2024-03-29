package state

import (
	"context"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/muhamadazmy/restate-sdk-go"
	"github.com/muhamadazmy/restate-sdk-go/generated/proto/dynrpc"
	"github.com/muhamadazmy/restate-sdk-go/generated/proto/protocol"
	"github.com/muhamadazmy/restate-sdk-go/internal/wire"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"
)

const (
	Version = 1
)

var (
	ErrInvalidVersion = fmt.Errorf("invalid version number")
)

var (
	_ restate.Context = (*Context)(nil)
)

// suspend is a struct we use to throw in a panic so we can rewind the stack
// then specially handle to suspend the invocation
type suspend struct {
	resumeEntry uint32
}

type Context struct {
	ctx     context.Context
	machine *Machine
}

func (c *Context) Ctx() context.Context {
	return c.ctx
}

func (c *Context) Set(key string, value []byte) error {
	return c.machine.set(key, value)
}

func (c *Context) Clear(key string) error {
	return c.machine.clear(key)

}

// ClearAll drops all associated keys
func (c *Context) ClearAll() error {
	return c.machine.clearAll()

}

func (c *Context) Get(key string) ([]byte, error) {
	return c.machine.get(key)
}

func (c *Context) Keys() ([]string, error) {
	return c.machine.keys()
}

func (c *Context) Sleep(until time.Time) error {
	return c.machine.sleep(until)
}

func (c *Context) Service(service string) restate.Service {
	return &serviceProxy{
		Context: c,
		service: service,
	}
}

func (c *Context) SideEffect(fn func() ([]byte, error), bo ...backoff.BackOff) ([]byte, error) {
	var back backoff.BackOff
	if len(bo) == 0 {
		back = &restate.DefaultBackoffPolicy
	} else if len(bo) == 1 {
		back = bo[0]
	} else {
		panic("only single backoff policy is allowed")
	}

	return c.machine.sideEffect(fn, back)
}

func newContext(inner context.Context, machine *Machine) *Context {

	// state := make(map[string][]byte)
	// for _, entry := range start.Payload.StateMap {
	// 	state[string(entry.Key)] = entry.Value
	// }

	ctx := &Context{
		ctx:     inner,
		machine: machine,
	}

	return ctx
}

type Machine struct {
	handler  restate.Handler
	protocol *wire.Protocol
	mutex    sync.Mutex

	// state
	id string

	partial bool
	current map[string][]byte

	entries    []wire.Message
	entryIndex int

	log zerolog.Logger
}

func NewMachine(handler restate.Handler, conn io.ReadWriter) *Machine {
	return &Machine{
		handler:  handler,
		protocol: wire.NewProtocol(conn),
		current:  make(map[string][]byte),
	}
}

// Start starts the state machine
func (m *Machine) Start(inner context.Context, trace string) error {
	// reader starts a rea
	msg, err := m.protocol.Read()
	if err != nil {
		return err
	}

	if msg.Type() != wire.StartMessageType {
		// invalid negotiation
		return wire.ErrUnexpectedMessage
	}

	start := msg.(*wire.StartMessage)

	m.id = start.Payload.DebugId
	if start.Version != Version {
		return ErrInvalidVersion
	}

	m.log = log.With().Str("id", start.Payload.DebugId).Str("method", trace).Logger()

	ctx := newContext(inner, m)

	m.log.Debug().Msg("start invocation")
	defer m.log.Debug().Msg("invocation ended")

	return m.process(ctx, start)
}

// handle handler response and build proper response message
func (m *Machine) output(r *dynrpc.RpcResponse, err error) proto.Message {
	if err != nil {
		m.log.Error().Err(err).Msg("failure")
	}

	if err != nil && restate.IsTerminalError(err) {
		// terminal errors.
		return &protocol.OutputStreamEntryMessage{
			Result: &protocol.OutputStreamEntryMessage_Failure{
				Failure: &protocol.Failure{
					Code:    uint32(restate.ErrorCode(err)),
					Message: err.Error(),
				},
			},
		}
	} else if err != nil {
		// non terminal error!
		return &protocol.ErrorMessage{
			Code:    uint32(restate.ErrorCode(err)),
			Message: err.Error(),
		}
	}

	bytes, err := proto.Marshal(r)
	if err != nil {
		// this shouldn't happen but in case we return a retry error
		return &protocol.ErrorMessage{
			Code:        uint32(restate.INTERNAL),
			Message:     err.Error(),
			Description: "failed to serialize call output",
		}
	}

	return &protocol.OutputStreamEntryMessage{
		Result: &protocol.OutputStreamEntryMessage_Value{
			Value: bytes,
		},
	}
}

func (m *Machine) invoke(ctx *Context, input *dynrpc.RpcRequest) error {
	// always terminate the invocation with
	// an end message.
	// this will always terminate the connection

	defer func() {
		// recover will return a non-nil object
		// if there was a panic
		//
		recovered := recover()

		switch typ := recovered.(type) {
		case nil:
			// nothing to do, just send end message and exit
			break
		case *suspend:
			// suspend object with thrown. we need to send a suspension
			// message. then terminate the connection
			m.log.Debug().Msg("suspending invocation")
			err := m.protocol.Write(&protocol.SuspensionMessage{
				EntryIndexes: []uint32{typ.resumeEntry},
			})

			if err != nil {
				m.log.Error().Err(err).Msg("error sending failure message")
			}
			return
		default:
			// unknown panic!
			// send an error message (retryable)
			err := m.protocol.Write(&protocol.ErrorMessage{
				Code:        uint32(restate.INTERNAL),
				Message:     fmt.Sprint(typ),
				Description: string(debug.Stack()),
			})

			if err != nil {
				m.log.Error().Err(err).Msg("error sending failure message")
			}
		}

		if err := m.protocol.Write(&protocol.EndMessage{}); err != nil {
			m.log.Error().Err(err).Msg("error sending end message")
		}
	}()

	output := m.output(m.handler.Call(ctx, input))

	return m.protocol.Write(output)
}

func (m *Machine) process(ctx *Context, start *wire.StartMessage) error {
	for _, entry := range start.Payload.StateMap {
		m.current[string(entry.Key)] = entry.Value
	}

	// expect input message
	msg, err := m.protocol.Read()
	if err != nil {
		return err
	}

	if msg.Type() != wire.PollInputEntryMessageType {
		return wire.ErrUnexpectedMessage
	}

	m.log.Trace().Uint32("known entries", start.Payload.KnownEntries).Msg("known entires")
	m.entries = make([]wire.Message, 0, start.Payload.KnownEntries-1)

	// we don't track the poll input entry
	for i := uint32(1); i < start.Payload.KnownEntries; i++ {
		msg, err := m.protocol.Read()
		if err != nil {
			return fmt.Errorf("failed to read entry: %w", err)
		}

		m.log.Trace().Uint16("type", uint16(msg.Type())).Msg("replay log entry")
		m.entries = append(m.entries, msg)
	}

	inputMsg := msg.(*wire.PollInputEntry)
	value := inputMsg.Payload.GetValue()
	var input dynrpc.RpcRequest
	if err := proto.Unmarshal(value, &input); err != nil {
		return fmt.Errorf("invalid invocation input: %w", err)
	}

	return m.invoke(ctx, &input)

}

func (c *Machine) currentEntry() (wire.Message, bool) {
	if c.entryIndex < len(c.entries) {
		return c.entries[c.entryIndex], true
	}

	return nil, false
}

// replayOrNew is a utility function to easily either
// replay a log entry, or create a new one if one
// does not exist
//
// this should be an instance method on Machine but unfortunately
// go does not support generics on instance methods
//
// the idea is when called, it will check if there is a log
// entry at current index, then compare the entry message type
// if not matching, that's obviously an error with the code version
// (code has changed and now doesn't match the play log)
//
// if type is okay, the function will then call a `replay“ callback.
// the replay callback just need to extract the result from the entry
//
// otherwise this function will call a `new` callback to create a new entry in the log
// by sending the proper runtime messages
func replayOrNew[M wire.Message, O any](
	m *Machine,
	typ wire.Type,
	replay func(msg M) (O, error),
	new func() (O, error),
) (output O, err error) {

	m.mutex.Lock()
	defer m.mutex.Unlock()

	defer func() {
		m.entryIndex += 1
	}()

	// check if there is an entry as this index
	entry, ok := m.currentEntry()

	// if entry exists, we need to replay it
	// by calling the replay function
	if ok {
		if entry.Type() != typ {
			return output, errEntryMismatch
		}
		return replay(entry.(M))
	}

	// other wise call the new function
	return new()
}
