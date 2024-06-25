// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/openshift/managed-upgrade-operator/pkg/dvo (interfaces: DvoClient)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockDvoClient is a mock of DvoClient interface.
type MockDvoClient struct {
	ctrl     *gomock.Controller
	recorder *MockDvoClientMockRecorder
}

// MockDvoClientMockRecorder is the mock recorder for MockDvoClient.
type MockDvoClientMockRecorder struct {
	mock *MockDvoClient
}

// NewMockDvoClient creates a new mock instance.
func NewMockDvoClient(ctrl *gomock.Controller) *MockDvoClient {
	mock := &MockDvoClient{ctrl: ctrl}
	mock.recorder = &MockDvoClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDvoClient) EXPECT() *MockDvoClientMockRecorder {
	return m.recorder
}

// GetMetrics mocks base method.
func (m *MockDvoClient) GetMetrics() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMetrics")
	ret0, _ := ret[0].(error)
	return ret0
}

// GetMetrics indicates an expected call of GetMetrics.
func (mr *MockDvoClientMockRecorder) GetMetrics() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMetrics", reflect.TypeOf((*MockDvoClient)(nil).GetMetrics))
}
