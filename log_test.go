package rotatelog

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"
)

func TestRotateLoggger(t *testing.T) {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	var ll = New(os.Stderr, "", log.Ldate|log.Ltime|log.Lshortfile, LevelInfo, nil)
	ll.Debug("test debug, should not see this")
	ll.Level = LevelDebug
	ll.Info("log Info, should see this")
}

func BenchmarkStdLogPrintf(b *testing.B) {
	l := log.New(ioutil.Discard, "prefix ", log.Ldate|log.Ltime)
	for i := 0; i < b.N; i++ {
		l.Printf("%s %s", "hello", "log")
	}
}
func BenchmarkrotatelogInfo(b *testing.B) {
	l := New(ioutil.Discard, "prefix ", log.Ldate|log.Ltime, LevelInfo, nil)
	for i := 0; i < b.N; i++ {
		l.Info("%s %s", "hello", "log")
	}
}

func TestRotate(t *testing.T) {
	os.Mkdir("logs", 0755)
	logFile := "logs/rotatelog.log"

	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		t.Errorf("open log file for test fail:%s", err.Error())
	}

	rotateConfig := &RotateConfig{Duration: time.Second, Rotate: 5, Compress: true, StartRoutine: true}
	logger := New(f, "", log.Ldate|log.Ltime|log.Lshortfile, LevelDebug, rotateConfig)
	logger.Notice("start")

	i := 0
	for i < 1000*100 {
		t.Logf("xx")
		time.Sleep(time.Microsecond)
		logger.Debug("debug %d", i)
		logger.Info("info %d", i)
		logger.Notice("notice %d", i)
		i++
	}
}
