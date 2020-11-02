package component

import (
	"context"
	"fmt"
	"github.com/mattn/go-colorable"
	"github.com/mocukie/webpdeep/internal/coder"
	"github.com/mocukie/webpdeep/pkg/eventbus"
	"io"
	"log"
	"time"
)

var requireTopics = []eventbus.Topic{
	EvtScannerNewJob,
	EvtScannerError,
	EvtScannerDone,
	EvtTransferJobDone,
	EvtTransferDone,
}

type counter struct {
	v int
	t int
}

type Monitor struct {
	eb         *eventbus.Bus
	config     *Config
	errLog     *log.Logger
	warnLog    *log.Logger
	infoLog    *log.Logger
	Convert    counter
	Copy       counter
	Errs       int
	Warnings   int
	jobCount   counter
	scannerErr int
	startTime  time.Time
}

func NewMonitor(eb *eventbus.Bus, config *Config, logOut io.Writer) *Monitor {
	m := &Monitor{eb: eb, config: config}
	flag := log.LstdFlags | log.Lmicroseconds
	m.errLog = log.New(logOut, "[ERROR] ", flag)
	m.warnLog = log.New(logOut, "[WARN ] ", flag)
	m.infoLog = log.New(logOut, "[INFO ] ", flag)

	return m
}

func (mo *Monitor) Start(ctx context.Context) {
	var (
		sub          = mo.subscribe()
		transferDone = false
		t1s          = time.NewTicker(1 * time.Second)
		t30s         = time.NewTicker(30 * time.Second)
		sc           *scannerResult
	)
	mo.startTime = time.Now()
	mo.hideCursor()
Loop:
	for {
		select {
		case msg := <-sub:
			switch msg.Topic {
			case EvtTransferDone:
				transferDone = true
			case EvtScannerDone:
				sc, _ = msg.Data.(*scannerResult)
			default:
				mo.precessEvent(msg)
			}
		case <-t1s.C:
			mo.updateConsole()
		case <-t30s.C:
			mo.logCounter()
		case <-ctx.Done():
			break Loop
		default:
			if transferDone && sc != nil &&
				mo.jobCount.v == sc.jobCount &&
				mo.jobCount.t == sc.jobCount &&
				mo.scannerErr == sc.errCount { //wait for last job event
				break Loop
			}
		}
	}
	t1s.Stop()
	t30s.Stop()
	fmt.Println()
	mo.showCursor()
	mo.unSubscribe(sub)
	mo.logCounter()
}

func (mo *Monitor) subscribe() eventbus.Subscriber {
	sub := make(eventbus.Subscriber, 512)
	for _, topic := range requireTopics {
		mo.eb.Subscribe(topic, sub)
	}
	return sub
}

func (mo *Monitor) unSubscribe(sub eventbus.Subscriber) {
	for _, topic := range requireTopics {
		mo.eb.UnSubscribe(topic, sub)
	}
}

func (mo *Monitor) precessEvent(msg eventbus.Message) {
	switch msg.Topic {
	case EvtScannerNewJob:
		job := msg.Data.(*Job)
		switch job.Codec.(type) {
		case *coder.Copy:
			mo.Copy.t++
		case *coder.WebP:
			mo.Convert.t++
		}
		mo.jobCount.v++
	case EvtTransferJobDone:
		mo.jobCount.t++
		job, _ := msg.Data.(*Job)
		if job.Err != nil {
			mo.Errs++
			mo.errLog.Printf("[Transfer] <%s> -> <%s>\n%+v\n", job.In.Path(), job.Out.Path(), job.Err)
		} else {
			switch job.Codec.(type) {
			case *coder.Copy:
				mo.Copy.v++
			case *coder.WebP:
				mo.Convert.v++
			}
		}
		mo.Warnings += len(job.Warnings)
		for _, warn := range job.Warnings {
			mo.warnLog.Printf("[Transfer] <%s> -> <%s>\n%+v\n", job.In.Path(), job.Out.Path(), warn)
		}
	case EvtScannerError:
		mo.scannerErr++
		mo.Errs++
		err, _ := msg.Data.(error)
		mo.errLog.Printf("[Scanner] %+v\n", err)

	}
	mo.updateConsole()
}

func (mo *Monitor) updateConsole() {
	fmt.Print("\r")
	mo.printCounter()
}

func (mo *Monitor) printCounter() {
	fmt.Printf("\x1b[36mconv\x1b[0m: %d/%d | \x1b[32mcopy\x1b[0m: %d/%d | \x1B[31merror\x1b[0m: %d | \x1b[33mwarn\x1B[0m: %d | elapsed: %10v",
		mo.Convert.v, mo.Convert.t, mo.Copy.v, mo.Copy.t, mo.Errs, mo.Warnings, time.Since(mo.startTime))
}

func (mo Monitor) logCounter() {
	mo.infoLog.Printf("conv: %d/%d | copy: %d/%d | error: %d | warn: %d | elapsed: %10v\n",
		mo.Convert.v, mo.Convert.t, mo.Copy.v, mo.Copy.t, mo.Errs, mo.Warnings, time.Since(mo.startTime))
}

func (mo *Monitor) hideCursor() {
	fmt.Print("\033[?25l")
}
func (mo *Monitor) showCursor() {
	fmt.Print("\033[?25h")
}

func init() {
	b := true
	colorable.EnableColorsStdout(&b)
}
