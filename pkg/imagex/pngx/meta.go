package pngx

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"github.com/mocukie/webpdeep/pkg/imagex"
	"hash"
	"hash/crc32"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"unsafe"
)

const pngMagic = "\x89PNG\r\n\x1a\n"

type iTXtChunk struct {
	key               string
	languageTag       string
	translatedKeyword string
	text              []byte
}

type MetaReader struct {
	in      io.Reader
	reaming int
	tmp     *bytes.Buffer
	crc     hash.Hash32
	tags    map[string][]byte
}

func NewMetaReader(reader io.Reader) (imagex.MetaChunksReader, error) {
	magick := make([]byte, len(pngMagic))
	if _, err := io.ReadFull(reader, magick); err != nil {
		return nil, err
	}

	if string(magick) != pngMagic {
		return nil, png.FormatError("not a PNG file")
	}

	return &MetaReader{
		in:      reader,
		tmp:     bytes.NewBuffer(magick),
		crc:     crc32.NewIEEE(),
		reaming: len(pngMagic),
		tags:    map[string][]byte{},
	}, nil

}

func (mr *MetaReader) Get(tag string) []byte {
	return mr.tags[tag]
}

func (mr *MetaReader) Empty() bool {
	return len(mr.tags) == 0
}

func (mr *MetaReader) Read(p []byte) (int, error) {
	reader := io.MultiReader(mr.tmp, mr.in)
	var err error
	read := 0
	for read < len(p) {
		var r int
		r, err = reader.Read(p[read:])
		read += r
		if err == io.EOF {
			break
		} else if err != nil {
			return read, err
		}
	}

	mr.reaming -= read
	if mr.reaming >= 0 {
		return read, err
	}

	mr.reaming *= -1
	if chkErr := mr.parseChunk(p[read-mr.reaming : read]); chkErr != nil {
		err = chkErr
	}

	return read, err
}

func (mr *MetaReader) parseChunk(read []byte) error {
	mr.tmp.Reset()

	for pos := 0; mr.reaming > 0; {
		var dataLen uint32
		var fourcc []byte
		if mr.reaming < 8 {
			r := io.MultiReader(bytes.NewBuffer(read[pos:]), io.TeeReader(mr.in, mr.tmp))
			buf := make([]byte, 8)
			if _, err := io.ReadFull(r, buf); err != nil {
				return err
			}
			dataLen = binary.BigEndian.Uint32(buf[:4])
			fourcc = buf[4:]
		} else {
			dataLen = binary.BigEndian.Uint32(read[pos : pos+4])
			fourcc = read[pos+4 : pos+8]
		}

		delta := int(dataLen + 8 + 4)
		mr.reaming -= delta

		var tagName string
		var bodyParse func(io.Reader) ([]byte, error)

		if string(fourcc) == "iCCP" && mr.tags["ICCP"] == nil {
			tagName = "ICCP"
			bodyParse = func(body io.Reader) ([]byte, error) {
				_, icc, err := mr.parseZTXt(body.(*bufio.Reader))
				return icc, err
			}
		} else if string(fourcc) == "eXIf" && mr.tags["EXIF"] == nil {
			tagName = "EXIF"
			bodyParse = ioutil.ReadAll
		} else if string(fourcc) == "iTXt" {
			tagName = "XMP"
			bodyParse = func(body io.Reader) ([]byte, error) {
				iTXt, err := mr.parseITXt(body.(*bufio.Reader))
				if err != nil || iTXt.key != "XML:com.adobe.xmp" {
					return nil, err
				}
				return iTXt.text, nil
			}
		} else {
			pos += delta
			continue
		}

		var err error
		chunk, body := mr.prepareChunkReader(pos, dataLen, read)
		//skip chunk fourcc
		if _, err = body.Discard(4); err != nil {
			return err
		}

		tagData, err := bodyParse(body)
		if err != nil {
			return err
		}

		if ok, err := mr.verifyChecksum(chunk); err != nil {
			return err
		} else if !ok {
			return png.FormatError("invalid checksum")
		}

		if len(tagData) != 0 {
			mr.tags[tagName] = tagData
		}

		pos += delta
	}

	mr.reaming *= -1

	return nil
}

func (mr *MetaReader) prepareChunkReader(pos int, dataLen uint32, read []byte) (io.Reader, *bufio.Reader) {
	var chunk io.Reader //fourcc + data + crc
	if mr.reaming < 0 {
		chunk = io.MultiReader(bytes.NewBuffer(read[pos+4:]), io.TeeReader(mr.in, mr.tmp))
	} else {
		chunk = bytes.NewBuffer(read[pos+4 : pos+int(dataLen+8+4)])
	}

	mr.crc.Reset()
	body := bufio.NewReader(io.TeeReader(io.LimitReader(chunk, int64(dataLen+4)), mr.crc)) //fourcc + data

	return chunk, body
}

func (mr *MetaReader) verifyChecksum(r io.Reader) (bool, error) {
	var checksum uint32
	if err := binary.Read(r, binary.BigEndian, &checksum); err != nil {
		return false, err
	}

	return checksum == mr.crc.Sum32(), nil
}

func (mr *MetaReader) parseZTXt(body *bufio.Reader) (string, []byte, error) {
	key, err := readCStr(body, 79)
	if err != nil {
		return "", nil, err
	}

	if b, err := body.ReadByte(); err != nil {
		return "", nil, err
	} else if b != 0 {
		//The only presently legitimate value for Compression method is 0 (deflate/inflate compression)
		return "", nil, png.FormatError("unknown compression method")
	}

	zr, err := zlib.NewReader(body)
	if err != nil {
		return "", nil, err
	}
	defer zr.Close()

	value := bytes.NewBuffer(make([]byte, 0, bytes.MinRead))
	if _, err := value.ReadFrom(zr); err != nil {
		return "", nil, err
	}

	return key, value.Bytes(), nil
}

func (mr *MetaReader) parseITXt(body *bufio.Reader) (*iTXtChunk, error) {
	var err error
	var iTXt = new(iTXtChunk)
	iTXt.key, err = readCStr(body, 79)
	if err != nil {
		return nil, err
	}

	com, err := body.ReadByte()
	if err != nil {
		return nil, err
	}
	if com == 1 {
		if b, err := body.ReadByte(); err != nil {
			return nil, err
		} else if b != 0 {
			//The only presently legitimate value for Compression method is 0 (deflate/inflate compression)
			return nil, png.FormatError("unknown compression method")
		}
	}

	iTXt.languageTag, err = readCStr(body, -1)
	if err != nil {
		return nil, err
	}

	iTXt.translatedKeyword, err = readCStr(body, -1)
	if err != nil {
		return nil, err
	}

	var r io.Reader = body
	if com == 1 {
		r, err = zlib.NewReader(body)
		if err != nil {
			return nil, err
		}
		defer r.(io.ReadCloser).Close()
	}

	value := bytes.NewBuffer(make([]byte, 0, bytes.MinRead))
	if _, err = value.ReadFrom(r); err != nil {
		return nil, err
	}
	iTXt.text = value.Bytes()

	return iTXt, nil
}

func readCStr(r *bufio.Reader, lim int) (string, error) {
	var str = make([]byte, 0, 32)
	if lim <= 0 {
		lim = math.MinInt32
	}
	for n := 0; n < lim; n++ {
		if b, err := r.ReadByte(); err != nil {
			return "", err
		} else if b == 0 {
			break
		} else {
			str = append(str, b)
		}
	}
	return *(*string)(unsafe.Pointer(&str)), nil
}

func init() {
	imagex.RegisterFormat("png", pngMagic, NewMetaReader, png.Decode)
}
