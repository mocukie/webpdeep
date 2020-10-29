package coder

import (
    "bufio"
    "github.com/pkg/errors"
    "io"
)

type Codec interface {
    Convert(in io.Reader, out io.Writer) (err error, warnings []error)
}

type Copy struct{}

func (*Copy) Convert(in io.Reader, out io.Writer) (error, []error) {
    var r, w = bufio.NewReader(in), bufio.NewWriter(out)

    if _, err := io.Copy(w, r); err != nil {

        return errors.Wrap(err, "[Copy] copy failed"), nil
    }
    if err := w.Flush(); err != nil {
        return errors.Wrap(err, "[Copy] flush output failed"), nil
    }

    return nil, nil
}
