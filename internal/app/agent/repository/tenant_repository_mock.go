// Code generated by MockGen. DO NOT EDIT.
// Source: ../repository/tenant_repository.go
//
// Generated by this command:
//
//	mockgen -source=../repository/tenant_repository.go -destination=../repository/tenant_repository_mock.go -package=repository
//

// Package repository is a generated GoMock package.
package repository

import (
	context "context"
	reflect "reflect"

	schema "github.com/GoPlugin/web3rpcproxy/internal/app/database/schema"
	gomock "go.uber.org/mock/gomock"
)

// MockITenantRepository is a mock of ITenantRepository interface.
type MockITenantRepository struct {
	ctrl     *gomock.Controller
	recorder *MockITenantRepositoryMockRecorder
}

// MockITenantRepositoryMockRecorder is the mock recorder for MockITenantRepository.
type MockITenantRepositoryMockRecorder struct {
	mock *MockITenantRepository
}

// NewMockITenantRepository creates a new mock instance.
func NewMockITenantRepository(ctrl *gomock.Controller) *MockITenantRepository {
	mock := &MockITenantRepository{ctrl: ctrl}
	mock.recorder = &MockITenantRepositoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockITenantRepository) EXPECT() *MockITenantRepositoryMockRecorder {
	return m.recorder
}

// GetTenantByToken mocks base method.
func (m *MockITenantRepository) GetTenantByToken(ctx context.Context, token string, info *schema.Tenant) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTenantByToken", ctx, token, info)
	ret0, _ := ret[0].(error)
	return ret0
}

// GetTenantByToken indicates an expected call of GetTenantByToken.
func (mr *MockITenantRepositoryMockRecorder) GetTenantByToken(ctx, token, info any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTenantByToken", reflect.TypeOf((*MockITenantRepository)(nil).GetTenantByToken), ctx, token, info)
}
