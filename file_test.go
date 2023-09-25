package logging

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFile1(t *testing.T) {
	log := NewLogger("TestFile1")
	fileBackend, err := NewDefaultFileBackend("test.log")
	if err != nil {
		panic(err)
	}
	var format = MustStringFormatter(
		`%{color}%{module} %{time:15:04:05.000} %{longfile} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	log.SetBackend(AddModuleLevel(NewBackendFormatter(fileBackend, format)))
	log.Debug("debug\n")
	log.Info("info\n")
	log.Notice("notice\n")
	log.Warning("warning\n")
	log.Error("error\n")
	log.Critical("critical\n")
	f, err := os.Open("test.log")
	if err != nil {
		t.Fatal(err)
	}
	b := bufio.NewReader(f)
	lineNum := 0
	for {
		line, _, err := b.ReadLine()
		if err != nil {
			break
		}
		if len(line) > 0 {
			lineNum++
		}
	}
	f.Close()
	fileBackend.Close()
	var expected = int(DEBUG) + 1
	if lineNum != expected {
		t.Fatal(lineNum, "not "+strconv.Itoa(expected)+" lines")
	}
	os.Remove("test.log")
}

func TestFile2(t *testing.T) {
	log := NewLogger("TestFile2")
	fileBackend, err := NewDefaultFileBackend("test2.log", 1000)
	if err != nil {
		panic(err)
	}
	var format = MustStringFormatter(
		`%{color}%{module} %{time:15:04:05.000} %{longfile} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	log.SetBackend(AddModuleLevel(NewBackendFormatter(fileBackend, format)))
	log.Debug("debug\n")
	log.Info("info\n")
	log.Notice("notice\n")
	log.Warning("warning\n")
	log.Error("error\n")
	log.Critical("critical\n")
	log.Close()
	f, err := os.Open("test2.log")
	if err != nil {
		t.Fatal(err)
	}
	b := bufio.NewReader(f)
	lineNum := 0
	for {
		line, _, err := b.ReadLine()
		if err != nil {
			break
		}
		if len(line) > 0 {
			lineNum++
		}
	}
	f.Close()
	var expected = int(DEBUG) + 1
	if lineNum != expected {
		t.Fatal(lineNum, "not "+strconv.Itoa(expected)+" lines")
	}
	os.Remove("test2.log")
}

func TestFileRotate(t *testing.T) {
	log := NewLogger("TestFileRotate")
	fileBackend, err := NewDefaultFileBackend("test3.log")
	if err != nil {
		panic(err)
	}
	fileBackend.MaxLines = 4
	log.SetBackend(AddModuleLevel(fileBackend))
	log.Debug("debug")
	log.Info("info")
	log.Notice("notice")
	log.Warn("warning")
	log.Error("error")
	log.Critical("critical")
	rotateName := "test3" + fmt.Sprintf(".%s.%03d", time.Now().Format("2006-01-02"), 1) + ".log"
	b, err := exists(rotateName)
	if !b || err != nil {
		os.Remove("test3.log")
		t.Fatal("rotate not generated")
	}
	fileBackend.Close()
	os.Remove(rotateName)
	os.Remove("test3.log")
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func BenchmarkFile(b *testing.B) {
	log := NewLogger("BenchmarkFile")
	fileBackend, err := NewDefaultFileBackend("test4.log")
	if err != nil {
		panic(err)
	}
	log.SetBackend(AddModuleLevel(fileBackend))
	for i := 0; i < b.N; i++ {
		log.Debug("debug")
	}
	fileBackend.Close()
	os.Remove("test4.log")
}

func BenchmarkFileAsynchronous(b *testing.B) {
	log := NewLogger("BenchmarkFileAsynchronous")
	fileBackend, err := NewDefaultFileBackend("test4.log")
	if err != nil {
		panic(err)
	}
	log.SetBackend(AddModuleLevel(fileBackend))
	for i := 0; i < b.N; i++ {
		log.Debug("debug")
	}
	fileBackend.Close()
	os.Remove("test4.log")
}

func BenchmarkFileCallDepth(b *testing.B) {
	log := NewLogger("BenchmarkFileCallDepth")
	fileBackend, err := NewDefaultFileBackend("test4.log")
	if err != nil {
		panic(err)
	}
	log.SetBackend(AddModuleLevel(fileBackend))
	log.ExtraCalldepth = 2
	for i := 0; i < b.N; i++ {
		log.Debug("debug")
	}
	fileBackend.Close()
	os.Remove("test4.log")
}

func BenchmarkFileAsynchronousCallDepth(b *testing.B) {
	log := NewLogger("BenchmarkFileAsynchronousCallDepth")
	fileBackend, err := NewDefaultFileBackend("test4.log")
	if err != nil {
		panic(err)
	}
	log.SetBackend(AddModuleLevel(fileBackend))
	log.ExtraCalldepth = 2
	for i := 0; i < b.N; i++ {
		log.Debug("debug")
	}
	fileBackend.Close()
	os.Remove("test4.log")
}

func BenchmarkFileOnGoroutine(b *testing.B) {
	log := NewLogger("BenchmarkFileOnGoroutine")
	fileBackend, err := NewDefaultFileBackend("test4.log")
	if err != nil {
		panic(err)
	}
	log.SetBackend(AddModuleLevel(fileBackend))
	for i := 0; i < b.N; i++ {
		go log.Debug("debug")
	}
	fileBackend.Close()
	os.Remove("test4.log")
}

func TestFileBackend_doRotate(t *testing.T) {
	assert := assert.New(t)
	file, err := os.OpenFile("test.log", os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		assert.FailNow(err.Error())
	}
	file.WriteString("hello")

	type fields struct {
		status           int8
		Filename         string
		fileWriter       *os.File
		MaxLines         int
		maxLinesCurLines int
		MaxSize          int
		maxSizeCurSize   int
		Daily            bool
		MaxDays          int64
		dailyOpenDate    int
		Rotate           bool
		Perm             os.FileMode
		fileNameOnly     string
		suffix           string
		asyncMsgChan     chan []byte
		asyncSignalChan  chan struct{}
	}
	type args struct {
		logTime time.Time
	}
	tests := []struct {
		name    string
		fields  fields
		newName string
		args    args
		err     string
	}{
		{
			name: "max size > 0 and max lines > 0",
			fields: fields{
				status:           0,
				Filename:         "test.log",
				fileWriter:       file,
				MaxLines:         100000,
				maxLinesCurLines: 0,
				MaxSize:          1 << 28,
				maxSizeCurSize:   0,
				Daily:            false,
				MaxDays:          0,
				dailyOpenDate:    0,
				Rotate:           true,
				Perm:             0660,
				fileNameOnly:     "test",
				suffix:           ".log",
				asyncMsgChan:     make(chan []byte),
				asyncSignalChan:  make(chan struct{}),
			},
			newName: "test." + time.Now().Format("2006-01-02") + ".001.log",
			args: args{
				logTime: time.Now(),
			},
			err: "",
		},
		{
			name: "max size > 0 and max lines == 0",
			fields: fields{
				status:           0,
				Filename:         "test.log",
				fileWriter:       file,
				MaxLines:         0,
				maxLinesCurLines: 0,
				MaxSize:          1 << 28,
				maxSizeCurSize:   0,
				Daily:            false,
				MaxDays:          0,
				dailyOpenDate:    0,
				Rotate:           true,
				Perm:             0660,
				fileNameOnly:     "test",
				suffix:           ".log",
				asyncMsgChan:     make(chan []byte),
				asyncSignalChan:  make(chan struct{}),
			},
			newName: "test." + time.Now().Format("2006-01-02") + ".002.log",
			args: args{
				logTime: time.Now(),
			},
			err: "",
		},
		{
			name: "max size == 0 and max lines > 0",
			fields: fields{
				status:           0,
				Filename:         "test.log",
				fileWriter:       file,
				MaxLines:         10000,
				maxLinesCurLines: 0,
				MaxSize:          0,
				maxSizeCurSize:   0,
				Daily:            false,
				MaxDays:          0,
				dailyOpenDate:    0,
				Rotate:           true,
				Perm:             0660,
				fileNameOnly:     "test",
				suffix:           ".log",
				asyncMsgChan:     make(chan []byte),
				asyncSignalChan:  make(chan struct{}),
			},
			newName: "test." + time.Now().Format("2006-01-02") + ".003.log",
			args: args{
				logTime: time.Now(),
			},
			err: "",
		},
		{
			name: "max size == 0 and max lines == 0 and daily == true",
			fields: fields{
				status:           0,
				Filename:         "test.log",
				fileWriter:       file,
				MaxLines:         10000,
				maxLinesCurLines: 0,
				MaxSize:          0,
				maxSizeCurSize:   0,
				Daily:            true,
				MaxDays:          0,
				dailyOpenDate:    0,
				Rotate:           true,
				Perm:             0660,
				fileNameOnly:     "test",
				suffix:           ".log",
				asyncMsgChan:     make(chan []byte),
				asyncSignalChan:  make(chan struct{}),
			},
			newName: "test." + time.Now().Format("2006-01-02") + ".003.log",
			args: args{
				logTime: time.Now().Add(24 * time.Hour),
			},
			err: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &FileBackend{
				status:           tt.fields.status,
				Filename:         tt.fields.Filename,
				fileWriter:       tt.fields.fileWriter,
				MaxLines:         tt.fields.MaxLines,
				maxLinesCurLines: tt.fields.maxLinesCurLines,
				MaxSize:          tt.fields.MaxSize,
				maxSizeCurSize:   tt.fields.maxSizeCurSize,
				Daily:            tt.fields.Daily,
				MaxDays:          tt.fields.MaxDays,
				dailyOpenDate:    tt.fields.dailyOpenDate,
				Rotate:           tt.fields.Rotate,
				Perm:             tt.fields.Perm,
				fileNameOnly:     tt.fields.fileNameOnly,
				suffix:           tt.fields.suffix,
				asyncMsgChan:     tt.fields.asyncMsgChan,
				asyncSignalChan:  tt.fields.asyncSignalChan,
			}
			if err := w.doRotate(tt.args.logTime); err != nil {
				if tt.err == "" {
					assert.Equal(err.Error(), "", "test: ["+tt.name+"] return not nil but want nil")
				} else {
					assert.Contains(err.Error(), tt.err, "test: ["+tt.name+"] error message not eq")
				}
			} else {
				_, err := os.Stat(tt.newName)
				if os.IsNotExist(err) {
					assert.Failf("targe file not exist", "test: [%s] file: %s not exist", tt.name, tt.newName)
				}
				assert.Equal("", tt.err, "test: ["+tt.name+"] return nil but want not nil")
			}
		})
	}

	removeTestFiles("test")
}

func removeTestFiles(prefix string) {
	matchingFiles, err := filepath.Glob(prefix + ".*")
	if err != nil {
		fmt.Println("Error finding files:", err)
		return
	}

	for _, file := range matchingFiles {
		err := os.Remove(file)
		if err != nil {
			fmt.Printf("Error removing file %s: %s\n", file, err)
		} else {
			fmt.Printf("Successfully removed file %s\n", file)
		}
	}
}
