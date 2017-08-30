package gcache

import (
	"encoding/json"
	"errors"
	"github.com/golang/groupcache"
	"io"
)

type lazyReaderAt struct {
	request   dataRequest
	size      int64
	groupName string
	ctx       cacheContext
}

func (reader lazyReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	//key := reader.request.key + "-" + strconv.Itoa(int(reader.request.block))

	jsonDataRequest, err := json.Marshal(reader.request)
	if err != nil {
		return 0, err
	}
	key := "data/" + string(jsonDataRequest)
	var byteView groupcache.ByteView
	err = groupcache.GetGroup(reader.groupName).Get(reader.ctx, key, groupcache.ByteViewSink(&byteView))
	if err != nil {
		return 0, err
	}
	n := byteView.SliceFrom(int(offset)).Copy(p)
	//n := copy(p, byteView.ByteSlice()[offset:])
	return int(n), err
}

func (reader lazyReaderAt) Size() int64 {
	return reader.size
}

func NewLazyReader(reader io.ReaderAt, start, end, blockSize int64) io.ReadSeeker {
	return &lazyReadSeeker{
		base:      reader,
		start:     start,
		end:       end,
		pos:       start,
		blockSize: blockSize,
	}
}

type WriterReadSeeker interface {
	io.ReadSeeker
	io.WriterTo
}

type lazyReadSeeker struct {
	base      io.ReaderAt
	start     int64
	end       int64
	pos       int64
	blockSize int64
}

func (reader *lazyReadSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == 0 {
		reader.pos = reader.start + offset
	} else if whence == 1 {
		reader.pos = reader.pos + offset
	} else if whence == 2 {
		reader.pos = reader.end - offset
	} else {
		return reader.pos, errors.New("Unknown whence value")
	}
	return reader.pos - reader.start, nil
}

func (reader *lazyReadSeeker) Read(p []byte) (int, error) {
	if reader.pos == reader.end {
		return 0, io.EOF
	}
	n, err := reader.base.ReadAt(p, reader.pos)
	reader.pos = reader.pos + int64(n)
	return n, err
}

func (reader *lazyReadSeeker) Size() int64 {
	return reader.end - reader.start
}

func (reader *lazyReadSeeker) WriteTo(w io.Writer) (int64, error) {
	var count int64 = 0
	for reader.pos < reader.end {
		var buf []byte
		curBlockSize := reader.blockSize
		if reader.end-reader.pos < reader.blockSize {
			curBlockSize = reader.end - reader.pos
		}
		buf = make([]byte, curBlockSize, curBlockSize)
		readN, err := reader.base.ReadAt(buf, reader.pos)
		if err != nil {
			return count, err
		}
		writeN, err := w.Write(buf[:readN])
		reader.pos += int64(writeN)
		count += int64(writeN)
		if err != nil {
			return count, err
		}
	}
	return int64(count), io.EOF
}
