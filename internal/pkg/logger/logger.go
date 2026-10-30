package logger

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogBroadcaster handles broadcasting log messages to WebSocket subscribers
type LogBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[chan []byte]struct{}
}

func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{
		subscribers: make(map[chan []byte]struct{}),
	}
}

// Write implements io.Writer to broadcast logs
func (b *LogBroadcaster) Write(p []byte) (n int, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Create a copy of the data to avoid race conditions
	data := make([]byte, len(p))
	copy(data, p)

	for ch := range b.subscribers {
		select {
		case ch <- data:
		default:
			// Drop message if subscriber is slow
		}
	}
	return len(p), nil
}

func (b *LogBroadcaster) Subscribe() chan []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan []byte, 100) // Buffer 100 logs
	b.subscribers[ch] = struct{}{}
	return ch
}

func (b *LogBroadcaster) Unsubscribe(ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		close(ch)
	}
}

// AsyncRotateWriter implements io.Writer with async, batch, and drop capabilities
type AsyncRotateWriter struct {
	filename string
	maxSize  int64

	ch      chan []byte
	file    *os.File
	size    int64
	dropCnt uint64

	closeCh chan struct{}
}

// NewAsyncRotateWriter creates a new AsyncRotateWriter
func NewAsyncRotateWriter(filename string, maxSizeMB int, buffer int) *AsyncRotateWriter {
	w := &AsyncRotateWriter{
		filename: filename,
		maxSize:  int64(maxSizeMB) * 1024 * 1024,
		ch:       make(chan []byte, buffer),
		closeCh:  make(chan struct{}),
	}

	w.openFile()
	go w.loop()

	return w
}

// openFile opens the log file for writing
func (w *AsyncRotateWriter) openFile() error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(w.filename), 0755); err != nil {
		return err
	}

	// Open file in append mode
	f, err := os.OpenFile(w.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Get current file size
	info, err := f.Stat()
	if err == nil {
		w.size = info.Size()
	}

	// Close old file if exists
	if w.file != nil {
		w.file.Close()
	}

	w.file = f
	return nil
}

// rotate rotates the log file
func (w *AsyncRotateWriter) rotate() error {
	// Close current file
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}

	// Create archive filename
	timestamp := time.Now().Format("20060102_150405")
	ext := filepath.Ext(w.filename)
	base := strings.TrimSuffix(w.filename, ext)
	archiveName := base + "_" + timestamp + ext + ".gz"

	// P1-16 防护（2026-10-13 审查）：压缩失败（含中途失败）绝不删除源文件。
	// 此前 compressLogFile 读取中途出错也返回 nil，生成截断 .gz 后源日志被删，
	// 这段日志永久丢失。现在：压缩失败保留源文件；成功才删除。
	if err := w.compressLogFile(w.filename, archiveName); err != nil {
		os.Stderr.WriteString("Log rotation: compress failed, keeping original file: " + err.Error() + "\n")
	} else {
		// Remove the original file after successful compression
		os.Remove(w.filename)
		// 清理超量历史归档，防止磁盘占满反噬 bbolt 写入
		w.pruneOldArchives(30)
	}

	// Open new file
	return w.openFile()
}

// compressLogFile compresses a log file using gzip.
// 所有错误路径（打开/写入/关闭 flush）都必须返回错误——中途失败时调用方
// 会保留源文件，截断的 .gz 也不会被误当作成功归档。
func (w *AsyncRotateWriter) compressLogFile(source, target string) error {
	// Open the source file
	// #nosec G304 -- source/target 由日志轮转器自身生成的文件路径，非外部输入
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	// Create the target gzip file
	// #nosec G304 -- 同上，轮转归档目标路径由轮转器生成
	dst, err := os.Create(target)
	if err != nil {
		return err
	}

	// Create a gzip writer
	gzw := gzip.NewWriter(dst)

	// Copy the source file to the gzip writer
	buf := make([]byte, 1024*1024) // 1MB buffer
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := gzw.Write(buf[:n]); werr != nil {
				gzw.Close()
				dst.Close()
				return werr
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				gzw.Close()
				dst.Close()
				return rerr
			}
			break
		}
	}

	// Close 错误必须检查：gzip 尾部/flush 失败 = 归档不完整
	if err := gzw.Close(); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return nil
}

// pruneOldArchives 清理超量的历史 .gz 归档（保留最新 maxKeep 份）。
// P1-16 防护：此前 .gz 无保留上限，logs/ 持续增长，磁盘占满反噬 bbolt 写入。
func (w *AsyncRotateWriter) pruneOldArchives(maxKeep int) {
	if maxKeep <= 0 {
		maxKeep = 30
	}
	ext := filepath.Ext(w.filename)
	base := strings.TrimSuffix(w.filename, ext)
	matches, err := filepath.Glob(base + "_*" + ext + ".gz")
	if err != nil || len(matches) <= maxKeep {
		return
	}
	sort.Strings(matches) // 文件名含时间戳，字典序即时间序
	for _, old := range matches[:len(matches)-maxKeep] {
		if err := os.Remove(old); err != nil {
			os.Stderr.WriteString("Log rotation: failed to prune old archive " + old + ": " + err.Error() + "\n")
		}
	}
}

// Write implements io.Writer (non-blocking, can drop)
func (w *AsyncRotateWriter) Write(p []byte) (int, error) {
	select {
	case w.ch <- append([]byte(nil), p...): // Copy to avoid race
	default:
		// Drop log if channel is full
		w.dropCnt++
		// Log drop warning occasionally
		if w.dropCnt%1000 == 0 {
			os.Stderr.WriteString("Log dropped: " + strconv.FormatUint(w.dropCnt, 10) + "\n")
		}
	}
	return len(p), nil
}

// loop processes log messages in background
func (w *AsyncRotateWriter) loop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	buffer := make([][]byte, 0, 100)

	for {
		select {
		case p := <-w.ch:
			buffer = append(buffer, p)

			// Batch write when buffer is full
			if len(buffer) >= 50 {
				w.flush(buffer)
				buffer = buffer[:0]
			}

		case <-ticker.C:
			if len(buffer) > 0 {
				w.flush(buffer)
				buffer = buffer[:0]
			}

		case <-w.closeCh:
			if len(buffer) > 0 {
				w.flush(buffer)
			}
			return
		}
	}
}

// flush writes buffered logs to file
func (w *AsyncRotateWriter) flush(buf [][]byte) {
	for _, p := range buf {
		if w.size+int64(len(p)) > w.maxSize {
			w.rotate()
		}

		if w.file != nil {
			n, _ := w.file.Write(p)
			w.size += int64(n)
		}
	}
}

// Close closes the writer
func (w *AsyncRotateWriter) Close() error {
	close(w.closeCh)
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Stats returns the number of dropped logs
func (w *AsyncRotateWriter) Stats() uint64 {
	return w.dropCnt
}

// InitLogger initializes the global logger
func InitLogger(logLevel string, logFile string, broadcaster *LogBroadcaster) (*zap.Logger, error) {
	// Parse log level
	level := zap.InfoLevel
	if logLevel != "" {
		if l, err := zapcore.ParseLevel(strings.ToLower(logLevel)); err == nil {
			level = l
		}
	}

	// 1. Console Encoder (Colorized)
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)

	// 2. File Encoder (Standard text)
	fileEncoderConfig := zap.NewProductionEncoderConfig()
	fileEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewConsoleEncoder(fileEncoderConfig)

	// 3. JSON Encoder for WebSocket (Easier to parse in frontend)
	jsonEncoderConfig := zap.NewProductionEncoderConfig()
	jsonEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(jsonEncoderConfig)

	cores := []zapcore.Core{}

	// Console Core
	cores = append(cores, zapcore.NewCore(
		consoleEncoder,
		zapcore.Lock(os.Stdout),
		level,
	))

	// File Core
	if logFile != "" {
		// Create async rotate writer with 10MB max size and 1000 buffer
		rw := NewAsyncRotateWriter(logFile, 10, 1000)
		cores = append(cores, zapcore.NewCore(
			fileEncoder,
			zapcore.AddSync(rw),
			level,
		))
	}

	// Broadcaster Core (WebSocket)
	if broadcaster != nil {
		// Always allow Debug logs for WebSocket to support real-time monitoring
		// regardless of the file/console log level.
		cores = append(cores, zapcore.NewCore(
			jsonEncoder,
			zapcore.AddSync(broadcaster),
			zap.DebugLevel,
		))
	}

	// Combine all cores and enrich JSON logs with category/channel/device metadata.
	core := wrapMetadataCore(zapcore.NewTee(cores...))

	// Create logger
	logger := zap.New(core, zap.AddCaller())

	// Replace global logger
	zap.ReplaceGlobals(logger)
	zap.RedirectStdLog(logger)

	return logger, nil
}
