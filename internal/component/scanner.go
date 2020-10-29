package component

import (
    "archive/zip"
    "context"
    "github.com/karrick/godirwalk"
    "github.com/mocukie/webpdeep/internal/coder"
    "github.com/mocukie/webpdeep/internal/iox"
    "github.com/mocukie/webpdeep/pkg/eventbus"
    "github.com/mocukie/webpdeep/pkg/zipx"
    "github.com/pkg/errors"
    "os"
    "path"
    "path/filepath"
    "time"
)

const (
    EvtScannerNewJob = "scanner.new-job"
    EvtScannerDone   = "scanner.done"
    EvtScannerError  = "scanner.error"
)

type pathPair struct {
    src string
    dst string
}

type scannerResult struct {
    pp       []pathPair
    jobCount int
    errCount int
}

type PathScanner struct {
    config *Config
    eb     *eventbus.Bus
    result *scannerResult
}

//config.Src and config.Dest must be cleaned by filepath.Clean first
func NewPathScanner(eb *eventbus.Bus, config *Config) *PathScanner {
    return &PathScanner{eb: eb, config: config, result: new(scannerResult)}
}

func (sc *PathScanner) Scan(ctx context.Context) {
    var (
        conf = sc.config
        err  error
        stat os.FileInfo
    )
    defer sc.eb.Publish(EvtScannerDone, sc.result)

    stat, err = os.Stat(conf.Src)
    if err != nil {
        sc.handleError(errors.Wrapf(err, "get <%s> stat failed", conf.Src))
        return
    }

    if stat.IsDir() {
        err = godirwalk.Walk(conf.Src, &godirwalk.Options{
            Callback: sc.walkDir,
            ErrorCallback: func(s string, e error) godirwalk.ErrorAction {
                sc.handleError(errors.Wrapf(err, "walk on file node <%s> failed", s))
                return godirwalk.SkipNode
            },
        })
        if err != nil {
            sc.handleError(errors.Wrapf(err, "can not walk directory <%s>", conf.Src))
        }
        return
    }

    dir := filepath.Dir(conf.Dest)
    if err = os.MkdirAll(dir, os.ModePerm); err != nil {
        sc.handleError(errors.Wrapf(err, "can not make output directory <%s>", dir))
        return
    }

    if conf.ArchiveMatch(conf.Src, true) {
        sc.walkZip(conf.Src, conf.Dest)
    } else {
        job := new(Job)
        job.CopyMeta = conf.CopyFileMeta
        job.Codec = &coder.WebP{Opts: conf.Opts, CopyMeta: conf.CopyImageMeta, CheckImage: conf.CheckImage}
        job.In = iox.NewFileInput(conf.Src, nil)
        job.Out = iox.NewFileOutput(conf.Dest)
        sc.sendJob(job)
    }

}

func (sc *PathScanner) walkDir(pathname string, de *godirwalk.Dirent) error {
    var conf = sc.config
    if conf.Src == pathname {
        if err := os.MkdirAll(conf.Dest, os.ModePerm); err != nil {
            sc.handleError(errors.Wrapf(err, "can not make dest directory <%s>", conf.Dest))
            return godirwalk.SkipThis
        }
        return nil
    } else if conf.Dest == pathname || conf.LogPath == pathname {
        return godirwalk.SkipThis
    }

    rel, _ := filepath.Rel(conf.Src, pathname)
    outPathname := filepath.Join(conf.Dest, rel)
    if de.IsDir() {
        var skip error
        if conf.Recursively {
            if err := os.MkdirAll(outPathname, os.ModePerm); err != nil {
                sc.handleError(errors.Wrapf(err, "can not make dest directory <%s>", outPathname))
                skip = godirwalk.SkipThis
            } else if conf.CopyFileMeta {
                sc.result.pp = append(sc.result.pp, pathPair{src: pathname, dst: outPathname})
            }
        } else {
            skip = godirwalk.SkipThis
        }
        return skip
    }

    if conf.ArchiveMatch(pathname, true) {
        if conf.Recursively {
            sc.walkZip(pathname, outPathname)
        }
        return nil
    }

    var (
        job      *Job
        copyMeta = conf.CopyFileMeta
    )
    if conf.ConvertMatch(pathname, true) {
        job = new(Job)
        job.Codec = &coder.WebP{Opts: conf.Opts, CopyMeta: conf.CopyImageMeta, CheckImage: conf.CheckImage}
        outPathname = outPathname[:len(outPathname)-len(filepath.Ext(outPathname))] + ".webp"
    } else if conf.CopyMatch != nil && conf.CopyMatch(pathname, true) {
        job = new(Job)
        job.Codec = &coder.Copy{}
        copyMeta = true
    } else {
        return nil
    }

    job.CopyMeta = copyMeta
    job.In = iox.NewFileInput(pathname, nil)
    job.Out = iox.NewFileOutput(outPathname)
    sc.sendJob(job)
    return nil
}

func (sc *PathScanner) walkZip(pathname, outPathname string) {
    conf := sc.config
    reader, err := zip.OpenReader(pathname)
    if err != nil {
        sc.handleError(errors.Wrapf(err, "can not open archive <%s>", pathname))
        return
    }
    defer reader.Close()

    var (
        jobs []*Job
        dirs []*zip.FileHeader
    )
    for _, entry := range reader.File {
        if entry.Mode().IsDir() {
            fh := entry.FileHeader
            fh.Name, fh.NonUTF8 = zipx.DetectZipUTF8Path(&entry.FileHeader)
            dirs = append(dirs, &fh)
            continue
        }

        var (
            job        *Job
            copyMeta   = conf.CopyFileMeta
            outName, _ = zipx.DetectZipUTF8Path(&entry.FileHeader)
        )
        if conf.ConvertMatch(entry.Name, false) {
            job = new(Job)
            job.Codec = &coder.WebP{Opts: conf.Opts, CopyMeta: conf.CopyImageMeta, CheckImage: conf.CheckImage}
            outName = outName[:len(outName)-len(path.Ext(outName))] + ".webp"
        } else if conf.CopyMatch != nil && conf.CopyMatch(entry.Name, false) {
            job = new(Job)
            job.Codec = &coder.Copy{}
            copyMeta = true
        } else {
            continue
        }
        job.CopyMeta = copyMeta
        job.In, _ = iox.NewZipInput(pathname + iox.NestSeparator + entry.Name)
        job.Out, _ = iox.NewZipOutput(outPathname + iox.NestSeparator + outName)
        jobs = append(jobs, job)
    }

    if len(jobs) == 0 {
        return
    }

    f, err := os.Create(outPathname)
    if err != nil {
        sc.handleError(errors.Wrapf(err, "can not create archive <%s>", outPathname))
        return
    }

    zw := iox.NewZipWriter(f, int32(len(jobs)))
    for _, dir := range dirs {
        if !conf.CopyFileMeta {
            dir.Modified = time.Now()
            dir.SetMode(os.ModePerm)
        }
        _, err := zw.CreateHeader(dir)
        if err != nil {
            sc.handleError(errors.Wrapf(err, "can not create archive entry <%s%s%s>", outPathname, iox.NestSeparator, dir.Name))
        }
    }
    for _, job := range jobs {
        job.Out.(*iox.ZipOutput).SetZipWriter(zw)
        sc.sendJob(job)
    }

    if conf.CopyFileMeta {
        sc.result.pp = append(sc.result.pp, pathPair{src: pathname, dst: outPathname})
    }
}

func (sc *PathScanner) handleError(err error) {
    sc.result.errCount++
    sc.eb.Publish(EvtScannerError, err)
}

func (sc *PathScanner) sendJob(job *Job) {
    sc.result.jobCount++
    sc.eb.Publish(EvtScannerNewJob, job)
    sc.config.JobQueue <- job
}
