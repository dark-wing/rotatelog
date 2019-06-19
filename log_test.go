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

	var ll = New(os.Stderr, "", log.Ldate|log.Ltime|log.Lshortfile, LevelInfo)
	ll.Debug("test debug, should not see this")
	ll.Level = LevelDebug
	ll.Info("log Info, should see this")
}

func BenchmarkStdLogPrintf(b *testing.B) {
	//var buf bytes.Buffer
	//l := log.New(&buf, "", log.Ldate|log.Ltime|log.Lshortfile)
	l := log.New(ioutil.Discard, "prefix ", log.Ldate|log.Ltime)
	for i := 0; i < b.N; i++ {
		//buf.Reset()
		l.Printf("%s %s", "hello", "log")
	}
}
func BenchmarkrotatelogInfo(b *testing.B) {
	//var buf bytes.Buffer
	//l := New(&buf, "", log.Ldate|log.Ltime|log.Lshortfile, LevelInfo)
	l := New(ioutil.Discard, "prefix ", log.Ldate|log.Ltime, LevelInfo)
	for i := 0; i < b.N; i++ {
		//buf.Reset()
		l.Info("%s %s", "hello", "log")
	}
}

func TestRotate(t *testing.T) {
	lg := New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile, LevelDebug)
	lg.SetLevel(NewLevel("debug"))
	logFile := "/home/dark/work/Test/hotspot/rotatelog.log"
	lg.SetLogParam(&LogParam{Duration: time.Second, Rotate: 5, Compress: true})
	lg.SetOutputByName(logFile)
	i := 0
	for true {
		time.Sleep(time.Microsecond)
		lg.Debug("%d", i)
		i++
	}

}
