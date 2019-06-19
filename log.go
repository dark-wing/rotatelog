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

	"sync/atomic"
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
	formatDay   = "20060102"
	formatHour  = "2006010215"
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

type LogParam struct {
	Rotate   int           //
	Duration time.Duration //
	Compress bool          //
}

// String returns the string representation of the log level
func (l Level) String() string {
	if name, ok := levelTags[l]; ok {
		return name
	}
	return "[Unknown] "
}
func (l *Logger) doRotate() {
	//beginTime := time.Now()
	if l.rotateWork {
		return
	}
	l.rotateWork = true
	for {
		time.Sleep(l.param.Duration)
		l.rotate()
	}
}
func NewLevel(name string) Level {
	if l, ok := levelNames[name]; ok {
		return l
	}
	return LevelError
}

type Logger struct {
	*log.Logger
	Level        Level
	param        *LogParam
	fileName     string
	lastSuffix   string
	fd           *os.File
	suffixFormat string
	//mx           sync.Mutex
	compressing int32
	rotateWork  bool
}

// @see log.New
func New(out io.Writer, prefix string, flag int, level Level) *Logger {
	return &Logger{
		Logger: log.New(out, prefix, flag),
		Level:  level,
		param:  &LogParam{0, 0, false},
	}
}
func (l *Logger) genSuffixStr() string {
	if "" == l.suffixFormat {
		return ""
	}
	return time.Now().Format(l.suffixFormat)
}
func (l *Logger) compress(path string) {
	if !atomic.CompareAndSwapInt32(&l.compressing, 0, 1) {
		return
	}
	defer atomic.CompareAndSwapInt32(&l.compressing, 1, 0)
	var (
		rawfile *os.File
		wf      *os.File
		gzfile  *gzip.Writer
		suc     bool
		err     error
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
		if suc {
			os.Remove(path)
		}
	}()
	rawfile, err = os.Open(path)
	if nil != err {
		l.Error("Failed to open raw file. file:%s, err:%s", path, err.Error())
		return
	}

	gfn := fmt.Sprintf("%s.gz", path)
	wf, err = os.OpenFile(gfn, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
	if nil != err {
		l.Error("Failed to create gz file. file:%s, err:%s", gfn, err.Error())
		return
	}
	gzfile = gzip.NewWriter(wf)
	_, err = io.Copy(gzfile, rawfile)
	if nil != err {
		l.Error("Failed to write gz file. file:%s, err:%s", gfn, err.Error())
		return
	}
	suc = true

	return

}
func (l *Logger) isOverdue(ts string) bool {
	tLocal := time.Local
	wt, err := time.ParseInLocation(l.suffixFormat, ts, tLocal)
	if nil != err {
		l.Error("Failed to parse time. ts:%s, err:%s", ts, err.Error())
	}

	now := time.Now()

	if now.Sub(wt) > l.param.Duration*time.Duration(l.param.Rotate) {
		return true
	}
	return false
}
func (l *Logger) deleteOverdue() {
	if "" == l.fileName || "" == l.suffixFormat {
		return
	}
	dir := filepath.Dir(l.fileName)
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
	sl := len(l.suffixFormat)
	pa := fmt.Sprintf("[0-9]{%d}\\.gz", sl)
	var rx *regexp.Regexp
	rx, err = regexp.Compile(pa)
	if nil != err {
		l.Error("Failed to compile pattern. pattern:%s, err:%s", pa, err.Error())
		return
	}
	for _, fn := range files {
		fs := rx.FindString(fn)
		if "" != fs {
			fs = strings.Replace(fs, ".gz", "", 1)
			if l.isOverdue(fs) {
				os.Remove(filepath.Join(dir, fn))
			}
		}
	}

}
func (l *Logger) rotate() error {
	if 0 == l.param.Duration || nil == l.fd || "" == l.fileName {
		return nil
	}
	suffix := l.genSuffixStr()
	if "" == l.lastSuffix {
		l.lastSuffix = suffix
		return nil
	}
	if l.lastSuffix == suffix {
		return nil
	}

	nf := fmt.Sprintf("%s.%s", l.fileName, suffix)
	err := os.Rename(l.fileName, nf)
	if nil != err {
		l.Error("Failed to rename. oldPath:%s, newPath:%s, err:%s", l.fileName, nf, err.Error())
		return err
	}
	var newFd *os.File
	newFd, err = os.OpenFile(l.fileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
	if nil != err {
		l.Error("Failed to open file. file:%s, err:%s", l.fileName, err.Error())
		err := os.Rename(nf, l.fileName)
		return err
	}
	oldFd := l.fd
	l.SetOutput(newFd)
	l.fd = newFd
	oldFd.Close()
	l.lastSuffix = suffix
	if l.param.Compress {
		go l.compress(nf)
	}
	go l.deleteOverdue()
	return nil

}
func (l *Logger) SetOutputByName(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		return err
	}

	l.SetOutput(f)
	l.fileName = path
	l.fd = f
	return nil
}
func (l *Logger) SetLogParam(param *LogParam) {
	l.param = param
	if l.param.Duration == 24*time.Hour {
		l.suffixFormat = formatDay
	} else if l.param.Duration == time.Hour {
		l.suffixFormat = formatHour
	} else if l.param.Duration == time.Minute {
		l.suffixFormat = formatMin
	} else if l.param.Duration == time.Second {
		l.suffixFormat = formatSec
	}

	if "" == l.lastSuffix {
		l.lastSuffix = l.genSuffixStr()
	}
	if l.param.Duration > 0 {
		go l.doRotate()
	}
}
func (l *Logger) SetLevel(level Level) {
	l.Level = level
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
	l.Info(format, v...)
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
