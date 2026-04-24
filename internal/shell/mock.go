package shell

import (
	"context"
	"fmt"
)

// MockExecutor records calls and returns pre-programmed responses.
// It is exported so other packages' tests can use it directly.
type MockExecutor struct {
	// Responses are consumed in order. When exhausted, the last one repeats.
	Responses []ExecResult
	// Errors are consumed in parallel with Responses.
	Errors []error
	// Calls records every ExecOpts passed to Run, in order.
	Calls []ExecOpts

	idx int
}

// Run records the call and returns the next pre-programmed response.
func (m *MockExecutor) Run(_ context.Context, opts ExecOpts) (ExecResult, error) {
	m.Calls = append(m.Calls, opts)

	i := m.idx
	if i >= len(m.Responses) && len(m.Responses) > 0 {
		i = len(m.Responses) - 1
	}
	if i >= len(m.Errors) && len(m.Errors) > 0 {
		i = len(m.Errors) - 1
	}

	var res ExecResult
	if i < len(m.Responses) {
		res = m.Responses[i]
	}
	var err error
	if i < len(m.Errors) {
		err = m.Errors[i]
	}

	m.idx++
	return res, err
}

// Called returns true if any call matched the given command prefix.
func (m *MockExecutor) Called(cmd string) bool {
	for _, c := range m.Calls {
		if len(c.Cmd) > 0 && c.Cmd[0] == cmd {
			return true
		}
	}
	return false
}

// CallCount returns the number of times Run was called.
func (m *MockExecutor) CallCount() int { return len(m.Calls) }

// Reset clears all recorded calls and resets the response index.
func (m *MockExecutor) Reset() {
	m.Calls = nil
	m.idx = 0
}

// OKResponse is a convenience pre-programmed success result.
func OKResponse(stdout string) ExecResult {
	return ExecResult{Stdout: stdout, ExitCode: 0}
}

// ErrResponse is a convenience pre-programmed failure result.
func ErrResponse(stderr string, code int) (ExecResult, error) {
	return ExecResult{Stderr: stderr, ExitCode: code},
		ErrNonZeroExit{ExitCode: code, Stderr: stderr}
}

// FailAt returns a MockExecutor that succeeds for the first n calls then fails.
func FailAt(n int, failStderr string) *MockExecutor {
	responses := make([]ExecResult, n+1)
	errs := make([]error, n+1)
	for i := range n {
		responses[i] = OKResponse("")
	}
	responses[n] = ExecResult{Stderr: failStderr, ExitCode: 1}
	errs[n] = fmt.Errorf("mock fail at call %d: %s", n, failStderr)
	return &MockExecutor{Responses: responses, Errors: errs}
}
