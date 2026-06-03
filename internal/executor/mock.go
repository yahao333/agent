package executor

import (
	"context"
	"fmt"
)

// MockExecutor is a programmable executor for testing.
type MockExecutor struct {
	// Responses is a list of responses to return in order.
	// Each call to Run consumes one response.
	Responses []*Response

	// Errors holds errors corresponding to each response position (nil = no error).
	Errors []error

	// Calls records every invocation of Run.
	Calls []Request

	// AfterEach, if set, is called after each Run with the request and response.
	// Useful for updating scratchpad between iterations in tests.
	AfterEach func(req Request, resp *Response, err error)

	callIdx int
}

// NewMock creates a ready-to-use MockExecutor with no responses.
func NewMock() *MockExecutor {
	return &MockExecutor{}
}

// AppendResponse adds a response to the end of the list.
func (m *MockExecutor) AppendResponse(resp *Response) {
	m.Responses = append(m.Responses, resp)
	m.Errors = append(m.Errors, nil)
}

// AppendError adds an error response (nil Response).
func (m *MockExecutor) AppendError(err error) {
	m.Responses = append(m.Responses, nil)
	m.Errors = append(m.Errors, err)
}

// Run implements the Executor interface.
func (m *MockExecutor) Run(ctx context.Context, req Request, sink EventSink, iterDir string) (*Response, error) {
	m.Calls = append(m.Calls, req)

	if m.callIdx >= len(m.Responses) {
		return nil, fmt.Errorf("MockExecutor: no more responses configured (call #%d)", m.callIdx+1)
	}

	resp := m.Responses[m.callIdx]
	err := m.Errors[m.callIdx]
	m.callIdx++

	if m.AfterEach != nil {
		m.AfterEach(req, resp, err)
	}

	if err != nil {
		return resp, err
	}
	return resp, nil
}

// Reset clears call history and response index.
func (m *MockExecutor) Reset() {
	m.Calls = nil
	m.callIdx = 0
}