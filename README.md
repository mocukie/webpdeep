# WebPDeep
A webp batch encoding tool.

## Feature
* Support jpeg/png/bmp/tiff as input
* Copy ICCP/XMP/EXIF (png only)
* Copy file mtime/atime
* Using zip as a directory
* Recursive conversion

## Usage
```shell script
webpdeep [options] /path/to/image/or/archive/or/dir -o out/file/or/dir
```

Single image mode
```shell script
webpdeep --lossless -q 75 image.png [-o image.webp]
```

Single zip/folder mode
```shell script
webpdeep --lossless -q 75 -p "*.png|*.bmp" in.zip -o out.zip
webpdeep --lossless -q 75 -p "*.png|*.bmp" ./in -o ./out
```

Recursive directory mode
```shell script
webpdeep -r --lossless -q 75 -p "*.png|*.bmp" ./in -o ./out
```

More information see ```--help``` option


## Install
Prerequisite 
* gcc
* libwebp 1.1.0

```shell script
go get -ldflags="-extldflags -static" github.com/mocukie/webpdeep/cmd/webpdeep
```

## License
[Apache-2.0 License](LICENSE)