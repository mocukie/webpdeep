package component

import (
	"github.com/mocukie/webp-go/webp"
	"path"
	"path/filepath"
	"strings"
)

type PathMatcher func(pathname string, depPlatform bool) bool

type Config struct {
	Src           string
	Dest          string
	Recursively   bool
	ConvertMatch  PathMatcher
	CopyMatch     PathMatcher
	ArchiveMatch  PathMatcher
	CopyFileMeta  bool
	CopyImageMeta bool
	CheckImage    bool
	MaxGo         int
	LogPath       string
	Opts          *webp.EncodeOptions
	JobQueue      chan *Job
}

func NewGlobMatcher(pattern string) (PathMatcher, error) {
	patterns := strings.Split(pattern, "|")
	for _, s := range patterns {
		_, err := path.Match(s, "foobar")
		if err != nil {
			return nil, err
		}
	}

	return func(pathname string, depPlatform bool) bool {
		var name string
		if depPlatform {
			name = filepath.Ext(pathname)
		} else {
			name = path.Ext(pathname)
		}
		var ok = false
		for _, s := range patterns {
			if ok, _ = path.Match(s, name); ok {
				break
			}
		}
		return ok
	}, nil
}
