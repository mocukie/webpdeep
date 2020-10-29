package coder

import (
    "bufio"
    "github.com/mocukie/webp-go/webp"
    "github.com/mocukie/webpdeep/pkg/imagex"
    "github.com/pkg/errors"
    "io"
    "strings"
)

var webpDecodeOpts = webp.NewDecOptions()

func init() {
    webpDecodeOpts.ImageType = webp.TypeNRGBA
}

type WebP struct {
    Opts       *webp.EncodeOptions
    CopyMeta   bool
    CheckImage bool
}

func (wp *WebP) Convert(in io.Reader, out io.Writer) (err error, warnings []error) {
    opts := wp.Opts

    img, _, meta, e := imagex.Decode(bufio.NewReader(in))
    if e != nil {
        err = errors.Wrap(e, "[WebP] decode image failed")
        return
    }

    var webpData []byte
    if webpData, err = webp.EncodeSlice(img, opts); err != nil {
        err = errors.Wrap(err, "[WebP] encode failed")
        return
    }

    if opts.Lossless && wp.CheckImage {
        webpImg, e := webp.DecodeSlice(webpData, webpDecodeOpts)
        if e != nil {
            warnings = append(warnings, errors.Wrap(e, "[WebP] decode failed when compare lossless image"))
        } else if !imagex.IsImageEqual(img, webpImg) {
            warnings = append(warnings, errors.New("[WebP] lossless options on, but image not equal"))
        }
    }

    if wp.CopyMeta && !meta.Empty() {
        for _, cc := range [...]webp.FourCC{webp.ICCP, webp.EXIF, webp.XMP} {
            if chunk := meta.Get(strings.TrimRight(string(cc[:4:4]), " ")); chunk != nil {
                var tmp []byte
                tmp, e = webp.SetMetadata(webpData, cc, chunk)
                if e != nil {
                    warnings = append(warnings, errors.Wrapf(e, "[WebP] set %s failed", cc))
                } else {
                    webpData = tmp
                }
            }
        }
    }

    if _, err = out.Write(webpData); err != nil {
        err = errors.Wrap(err, "[WebP] write to output failed")
    }

    return
}
