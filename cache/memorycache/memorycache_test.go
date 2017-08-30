package gcache

import (
	"bytes"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	"io/ioutil"
	"testing"
)

type testHydrator struct {
	mock.Mock
}

type testDiskCache struct {
	mock.Mock
}

func TestDiskCacheAccess(t *testing.T) {
	hydrator := new(testHydrator)
	diskCache := new(testDiskCache)

	diskCache.On("Get", "foo").Return(ioutil.NopCloser(bytes.NewBuffer(make([]byte, 2048, 2048))), nil)
	//hydrator.On("Get", "foo").Return(make([]byte, 2048, 2048), nil)

	config := Config{
		BlockSize:      int64(1 * 1024 * 1024),
		MaxMemoryUsage: 64 * 1024 * 1024,
		Hydrator:       hydrator,
		DiskCache:      diskCache,
		GroupName:      "testdiskcache",
	}

	cache := NewCache(config)
	reader, err := cache.Get("foo", make(map[string]string))
	if err != nil {
		t.Fail()
	}
	data := make([]byte, 10, 10)
	length, err := reader.ReadAt(data, 0)
	assert.Equal(t, 10, length)
	assert.Equal(t, nil, err)
	diskCache.AssertExpectations(t)
	hydrator.AssertExpectations(t)
}

func TestHydratorAccess(t *testing.T) {
	hydrator := new(testHydrator)
	diskCache := new(testDiskCache)

	diskCache.On("Get", "foo").Return(nil, errors.New("Not Found"))
	hydrator.On("Get", "foo", int64(0), int64(1048576)).Return(make([]byte, 10, 10), nil)
	diskCache.On("Put", "foo", mock.Anything).Return(nil)

	config := Config{
		BlockSize:      int64(1 * 1024 * 1024),
		MaxMemoryUsage: 64 * 1024 * 1024,
		Hydrator:       hydrator,
		DiskCache:      diskCache,
		GroupName:      "testhydrator",
	}
	cache := NewCache(config)
	reader, err := cache.Get("foo", make(map[string]string))
	if err != nil {
		t.Fail()
	}
	data := make([]byte, 10, 10)
	length, err := reader.ReadAt(data, 0)
	assert.Equal(t, 10, length)
	assert.Equal(t, nil, err)
	diskCache.AssertExpectations(t)
	hydrator.AssertExpectations(t)
}

func (m *testHydrator) Get(url string, offset int64, length int64) ([]byte, error) {
	args := m.Called(url, offset, length)
	var ret0 []byte = nil
	if args.Get(0) != nil {
		ret0 = args.Get(0).([]byte)
	}
	var ret1 error = nil
	if args.Get(1) != nil {
		ret1 = args.Get(1).(error)
	}
	return ret0, ret1
}

func (m *testHydrator) GetMetadata(url string) (map[string]string, error) {
	args := m.Called(url)
	return args.Get(0).(map[string]string), args.Get(1).(error)
}

func (m *testDiskCache) Get(url string) (io.ReadCloser, error) {
	args := m.Called(url)
	var ret0 io.ReadCloser
	if args.Get(0) != nil {
		ret0 = args.Get(0).(io.ReadCloser)
	}
	var ret1 error = nil
	if args.Get(1) != nil {
		ret1 = args.Get(1).(error)
	}
	return ret0, ret1
}

func (m *testDiskCache) GetRange(url string, one, two int64) (io.ReadCloser, error) {
	args := m.Called(url, one, two)
	return args.Get(0).(io.ReadCloser), args.Get(1).(error)
}
func (m *testDiskCache) Hit(url string) error {
	args := m.Called(url)
	return args.Get(0).(error)
}
func (m *testDiskCache) Put(url string, data io.Reader) error {
	args := m.Called(url, data)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(error)
}

func (m *testDiskCache) Shutdown() error {
	args := m.Called()
	return args.Get(0).(error)
}
