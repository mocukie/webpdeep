package zipx

import (
	"archive/zip"
	"encoding/binary"
	"hash/crc32"
	"unsafe"
)

func DetectZipUTF8Path(fh *zip.FileHeader) (string, bool) {
	if !fh.NonUTF8 { //utf8 already
		return fh.Name, false
	}

	var name = fh.Name
	var nonUtf8 = true
	var pos = 0
	for pos < len(fh.Extra) {
		tag := binary.LittleEndian.Uint16(fh.Extra[pos : pos+2])
		pos += 2
		size := int(binary.LittleEndian.Uint16(fh.Extra[pos : pos+2]))
		pos += 2
		/* zip format specification https://pkware.cachefly.net/webdocs/casestudies/APPNOTE.TXT
		   4.6.9 -Info-ZIP Unicode Path Extra Field (0x7075):
		   Stores the UTF-8 version of the file name field as stored in the
		   local header and central directory header. (Last Revision 20070912)

		   Value         Size        Description
		   -----         ----        -----------
		   0x7075        Short       tag for this extra block type ("up")
		   TSize         Short       total data size for this block
		   Version       1 byte      version of this extra field, currently 1
		   NameCRC32     4 bytes     File Name Field CRC32 Checksum
		   UnicodeName   Variable    UTF-8 version of the entry File Name

		   The NameCRC32 is the standard zip CRC32 checksum of the File Name
		   field in the header.  This is used to verify that the header
		   File Name field has not changed since the Unicode Path extra field
		   was created.  This can happen if a utility renames the File Name but
		   does not update the UTF-8 path extra field.  If the CRC check fails,
		   this UTF-8 Path Extra Field SHOULD be ignored and the File Name field
		   in the header SHOULD be used instead
		*/
		if tag == 0x7075 {
			data := fh.Extra[pos : pos+size]
			crc := binary.LittleEndian.Uint32(data[1:5])
			if crc == crc32.ChecksumIEEE([]byte(fh.Name)) {
				_name := data[5:]
				name = *(*string)(unsafe.Pointer(&_name))
				nonUtf8 = false
				break
			}
		}
		pos += size
	}
	return name, nonUtf8
}
