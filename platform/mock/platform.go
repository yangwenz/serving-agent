// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/HyperGAI/serving-agent/platform (interfaces: Platform)

// Package mockplatform is a generated GoMock package.
package mockplatform

import (
	reflect "reflect"

	platform "github.com/HyperGAI/serving-agent/platform"
	gomock "go.uber.org/mock/gomock"
)

// MockPlatform is a mock of Platform interface.
type MockPlatform struct {
	ctrl     *gomock.Controller
	recorder *MockPlatformMockRecorder
}

// MockPlatformMockRecorder is the mock recorder for MockPlatform.
type MockPlatformMockRecorder struct {
	mock *MockPlatform
}

// NewMockPlatform creates a new mock instance.
func NewMockPlatform(ctrl *gomock.Controller) *MockPlatform {
	mock := &MockPlatform{ctrl: ctrl}
	mock.recorder = &MockPlatformMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPlatform) EXPECT() *MockPlatformMockRecorder {
	return m.recorder
}

// Docs mocks base method.
func (m *MockPlatform) Docs(arg0 *platform.DocsRequest) (interface{}, *platform.RequestError) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Docs", arg0)
	ret0, _ := ret[0].(interface{})
	ret1, _ := ret[1].(*platform.RequestError)
	return ret0, ret1
}

// Docs indicates an expected call of Docs.
func (mr *MockPlatformMockRecorder) Docs(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Docs", reflect.TypeOf((*MockPlatform)(nil).Docs), arg0)
}

// Predict mocks base method.
func (m *MockPlatform) Predict(arg0 *platform.InferRequest, arg1 string) (*platform.InferResponse, *platform.RequestError) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Predict", arg0, arg1)
	ret0, _ := ret[0].(*platform.InferResponse)
	ret1, _ := ret[1].(*platform.RequestError)
	return ret0, ret1
}

// Predict indicates an expected call of Predict.
func (mr *MockPlatformMockRecorder) Predict(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Predict", reflect.TypeOf((*MockPlatform)(nil).Predict), arg0, arg1)
}
