package iox

import (
	"archive/zip"
	"bytes"
	"github.com/pkg/errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ZipInput struct {
	io.ReadCloser
	zip         *zip.ReadCloser
	entry       *zip.File
	path        string
	entrySepIdx int
}

func NewZipInput(path string) (*ZipInput, error) {
	idx := strings.LastIndex(path, NestSeparator)
	if idx == -1 {
		return nil, errors.New("invalid zip path")
	}
	return &ZipInput{
		path:        path,
		entrySepIdx: idx,
	}, nil
}

func (zi *ZipInput) Path() string {
	return zi.path
}

func (zi *ZipInput) Open() error {
	var err error
	zi.zip, err = zip.OpenReader(zi.path[:zi.entrySepIdx])
	if err != nil {
		return errors.WithStack(err)
	}

	name := zi.path[zi.entrySepIdx+1:]
	for _, entry := range zi.zip.File {
		if name == entry.Name {
			zi.entry = entry
			zi.ReadCloser, err = entry.Open()
			break
		}
	}
	if zi.entry == nil {
		err = errors.New("zip entry not found")
	}
	return err
}

func (zi *ZipInput) Info() (os.FileInfo, error) {
	if zi.entry == nil {
		return nil, errors.New("zip not open yet")
	}
	return zi.entry.FileInfo(), nil
}

func (zi *ZipInput) Close() error {
	if zi.zip != nil {
		if err := zi.zip.Close(); err != nil {
			return err
		}
	}
	zi.zip = nil
	zi.entry = nil
	return nil
}

type SafeZipWriter struct {
	f io.Closer
	*zip.Writer
	*sync.Mutex
	ref int32
}

func NewZipWriter(f io.WriteCloser, ref int32) *SafeZipWriter {
	return &SafeZipWriter{
		f:      f,
		Writer: zip.NewWriter(f),
		Mutex:  new(sync.Mutex),
		ref:    ref,
	}
}

func (zw *SafeZipWriter) UnRef() error {
	if atomic.AddInt32(&zw.ref, -1) != 0 {
		return nil
	}
	if err := zw.Writer.Close(); err != nil {
		return errors.WithStack(err)
	}
	if err := zw.f.Close(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type ZipOutput struct {
	io.Writer
	zip         *SafeZipWriter
	path        string
	entrySepIdx int
	fh          *zip.FileHeader
}

func NewZipOutput(path string) (*ZipOutput, error) {
	idx := strings.LastIndex(path, NestSeparator)
	if idx == -1 {
		return nil, errors.New("invalid zip path")
	}
	return &ZipOutput{
		path:        path,
		entrySepIdx: idx,
	}, nil
}

func (zo *ZipOutput) SetZipWriter(zw *SafeZipWriter) {
	zo.zip = zw
}

func (zo *ZipOutput) Path() string {
	return zo.path
}

func (zo *ZipOutput) Open(info os.FileInfo) error {
	zo.Writer = bytes.NewBuffer(nil)

	var fh *zip.FileHeader

	if info != nil {
		fh, _ = zip.FileInfoHeader(info)
	} else {
		fh = new(zip.FileHeader)
		fh.Modified = time.Now()
	}
	fh.Name = zo.path[zo.entrySepIdx+1:]
	zo.fh = fh
	return nil
}

func (zo *ZipOutput) Close() error {
	var err error
	var z = zo.zip
	defer z.Unlock()
	z.Lock()
	if zo.fh != nil {
		var w io.Writer
		w, err = z.CreateHeader(zo.fh)
		if err == nil {
			_, err = io.Copy(w, zo.Writer.(*bytes.Buffer))
		}
	}

	if err := zo.zip.UnRef(); err != nil {
		return errors.WithStack(err)
	}

	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
