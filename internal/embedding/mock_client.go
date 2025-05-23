// Code generated by mockery v2.50.0. DO NOT EDIT.

package embedding

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// MockClient is an autogenerated mock type for the Client type
type MockClient struct {
	mock.Mock
}

type MockClient_Expecter struct {
	mock *mock.Mock
}

func (_m *MockClient) EXPECT() *MockClient_Expecter {
	return &MockClient_Expecter{mock: &_m.Mock}
}

// Embed provides a mock function with given fields: ctx, text
func (_m *MockClient) Embed(ctx context.Context, text string) ([]float32, error) {
	ret := _m.Called(ctx, text)

	if len(ret) == 0 {
		panic("no return value specified for Embed")
	}

	var r0 []float32
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) ([]float32, error)); ok {
		return rf(ctx, text)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) []float32); ok {
		r0 = rf(ctx, text)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]float32)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, text)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClient_Embed_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Embed'
type MockClient_Embed_Call struct {
	*mock.Call
}

// Embed is a helper method to define mock.On call
//   - ctx context.Context
//   - text string
func (_e *MockClient_Expecter) Embed(ctx interface{}, text interface{}) *MockClient_Embed_Call {
	return &MockClient_Embed_Call{Call: _e.mock.On("Embed", ctx, text)}
}

func (_c *MockClient_Embed_Call) Run(run func(ctx context.Context, text string)) *MockClient_Embed_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *MockClient_Embed_Call) Return(_a0 []float32, _a1 error) *MockClient_Embed_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClient_Embed_Call) RunAndReturn(run func(context.Context, string) ([]float32, error)) *MockClient_Embed_Call {
	_c.Call.Return(run)
	return _c
}

// EmbedBatch provides a mock function with given fields: ctx, texts
func (_m *MockClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	ret := _m.Called(ctx, texts)

	if len(ret) == 0 {
		panic("no return value specified for EmbedBatch")
	}

	var r0 [][]float32
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, []string) ([][]float32, error)); ok {
		return rf(ctx, texts)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []string) [][]float32); ok {
		r0 = rf(ctx, texts)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([][]float32)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, []string) error); ok {
		r1 = rf(ctx, texts)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockClient_EmbedBatch_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'EmbedBatch'
type MockClient_EmbedBatch_Call struct {
	*mock.Call
}

// EmbedBatch is a helper method to define mock.On call
//   - ctx context.Context
//   - texts []string
func (_e *MockClient_Expecter) EmbedBatch(ctx interface{}, texts interface{}) *MockClient_EmbedBatch_Call {
	return &MockClient_EmbedBatch_Call{Call: _e.mock.On("EmbedBatch", ctx, texts)}
}

func (_c *MockClient_EmbedBatch_Call) Run(run func(ctx context.Context, texts []string)) *MockClient_EmbedBatch_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([]string))
	})
	return _c
}

func (_c *MockClient_EmbedBatch_Call) Return(_a0 [][]float32, _a1 error) *MockClient_EmbedBatch_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockClient_EmbedBatch_Call) RunAndReturn(run func(context.Context, []string) ([][]float32, error)) *MockClient_EmbedBatch_Call {
	_c.Call.Return(run)
	return _c
}

// Name provides a mock function with no fields
func (_m *MockClient) Name() string {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Name")
	}

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// MockClient_Name_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Name'
type MockClient_Name_Call struct {
	*mock.Call
}

// Name is a helper method to define mock.On call
func (_e *MockClient_Expecter) Name() *MockClient_Name_Call {
	return &MockClient_Name_Call{Call: _e.mock.On("Name")}
}

func (_c *MockClient_Name_Call) Run(run func()) *MockClient_Name_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *MockClient_Name_Call) Return(_a0 string) *MockClient_Name_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *MockClient_Name_Call) RunAndReturn(run func() string) *MockClient_Name_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockClient creates a new instance of MockClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockClient {
	mock := &MockClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
