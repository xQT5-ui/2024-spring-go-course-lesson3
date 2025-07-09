package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

const (
	defaultBlockSize = 4096 // block size of bytes for reading data
	minSize          = 0    // difference of bytes for reaching limit
)

// Options for using flags
type Options struct {
	From, To                 string
	Offset, Limit, BlockSize uint64
	Conv                     []string
}

// DataProcessor interface for processing data
type DataProcessor interface {
	Process(src io.Reader, dst io.Writer, opts *Options) error
	Convers(data []byte, conv string) []byte
}

// FileProcessor struct for implementing DataProcessor
type FileProcessor struct{}

// ProcessingContext for store processing data
type ProcessingContext struct {
	src       io.Reader
	dst       io.Writer
	opts      *Options
	buffer    []byte
	totalRead uint64
	processor *FileProcessor
	// fields for processing case with block-size=1
	trimBuffer    []byte // summarized buffer for trim spaces
	hasTrimSpaces bool
}

func newProcessingContext(src io.Reader, dst io.Writer, opts *Options, processor *FileProcessor) *ProcessingContext {
	blockSize := opts.BlockSize
	// set default size
	if blockSize == 0 {
		blockSize = defaultBlockSize
	}

	return &ProcessingContext{
		src:           src,
		dst:           dst,
		opts:          opts,
		buffer:        make([]byte, blockSize),
		totalRead:     0,
		processor:     processor,
		trimBuffer:    make([]byte, 0),
		hasTrimSpaces: slices.Contains(opts.Conv, "trim_spaces"),
	}
}

func validateOptions(options *Options) error {
	// check convertion options
	hasUpper, hasLower := false, false
	for _, v := range options.Conv {
		switch v {
		case "upper_case":
			hasUpper = true
		case "lower_case":
			hasLower = true
		case "trim_spaces":
			// OK
		default:
			return fmt.Errorf("unknown conversion type: %s", v)
		}
	}

	if hasLower && hasUpper {
		return fmt.Errorf("can't use both 'upper_case' and 'lower_case' conversion types")
	}

	return nil
}

func ParseFlags() (*Options, error) {
	var (
		opts                                       Options
		offsetStr, limitStr, blockSizeStr, convStr string
		err                                        error
	)

	// input flags from user
	flag.StringVar(&opts.From, "from", "", "file to read. Default - stdin")
	flag.StringVar(&opts.To, "to", "", "file to write. Default - stdout")
	flag.StringVar(&offsetStr, "offset", "0", "byte's offset to read. Default - 0")
	flag.StringVar(&limitStr, "limit", "0", "byte's limit to read. Default - 0")
	flag.StringVar(&blockSizeStr, "block-size", "0", "byte's block to work. Default - 0")
	flag.StringVar(&convStr, "conv", "", "comma-separated list of conversion types. Possible values: upper_case, lower_case, trim_spaces")

	flag.Parse()

	// parsing number values
	if opts.Offset, err = strconv.ParseUint(offsetStr, 10, 64); err != nil {
		return nil, fmt.Errorf("invalid offset: %w", err)
	}
	if opts.Limit, err = strconv.ParseUint(limitStr, 10, 64); err != nil {
		return nil, fmt.Errorf("invalid limit: %w", err)
	}
	if opts.BlockSize, err = strconv.ParseUint(blockSizeStr, 10, 64); err != nil {
		return nil, fmt.Errorf("invalid block-size: %w", err)
	}

	if convStr != "" {
		opts.Conv = strings.Split(convStr, ",")
		// trim spaces
		for i, v := range opts.Conv {
			opts.Conv[i] = strings.TrimSpace(v)
		}
	}

	if err = validateOptions(&opts); err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	return &opts, nil
}

// Process func for working with main logic for file processing
func (fp *FileProcessor) Process(src io.Reader, dst io.Writer, opts *Options) error {
	// 1. Use offset (skip bytes)
	if err := fp.skipBytes(src, opts.Offset); err != nil {
		return err
	}

	// 2. Block-size reading with max limit
	return fp.processData(src, dst, opts)
}

func (fp *FileProcessor) skipBytes(src io.Reader, offset uint64) error {
	if offset > 0 {
		if _, err := io.CopyN(io.Discard, src, int64(offset)); err != nil {
			return fmt.Errorf("can't skip offset: %w", err)
		}
	}

	return nil
}

func (fp *FileProcessor) processData(src io.Reader, dst io.Writer, opts *Options) error {
	ctx := newProcessingContext(src, dst, opts, fp)

	for {
		// calc correct bytes for reading
		toRead := ctx.calculateReadSize()
		// reach limit
		if toRead == minSize {
			break
		}

		// invoke reading and processing data
		err := ctx.readAndProcess(toRead)
		// check end of file
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read data error: %w", err)
		}
	}

	// 3. Trim spaces if needed
	if err := ctx.finalizeTrimProcessing(); err != nil {
		return fmt.Errorf("can't trim spaces: %w", err)
	}

	return nil
}

func (ctx *ProcessingContext) calculateReadSize() uint64 {
	readSize := uint64(len(ctx.buffer))

	if ctx.opts.Limit > 0 {
		remaining := ctx.opts.Limit - ctx.totalRead

		// check def block size
		if remaining < readSize {
			readSize = remaining
		}
	}

	return readSize
}

func (ctx *ProcessingContext) readAndProcess(toRead uint64) error {
	// read pieces of data
	n, err := ctx.src.Read(ctx.buffer[:toRead])
	if n > 0 {
		if ctx.opts.BlockSize == 1 && ctx.hasTrimSpaces {
			// summarize buffer for finalTrimming
			ctx.trimBuffer = append(ctx.trimBuffer, ctx.buffer[:n]...)
		} else {
			// 3. Apply conversions (Options)
			processedData := ctx.processor.applyConversions(ctx.buffer[:n], ctx.opts.Conv)

			// 4. Write result
			if _, writeErr := ctx.dst.Write(processedData); writeErr != nil {
				return fmt.Errorf("write processed data error: %w", writeErr)
			}
		}

		// set amount of reading bytes
		ctx.totalRead += uint64(n)
	}

	return err
}

func (fp *FileProcessor) applyConversions(data []byte, conversions []string) []byte {
	if len(conversions) == 0 {
		return data
	}

	// apply conversions in input order
	result := data
	for _, conv := range conversions {
		result = fp.Convers(result, conv)
	}

	return result
}

func (fp *FileProcessor) Convers(data []byte, conv string) []byte {
	switch conv {
	case "upper_case":
		return upperCase(data)
	case "lower_case":
		return lowerCase(data)
	case "trim_spaces":
		return trimSpaces(data)
	default:
		return data
	}
}

func (ctx *ProcessingContext) finalizeTrimProcessing() error {
	if ctx.opts.BlockSize == 1 && ctx.hasTrimSpaces && len(ctx.trimBuffer) > 0 {
		// 3. Apply conversions (Options)
		processedData := ctx.processor.applyConversions(ctx.trimBuffer, ctx.opts.Conv)

		// 4. Write result
		if _, err := ctx.dst.Write(processedData); err != nil {
			return fmt.Errorf("write final processed data error: %w", err)
		}
	}

	return nil
}

// instrument funcs
func upperCase(data []byte) []byte {
	str := string(data)
	result := strings.ToUpper(str)

	return []byte(result)
}

func lowerCase(data []byte) []byte {
	str := string(data)
	result := strings.ToLower(str)

	return []byte(result)
}

func trimSpaces(data []byte) []byte {
	if len(data) <= 2 {
		return data
	}

	str := string(data)

	// spacesTrim := regexp.MustCompile(`(?m)(^\n|^\s+\n|\s+\n|^\s+|\s+$)`)
	// result := spacesTrim.ReplaceAllString(str, "")

	result := strings.TrimSpace(str)

	return []byte(result)
}

// getReader return Reader (input contents) from options
func getReader(opts *Options) (io.Reader, func() error, error) {
	if opts.From == "" {
		return os.Stdin, func() error { return nil }, nil
	}

	file, err := os.Open(opts.From)
	if err != nil {
		return nil, func() error { return nil }, fmt.Errorf("can't open input file: %w", err)
	}

	return file, file.Close, nil
}

// getWriter return Writer (output destination) from options
func getWriter(opts *Options) (io.Writer, func() error, error) {
	if opts.To == "" {
		return os.Stdout, func() error { return nil }, nil
	}

	// check existing file
	if _, err := os.Stat(opts.To); err == nil {
		return nil, func() error { return nil }, fmt.Errorf("output file already exists: %s", opts.To)
	}

	// create new file
	file, err := os.Create(opts.To)
	if err != nil {
		return nil, func() error { return nil }, fmt.Errorf("can't create output file: %w", err)
	}

	return file, file.Close, nil
}

func main() {
	opts, err := ParseFlags()
	if err != nil {
		// requirement for using Stderr
		_, _ = fmt.Fprintln(os.Stderr, "can't parse flags:", err)
		os.Exit(1)
	}

	// get reader
	src, srcClose, err := getReader(opts)
	if err != nil {
		// requirement for using Stderr
		_, _ = fmt.Fprintln(os.Stderr, "can't open input file:", err)
		os.Exit(1)
	}
	defer srcClose()

	// get writer
	dst, dstClose, err := getWriter(opts)
	if err != nil {
		// requirement for using Stderr
		_, _ = fmt.Fprintln(os.Stderr, "can't create output file:", err)
		os.Exit(1)
	}
	defer dstClose()

	// processing data
	processor := &FileProcessor{}
	if err := processor.Process(src, dst, opts); err != nil {
		// requirement for using Stderr
		_, _ = fmt.Fprintln(os.Stderr, "can't process file:", err)
		os.Exit(1)
	}
}
