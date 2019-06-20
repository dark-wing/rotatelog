package rotatelog

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Level describes the level of a log message.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelNotice
	LevelWarning
	LevelError
	LevelCritical
)

const (
	tagDebug    = "[Debug] "
	tagInfo     = "[Info] "
	tagNotice   = "[Notice] "
	tagWarning  = "[Warning] "
	tagError    = "[Error] "
	tagCritical = "[Critical] "
	formatMin   = "200601021504"
	formatSec   = "20060102150405"
)

var (
	// Leveltags maps log levels to names
	levelTags = map[Level]string{
		LevelDebug:    tagDebug,
		LevelInfo:     tagInfo,
		LevelNotice:   tagNotice,
		LevelWarning:  tagWarning,
		LevelError:    tagError,
		LevelCritical: tagCritical,
	}

	levelNames = map[string]Level{
		"debug":    LevelDebug,
		"info":     LevelInfo,
		"notice":   LevelNotice,
		"warning":  LevelWarning,
		"error":    LevelError,
		"critical": LevelCritical,
	}
)

func NewLevel(name string) Level {
	if l, ok := levelNames[name]; ok {
		return l
	}
	return LevelError
}

// String returns the string representation of the log level
func (l Level) String() string {
	if name, ok := levelTags[l]; ok {
		return name
	}
	return "[Unknown] "
}

type RotateConfig struct {
	Rotate   int           // keeped log files count
	Duration time.Duration // log rotate duration
	Compress bool

	// whether start a go routine to rotate log
	StartRoutine bool
}

type Logger struct {
	*log.Logger
	Level Level

	w io.Writer

	rotateCfg    *RotateConfig
	suffixFormat string
}

// @see log.New
func New(out io.Writer, prefix string, flag int, level Level, rc *RotateConfig) *Logger {
	l := &Logger{
		Logger:    log.New(out, prefix, flag),
		Level:     level,
		w:         out,
		rotateCfg: rc,
	}

	if rc != nil && rc.StartRoutine {
		go l.startRotateRoutine()
	}
	return l
}

func (l *Logger) SetOutput(w io.Writer) {
	l.w = w
	l.Logger.SetOutput(w)
}

func (l *Logger) SetLevel(level Level) {
	l.Level = level
}

func (l *Logger) Rotate() (err error) {

	if l.rotateCfg.Duration < time.Minute {
		l.suffixFormat = formatSec
	} else {
		l.suffixFormat = formatMin
	}

	var (
		fd       *os.File
		fileName string
	)
	switch f := l.w.(type) {
	case *os.File:
		fd = f
		fileName = fd.Name()
	default:
		return
	}

	suffix := l.genSuffixStr()
	targetLogName := fmt.Sprintf("%s.%s", fileName, suffix)
	err = os.Rename(fileName, targetLogName)
	if nil != err {
		l.Error("rename fail: %s", err.Error())
		return err
	}

	var newFd *os.File
	newFd, err = os.OpenFile(fileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if nil != err {
		l.Error("open fail: %s", err.Error())
		os.Rename(targetLogName, fileName) // rename back?
		return
	}

	oldFd := fd
	l.SetOutput(newFd)
	oldFd.Close()

	// compress and clean async
	go func() {
		if l.rotateCfg.Compress {
			l.compress(targetLogName)
		}
		l.cleanOldLogs(fileName)
	}()
	return nil
}

func (l *Logger) log(level Level, format string, v ...interface{}) {
	if level < l.Level {
		return
	}
	l.Output(3, fmt.Sprint(level.String(), fmt.Sprintf(format, v...)))
}

func (l *Logger) Log(level Level, format string, v ...interface{}) {
	l.log(level, format, v...)
}

func (l *Logger) Printf(format string, v ...interface{}) {
	l.log(LevelInfo, format, v...)
}

// leveled log function for easy use.
func (l *Logger) Debug(format string, v ...interface{}) {
	l.log(LevelDebug, format, v...)
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.log(LevelInfo, format, v...)
}

func (l *Logger) Notice(format string, v ...interface{}) {
	l.log(LevelNotice, format, v...)
}

func (l *Logger) Warning(format string, v ...interface{}) {
	l.log(LevelWarning, format, v...)
}

func (l *Logger) Error(format string, v ...interface{}) {
	l.log(LevelError, format, v...)
}

func (l *Logger) Critical(format string, v ...interface{}) {
	l.log(LevelCritical, format, v...)
}

func (l *Logger) startRotateRoutine() {
	if l.rotateCfg == nil || l.rotateCfg.Rotate <= 0 || l.rotateCfg.Duration < 1*time.Second {
		return
	}
	for {
		time.Sleep(l.rotateCfg.Duration)
		l.Rotate()
	}
}

func (l *Logger) genSuffixStr() string {

	var t = time.Now().Truncate(l.rotateCfg.Duration)
	return t.Format(l.suffixFormat)
}

func (l *Logger) compress(path string) (err error) {
	var (
		rawfile *os.File
		wf      *os.File
		gzfile  *gzip.Writer
	)

	defer func() {
		if nil != rawfile {
			rawfile.Close()
		}
		if nil != gzfile {
			gzfile.Flush()
			gzfile.Close()
		}
		if nil != wf {
			wf.Close()
		}
		if err == nil {
			os.Remove(path)
		}
	}()

	rawfile, err = os.Open(path)
	if nil != err {
		l.Error("open file for compress err:%s", err.Error())
		return
	}

	gfn := fmt.Sprintf("%s.gz", path)
	wf, err = os.OpenFile(gfn, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if nil != err {
		l.Error("open gz file err:%s", err.Error())
		return
	}

	gzfile = gzip.NewWriter(wf)
	_, err = io.Copy(gzfile, rawfile)
	if nil != err {
		l.Error("write gz file:%s, err:%s", gfn, err.Error())
		return
	}
	return
}

func (l *Logger) isOverdue(ts string) (due bool) {
	wt, err := time.ParseInLocation(l.suffixFormat, ts, time.Local)
	if nil != err {
		l.Error("parse time err. time-str:%s, err:%s", ts, err.Error())
		return
	}

	now := time.Now()
	if now.Sub(wt) > l.rotateCfg.Duration*time.Duration(l.rotateCfg.Rotate) {
		return true
	}
	return false
}

func (l *Logger) cleanOldLogs(fileName string) (err error) {
	dir := filepath.Dir(fileName)
	d, err := os.Open(dir)
	if err != nil {
		l.Error("Failed to open directory. dir:%s, err:%s", err, err.Error())
		return
	}
	defer d.Close()

	var files []string
	files, err = d.Readdirnames(-1)
	if nil != err {
		l.Error("Failed to read directory. dir:%s, err:%s", err, err.Error())
		return
	}

	var (
		sl      = len(l.suffixFormat)
		pattern string
	)
	if l.rotateCfg.Compress {
		pattern = fmt.Sprintf("[0-9]{%d}\\.gz", sl)
	} else {
		pattern = fmt.Sprintf("[0-9]{%d}", sl)
	}

	var rx *regexp.Regexp
	rx, err = regexp.Compile(pattern)
	if nil != err {
		l.Error("Failed to compile pattern. pattern:%s, err:%s", pattern, err.Error())
		return
	}

	for _, fn := range files {
		fs := rx.FindString(fn)
		if "" == fs {
			continue
		}
		if l.rotateCfg.Compress {
			fs = strings.Replace(fs, ".gz", "", 1)
		}
		if l.isOverdue(fs) {
			os.Remove(filepath.Join(dir, fn))
		}
	}
	return
}
