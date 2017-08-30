package gcache

import (
	"github.com/stretchr/testify/mock"
	"io"
	"testing"
)

func TestLazyReader(t *testing.T) {
	mock := new(MockSomething)
	mock.On("Get", "hello", int64(0), int64(0)).Return(nil, nil)
	mock.Get("hello", int64(0), int64(0))
	mock.AssertExpectations(t)
}

type MockSomething struct {
	mock.Mock
}

func (m *MockSomething) Get(url string, offset, length int64) (io.ReadCloser, error) {
	args := m.Called(url, offset, length)
	var ret0 io.ReadCloser = nil
	var ret1 error = nil
	if args.Get(0) != nil {
		ret0 = args.Get(0).(io.ReadCloser)
	}
	if args.Get(1) != nil {
		ret1 = args.Get(1).(error)
	}
	return ret0, ret1
}
