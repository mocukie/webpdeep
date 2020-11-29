package main

import "C"
import (
	"context"
	"fmt"
	"github.com/mocukie/webp-go/webp"
	"github.com/mocukie/webpdeep/internal/component"
	"github.com/mocukie/webpdeep/pkg/eventbus"
	"github.com/mocukie/webpdeep/pkg/imagex/pngx"
	"github.com/pkg/errors"
	flag "github.com/spf13/pflag"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"gopkg.in/vrecan/death.v3"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

var (
	convertPattern string
	copyPattern    string
	archivePattern string

	quality        float32
	preset         string
	losslessPreset int
	strong         bool
	alphaFilter    string
	hint           string

	cmdFlags        *flag.FlagSet
	configFlags     *flag.FlagSet
	webpPresetFlags *flag.FlagSet
	webpMainFlags   *flag.FlagSet
	webpExFlags     *flag.FlagSet
)

func initConfig() *component.Config {
	var conf = new(component.Config)
	configFlags = flag.NewFlagSet("configFlags", flag.ContinueOnError)
	configFlags.BoolVarP(&conf.Recursively, "recursive", "r", false, "scan input directory recursively")
	configFlags.StringVarP(&convertPattern, "pattern", "p", "*.png|*.jpg|*.bmp|*.tiff", "convert glob pattern in batch mode")
	configFlags.StringVar(&copyPattern, "copy", "", "copy glob pattern in batch mode")
	configFlags.Lookup("copy").NoOptDefVal = "*"
	configFlags.StringVar(&archivePattern, "archive", "*.zip|*.cbz", "archive glob pattern in batch mode")
	configFlags.BoolVar(&conf.CopyFileMeta, "file_meta", false, "copy file metadata")
	configFlags.BoolVar(&conf.CopyImageMeta, "image_meta", false, "copy image metadata")
	configFlags.BoolVar(&conf.CheckImage, "check_image", false, "check output image in lossless mode")
	configFlags.IntVar(&conf.MaxGo, "max_go", runtime.NumCPU(), "max thread number")
	configFlags.StringVarP(&conf.Dest, "output", "o", "", "output path, can be omitted in single image mode")
	configFlags.StringVar(&conf.LogPath, "log", "", "log file path")
	configFlags.SortFlags = false
	return conf
}

func setupConfig(conf *component.Config) error {
	var err error

	conf.ConvertMatch, err = component.NewGlobMatcher(convertPattern)
	if err != nil {
		return errors.WithMessage(err, "invalid convert pattern: "+convertPattern)
	}

	if copyPattern != "" {
		conf.CopyMatch, err = component.NewGlobMatcher(copyPattern)
		if err != nil {
			return errors.WithMessage(err, "invalid copy pattern: "+convertPattern)
		}
	}

	conf.ArchiveMatch, err = component.NewGlobMatcher(archivePattern)
	if err != nil {
		return errors.WithMessage(err, "invalid archive pattern: "+archivePattern)
	}

	if conf.MaxGo <= 0 {
		conf.MaxGo = runtime.NumCPU()
	}

	conf.Src = cmdFlags.Arg(0)
	if conf.Src == "" {
		return errors.New("input not specify")
	}

	conf.Src = filepath.Clean(conf.Src)
	stat, err := os.Stat(conf.Src)

	if !cmdFlags.Lookup("output").Changed {
		if err == nil && !stat.IsDir() {
			conf.Dest = conf.Src[:len(conf.Src)-len(filepath.Ext(conf.Src))] + ".webp"
		} else {
			return errors.New("output not specify")
		}
	}
	conf.Dest = filepath.Clean(conf.Dest)

	if err == nil && !cmdFlags.Lookup("log").Changed {
		if stat.IsDir() {
			conf.LogPath = conf.Dest
		} else {
			conf.LogPath = filepath.Dir(conf.Dest)
		}
	}
	conf.LogPath = filepath.Clean(conf.LogPath)

	return nil
}

func initEncodeOption() (*webp.EncodeOptions, error) {
	var (
		err  error
		opts *webp.EncodeOptions
	)

	webpPresetFlags = flag.NewFlagSet("webpPresetFlags", flag.ContinueOnError)
	webpPresetFlags.Usage = func() {}
	webpPresetFlags.Float32VarP(&quality, "quality", "q", webp.LossyDefaultQuality, "quality factor (0:small..100:big)")
	webpPresetFlags.StringVar(&preset, "preset", "default", "preset setting, one of: default, photo, picture, drawing, icon, text")
	_ = webpPresetFlags.Parse(os.Args[1:])
	opts, err = webp.NewEncOptionsByPreset(presetStringToVal(preset), quality)
	if err != nil {
		return nil, errors.WithMessagef(err, "invalid preset <%s> or quality <%f>", preset, quality)
	}

	webpMainFlags = flag.NewFlagSet("webpMainFlags", flag.ContinueOnError)
	webpMainFlags.BoolVar(&opts.Lossless, "lossless", false, "encode image losslessly")
	webpMainFlags.IntVarP(&opts.Method, "method", "m", opts.Method, "compression method (0=fast, 6=slowest)")
	webpMainFlags.IntVarP(&losslessPreset, "z", "z", 0, "activates lossless preset with given level in [0:fast, ..., 9:slowest]")
	webpMainFlags.Lookup("z").NoOptDefVal = strconv.Itoa(webp.LosslessDefaultLevel)
	////
	webpMainFlags.IntVar(&opts.Segments, "segments", opts.Segments, "number of segments to use (1..4)")
	webpMainFlags.IntVar(&opts.TargetSize, "size", opts.TargetSize, "target size (in bytes)")
	webpMainFlags.Float32Var(&opts.TargetPSNR, "psnr", opts.TargetPSNR, "target PSNR (in dB. typically: 42)")
	webpMainFlags.IntVar(&opts.SnsStrength, "sns", opts.SnsStrength, "spatial noise shaping (0:off, 100:max)")
	webpMainFlags.IntVarP(&opts.FilterStrength, "strength", "f", opts.FilterStrength, "filter strength (0=off..100)")
	webpMainFlags.IntVar(&opts.FilterSharpness, "sharpness", opts.FilterSharpness, "filter sharpness (0:most .. 7:least sharp)")
	webpMainFlags.BoolVar(&strong, "strong", opts.FilterType == 1, "use strong filter instead of simple")
	webpMainFlags.BoolVar(&opts.UseSharpYUV, "sharp_yuv", opts.UseSharpYUV, "use sharper (and slower) RGB->YUV conversion")
	webpMainFlags.IntVar(&opts.PartitionLimit, "partition_limit", opts.PartitionLimit, "limit quality to fit the 512k limit on the first partition (0=no degradation ... 100=full)")
	webpMainFlags.IntVar(&opts.Pass, "pass", opts.Pass, "analysis pass number (1..10)")

	webpMainFlags.BoolVar(&opts.ThreadLevel, "mt", opts.ThreadLevel, "use multi-threading if available")
	webpMainFlags.BoolVar(&opts.LowMemory, "low_memory", opts.LowMemory, "reduce memory usage (slower encoding)")
	webpMainFlags.IntVar(&opts.AlphaCompression, "alpha_method", opts.AlphaCompression, "transparency-compression method (0..1), default=1")
	webpMainFlags.StringVar(&alphaFilter, "alpha_filter", "fast", "predictive filtering for alpha plane, one of: none, fast or best")

	webpMainFlags.BoolVar(&opts.Exact, "exact", opts.Exact, "preserve RGB values in transparent area")
	webpMainFlags.IntVar(&opts.NearLossless, "near_lossless", opts.NearLossless, "use near-lossless image preprocessing (0..100=off)")
	webpMainFlags.StringVar(&hint, "hint", "", "specify image characteristics hint, one of: photo, picture or graph")

	//(experimental)
	webpExFlags = flag.NewFlagSet("webpExFlags", flag.ContinueOnError)
	webpExFlags.BoolVar(&opts.EmulateJpegSize, "jpeg_like", opts.EmulateJpegSize, "roughly match expected JPEG size")
	webpExFlags.BoolVar(&opts.AutoFilter, "af", opts.AutoFilter, "auto-adjust filter strength")
	webpExFlags.IntVar(&opts.Preprocessing, "pre", opts.Preprocessing, "pre-processing filter")

	webpPresetFlags.SortFlags = false
	webpMainFlags.SortFlags = false
	webpExFlags.SortFlags = false

	return opts, nil
}

func setupEncodeOptions(opts *webp.EncodeOptions) error {

	if opts.Lossless && !cmdFlags.Lookup("quality").Changed {
		opts.Quality = webp.LosslessDefaultQuality
	}
	//overwrite lossless setting by preset if set
	if cmdFlags.Lookup("z").Changed {
		if err := opts.SetupLosslessPreset(losslessPreset); err != nil {
			return errors.WithMessagef(err, "invalid lossless method(z option): %d", losslessPreset)
		}
	}

	if strong {
		opts.FilterType = 1
	} else {
		opts.FilterType = 0
	}

	if cmdFlags.Lookup("alpha_filter").Changed {
		switch alphaFilter {
		case "none":
			opts.AlphaFiltering = 0
		case "fast":
			opts.AlphaFiltering = 1
		case "best":
			opts.AlphaFiltering = 2
		default:
			return errors.New("invalid alpha_filter: " + alphaFilter)
		}
	}

	if cmdFlags.Lookup("hint").Changed {
		switch hint {
		case "photo":
			opts.ImageHint = webp.HintPhoto
		case "picture":
			opts.ImageHint = webp.HintPicture
		case "graph":
			opts.ImageHint = webp.HintGraph
		default:
			return errors.New("invalid image hint: " + hint)
		}
	}

	return opts.Validate()
}

func presetStringToVal(s string) webp.EncodePreset {
	switch s {
	case "default":
		return webp.PresetDefault
	case "photo":
		return webp.PresetPhoto
	case "picture":
		return webp.PresetPicture
	case "drawing":
		return webp.PresetDrawing
	case "icon":
		return webp.PresetIcon
	case "text":
		return webp.PresetText
	}
	return -1
}

func printBanner() {
	var banner = ` __    __     _       ___      ___
/ / /\ \ \___| |__   / _ \    /   \___  ___ _ __
\ \/  \/ / _ \ '_ \ / /_)/   / /\ / _ \/ _ \ '_ \
 \  /\  /  __/ |_) / ___/   / /_//  __/  __/ |_) |
  \/  \/ \___|_.__/\/      /___,' \___|\___| .__/
   %39v |_|
==================================================
`
	fmt.Printf(banner, "libwebp: v"+webp.EncoderVersion().String())
}
func printUsage() {
	printBanner()
	fmt.Println("Usage:")
	fmt.Printf("\t%v [options] /path/to/image/or/archive/or/dir -o out/file/or/dir\n", filepath.Base(os.Args[0]))
	fmt.Println()

	fmt.Println("Options:")
	fmt.Print(configFlags.FlagUsages())
	fmt.Println()

	fmt.Println("WebP Encode Options:")
	fmt.Print(webpPresetFlags.FlagUsages())
	fmt.Print(webpMainFlags.FlagUsages())
	fmt.Println()

	fmt.Println("WebP Experimental Encode Options:")
	fmt.Print(webpExFlags.FlagUsages())
}

func main() {
	var err error

	conf := initConfig()
	conf.Opts, err = initEncodeOption()
	if err != nil {
		log.Fatal(err)
	}

	//parse argument
	cmdFlags = flag.NewFlagSet("cmdFlags", flag.ContinueOnError)
	cmdFlags.AddFlagSet(configFlags)
	cmdFlags.AddFlagSet(webpPresetFlags)
	cmdFlags.AddFlagSet(webpMainFlags)
	cmdFlags.AddFlagSet(webpExFlags)
	version := cmdFlags.BoolP("version", "v", false, "print version")
	cmdFlags.Usage = printUsage
	cmdFlags.SortFlags = false
	err = cmdFlags.Parse(os.Args[1:])
	if err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if *version {
		fmt.Printf("WebP encoder version: v%v\n", webp.EncoderVersion())
		os.Exit(0)
	}

	err = setupConfig(conf)
	if err != nil {
		log.Fatal(err)
	}

	err = setupEncodeOptions(conf.Opts)
	if err != nil {
		log.Fatal(err)
	}

	if err = os.MkdirAll(conf.LogPath, os.ModePerm); err != nil {
		log.Fatalf("can not make log directory <%s>, %v", conf.LogPath, err)
	}
	conf.LogPath = filepath.Join(conf.LogPath, time.Now().Format("webpdeep-2006-01-02T15.04.05Z07.00.log"))
	logOut, err := os.Create(conf.LogPath)
	if err != nil {
		log.Fatalf("can not create log file %v", err)
	}
	defer logOut.Close()

	conf.JobQueue = make(chan *component.Job, 1024)
	var (
		eb         = eventbus.New()
		transfer   = component.NewTransfer(eb, conf)
		monitor    = component.NewMonitor(eb, conf, logOut)
		scanner    = component.NewPathScanner(eb, conf)
		ctx, abort = context.WithCancel(context.Background())
	)

	hook := death.NewDeath(syscall.SIGINT, syscall.SIGTERM)
	go hook.WaitForDeathWithFunc(abort)

	printBanner()
	go transfer.Start(ctx)
	go scanner.Scan(ctx)
	monitor.Start(ctx)
	fmt.Println("\nDone.")
}

//register format
var _ = pngx.NewMetaReader
var _ = jpeg.Decode
var _ = tiff.Decode
var _ = bmp.Decode
