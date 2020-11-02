package component

import (
	"context"
	"github.com/mocukie/webpdeep/internal/coder"
	"github.com/mocukie/webpdeep/internal/iox"
	"github.com/mocukie/webpdeep/pkg/atomicx"
	"github.com/mocukie/webpdeep/pkg/eventbus"
	"github.com/pkg/errors"
	"os"
	"sync"
	"time"
)

const (
	EvtTransferDone    = "transfer.done"
	EvtTransferJobDone = "transfer.job-done"
)

type Job struct {
	In       iox.Input
	Out      iox.Output
	Codec    coder.Codec
	CopyMeta bool
	Err      error
	Warnings []error
}

func (job *Job) do() {
	var (
		in   = job.In
		out  = job.Out
		info os.FileInfo
	)

	defer func() {
		if e := in.Close(); e != nil && job.Err == nil {
			job.Err = errors.WithStack(e)
		}
		if e := out.Close(); e != nil && job.Err == nil {
			job.Err = errors.WithStack(e)
		}
	}()

	if err := in.Open(); err != nil {
		job.Err = errors.WithStack(err)
		return
	}

	if job.CopyMeta {
		var err error
		info, err = in.Info()
		if err != nil {
			job.Warnings = append(job.Warnings, errors.WithStack(err))
		}
	}

	if err := out.Open(info); err != nil {
		job.Err = errors.WithStack(err)
		return
	}

	e, w := job.Codec.Convert(in, out)
	if e != nil {
		job.Err = errors.WithStack(e)
	}
	job.Warnings = append(job.Warnings, w...)
}

type Transfer struct {
	maxGo    int
	noMore   *atomicx.Bool
	jobQueue <-chan *Job
	eb       *eventbus.Bus
}

func NewTransfer(eb *eventbus.Bus, config *Config) *Transfer {
	return &Transfer{
		noMore:   atomicx.NewBool(false),
		maxGo:    config.MaxGo,
		jobQueue: config.JobQueue,
		eb:       eb,
	}
}

func (tr *Transfer) Start(ctx context.Context) {
	sub := make(eventbus.Subscriber, 1)
	tr.eb.Subscribe(EvtScannerDone, sub)
	tr.noMore.Set(false)
	var wg = new(sync.WaitGroup)
	for i := 0; i < tr.maxGo; i++ {
		wg.Add(1)
		go tr.worker(wg, ctx)
	}

Loop:
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			break Loop
		case msg := <-sub:
			tr.noMore.Set(true)
			wg.Wait()
			sc := msg.Data.(*scannerResult)
			for _, pair := range sc.pp {
				if info, err := os.Stat(pair.src); err == nil {
					_ = os.Chmod(pair.dst, info.Mode())
					_ = os.Chtimes(pair.dst, time.Now(), info.ModTime())
				}
			}
			break Loop
		}
	}

	tr.eb.Publish(EvtTransferDone, nil)
	tr.eb.UnSubscribe(EvtScannerDone, sub)
}

func (tr *Transfer) worker(wg *sync.WaitGroup, ctx context.Context) {
	defer wg.Done()
Loop:
	for {
		select {
		case job := <-tr.jobQueue:
			job.do()
			tr.eb.Publish(EvtTransferJobDone, job)
		case <-ctx.Done():
			break Loop
		default:
			if tr.noMore.T() {
				break Loop
			}
		}
	}
}
