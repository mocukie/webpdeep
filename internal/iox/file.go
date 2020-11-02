package iox

import (
	"github.com/pkg/errors"
	"os"
	"time"
)

type FileInput struct {
	*os.File
	path string
	info os.FileInfo
}

func NewFileInput(path string, info os.FileInfo) *FileInput {
	return &FileInput{
		path: path,
		info: info,
	}
}

func (fi *FileInput) Path() string {
	return fi.path
}

func (fi *FileInput) Open() error {
	var err error
	fi.File, err = os.Open(fi.path)
	return err
}

func (fi *FileInput) Info() (os.FileInfo, error) {
	var err error
	if fi.info == nil {
		fi.info, err = os.Stat(fi.path)
	}
	return fi.info, err
}

type FileOutput struct {
	*os.File
	path string
	info os.FileInfo
}

func NewFileOutput(path string) *FileOutput {
	return &FileOutput{path: path}
}

func (fo *FileOutput) Path() string {
	return fo.path
}

func (fo *FileOutput) Open(info os.FileInfo) error {
	var err error
	fo.File, err = os.Create(fo.path)
	fo.info = info
	return err
}

func (fo *FileOutput) Close() error {
	var err error
	if fo.File != nil {
		err = fo.File.Close()
		if err != nil {
			return errors.WithStack(err)
		}
		fo.File = nil
	}

	if fo.info == nil {
		return nil
	}

	i, p := fo.info, fo.path
	if err = os.Chmod(p, i.Mode()); err != nil {
		return errors.WithStack(err)
	}
	if err = os.Chtimes(p, time.Now(), i.ModTime()); err != nil {
		return errors.WithStack(err)
	}

	fo.info = nil
	return nil
}
