package imagex

import (
    "bufio"
    "image"
    "io"
    "sync"
    "sync/atomic"
)

type MetaChunks interface {
    Get(tag string) []byte
    Empty() bool
}

type MetaChunksReader interface {
    io.Reader
    MetaChunks
}

type EmptyMetaChunks struct{}

func (r EmptyMetaChunks) Get(tag string) []byte {
    return nil
}

func (r EmptyMetaChunks) Empty() bool {
    return true
}

var (
    formats     atomic.Value
    formatsLock sync.Mutex
)

type format struct {
    name, magic string
    new         func(io.Reader) (MetaChunksReader, error)
    decode      func(io.Reader) (image.Image, error)
}

func RegisterFormat(name, magic string, new func(io.Reader) (MetaChunksReader, error), decode func(io.Reader) (image.Image, error)) {
    formatsLock.Lock()
    tmp, _ := formats.Load().([]format)
    formats.Store(append(tmp, format{name, magic, new, decode}))
    formatsLock.Unlock()
}

type reader interface {
    io.Reader
    Peek(int) ([]byte, error)
}

func asReader(r io.Reader) reader {
    if rr, ok := r.(reader); ok {
        return rr
    }
    return bufio.NewReader(r)
}

func match(header []byte, magic string) bool {
    if len(header) != len(magic) {
        return false
    }
    for i, b := range header {
        if magic[i] != b && magic[i] != '?' {
            return false
        }
    }
    return true
}

func Decode(r io.Reader) (image.Image, string, MetaChunks, error) {
    rr := asReader(r)
    tmp, _ := formats.Load().([]format)
    for _, f := range tmp {
        header, err := rr.Peek(len(f.magic))
        if err == nil && match(header, f.magic) {
            meta, _ := f.new(rr)
            img, e := f.decode(meta)
            return img, f.name, meta, e
        }
    }
    img, name, err := image.Decode(rr)
    return img, name, EmptyMetaChunks{}, err
}
