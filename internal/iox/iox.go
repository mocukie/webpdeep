package iox

import (
    "io"
    "os"
)

const NestSeparator = "|"

type Input interface {
    io.Reader
    Path() string
    Info() (os.FileInfo, error)
    Open() error
    Close() error
}

type Output interface {
    io.Writer
    Path() string
    Open(info os.FileInfo) error
    Close() error
}
