package restate

import (
	"errors"
	"fmt"
)

type Code uint16

const (
	/**
	* The operation was cancelled, typically by the caller.
	* HTTP 408
	 */
	CANCELLED Code = 1
	/**
	* Unknown error. For example, this error may be returned when a
	* Status value received from another address space belongs to an error
	* space that is not known in this address space. Also errors raised by APIs
	* that do  not return enough error information may be converted to this
	* error.
	* HTTP 500
	 */
	UNKNOWN Code = 2
	/**
	* The client specified an invalid argument. Note that
	* this differs from FAILED_PRECONDITION. INVALID_ARGUMENT indicates
	* arguments that are problematic regardless of the state of the system
	* (e.g., a malformed file name).
	* HTTP 400
	 */
	INVALID_ARGUMENT Code = 3
	/**
	* The deadline expired before the operation could
	* complete. For operations that change the state of the system, this error
	* may be returned even if the operation has completed successfully. For
	* example, a successful response from a server could have been delayed
	* long.
	* HTTP 408
	 */
	DEADLINE_EXCEEDED Code = 4
	/**
	* Some requested entity (e.g., file or directory) was not
	* found. Note to server developers: if a request is denied for an entire
	* class of users, such as gradual feature rollout or undocumented
	* allowlist, NOT_FOUND may be used. If a request is denied for some users
	* within a class of users, such as user-based access control,
	* PERMISSION_DENIED must be used.
	* HTTP 404
	 */
	NOT_FOUND Code = 5
	/**
	* The entity that a client attempted to create (e.g., file
	* or directory) already exists.
	* HTTP 409
	 */
	ALREADY_EXISTS Code = 6
	/**
	* The caller does not have permission to execute the
	* specified operation. PERMISSION_DENIED must not be used for rejections
	* caused by exhausting some resource (use RESOURCE_EXHAUSTED instead for
	* those errors). PERMISSION_DENIED must not be used if the caller can not
	* be identified (use UNAUTHENTICATED instead for those errors). This error
	* code does not imply the request is valid or the requested entity exists
	* or satisfies other pre-conditions.
	* HTTP 403
	 */
	PERMISSION_DENIED Code = 7
	/**
	* Some resource has been exhausted, perhaps a per-user
	* quota, or perhaps the entire file system is out of space.
	* HTTP 413
	 */
	RESOURCE_EXHAUSTED Code = 8
	/**
	* The operation was rejected because the system is
	* not in a state required for the operation's execution. For example, the
	* directory to be deleted is non-empty, an rmdir operation is applied to a
	* non-directory, etc. Service implementors can use the following guidelines
	* to decide between FAILED_PRECONDITION, ABORTED, and UNAVAILABLE: (a) Use
	* UNAVAILABLE if the client can retry just the failing call. (b) Use
	* ABORTED if the client should retry at a higher level (e.g., when a
	* client-specified test-and-set fails, indicating the client should restart
	* a read-modify-write sequence). (c) Use FAILED_PRECONDITION if the client
	* should not retry until the system state has been explicitly fixed. E.g.,
	* if an "rmdir" fails because the directory is non-empty,
	* FAILED_PRECONDITION should be returned since the client should not retry
	* unless the files are deleted from the directory.
	* HTTP 412
	 */
	FAILED_PRECONDITION Code = 9
	/**
	* The operation was aborted, typically due to a concurrency issue
	* such as a sequencer check failure or transaction abort. See the
	* guidelines above for deciding between FAILED_PRECONDITION, ABORTED, and
	* UNAVAILABLE.
	* HTTP 409
	 */
	ABORTED Code = 10
	/**
	* The operation was attempted past the valid range. E.g.,
	* seeking or reading past end-of-file. Unlike INVALID_ARGUMENT, this error
	* indicates a problem that may be fixed if the system state changes. For
	* example, a 32-bit file system will generate INVALID_ARGUMENT if asked to
	* read at an offset that is not in the range [0,2^32-1], but it will
	* generate OUT_OF_RANGE if asked to read from an offset past the current
	* file size. There is a fair bit of overlap between FAILED_PRECONDITION and
	* OUT_OF_RANGE. We recommend using OUT_OF_RANGE (the more specific error)
	* when it applies so that callers who are iterating through a space can
	* easily look for an OUT_OF_RANGE error to detect when they are done.
	* HTTP 400
	 */
	OUT_OF_RANGE Code = 11
	/**
	* The operation is not implemented or is not
	* supported/enabled in this service.
	* HTTP 501
	 */
	UNIMPLEMENTED Code = 12
	/**
	* Internal errors. This means that some invariants expected by
	* the underlying system have been broken. This error code is reserved for
	* serious errors.
	* HTTP 500
	 */
	INTERNAL Code = 13
	/**
	* The service is currently unavailable. This is most likely a
	* transient condition, which can be corrected by retrying with a backoff.
	* Note that it is not always safe to retry non-idempotent operations.
	* HTTP 503
	 */
	UNAVAILABLE Code = 14
	/**
	* Unrecoverable data loss or corruption.
	* HTTP 500
	 */
	DATA_LOSS Code = 15
	/**
	* The request does not have valid authentication
	* credentials for the operation.
	* HTTP 401
	 */
	UNAUTHENTICATED Code = 16
)

type codeError struct {
	code  Code
	inner error
}

func (e *codeError) Error() string {
	return fmt.Sprintf("[CODE %04X] %s", e.code, e.inner)
}

func (e *codeError) Unwrap() error {
	return e.inner
}

type terminalError struct {
	inner error
}

func (e *terminalError) Error() string {
	return e.inner.Error()
}

func (e *terminalError) Unwrap() error {
	return e.inner
}

// WithErrorCode returns an error with specific
func WithErrorCode(err error, code Code) error {
	if err == nil {
		return nil
	}

	return &codeError{
		inner: err,
		code:  code,
	}
}

// TerminalError returns a terminal error with optional code.
// code is optional but only one code is allowed.
// By default restate will retry the invocation forever unless a terminal error
// is returned
func TerminalError(err error, code ...Code) error {
	if err == nil {
		return nil
	}

	if len(code) > 1 {
		panic("only single code is allowed")
	}

	err = &terminalError{
		inner: err,
	}

	if len(code) == 1 {
		err = &codeError{
			inner: err,
			code:  code[0],
		}
	}

	return err
}

// IsTerminalError checks if err is terminal
func IsTerminalError(err error) bool {
	if err == nil {
		return false
	}
	var t *terminalError
	return errors.As(err, &t)
}

// ErrorCode returns code associated with error or UNKNOWN
func ErrorCode(err error) Code {
	var e *codeError
	if errors.As(err, &e) {
		return e.code
	}

	return UNKNOWN
}
