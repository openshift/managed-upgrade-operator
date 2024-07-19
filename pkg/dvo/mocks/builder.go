// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/openshift/managed-upgrade-operator/pkg/dvo (interfaces: DvoClientBuilder)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
	dvo "github.com/openshift/managed-upgrade-operator/pkg/dvo"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

// MockDvoClientBuilder is a mock of DvoClientBuilder interface.
type MockDvoClientBuilder struct {
	ctrl     *gomock.Controller
	recorder *MockDvoClientBuilderMockRecorder
}

// MockDvoClientBuilderMockRecorder is the mock recorder for MockDvoClientBuilder.
type MockDvoClientBuilderMockRecorder struct {
	mock *MockDvoClientBuilder
}

// NewMockDvoClientBuilder creates a new mock instance.
func NewMockDvoClientBuilder(ctrl *gomock.Controller) *MockDvoClientBuilder {
	mock := &MockDvoClientBuilder{ctrl: ctrl}
	mock.recorder = &MockDvoClientBuilderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDvoClientBuilder) EXPECT() *MockDvoClientBuilderMockRecorder {
	return m.recorder
}

// New mocks base method.
func (m *MockDvoClientBuilder) New(arg0 client.Client) (dvo.DvoClient, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "New", arg0)
	ret0, _ := ret[0].(dvo.DvoClient)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// New indicates an expected call of New.
func (mr *MockDvoClientBuilderMockRecorder) New(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "New", reflect.TypeOf((*MockDvoClientBuilder)(nil).New), arg0)
}
