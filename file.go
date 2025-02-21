package logging

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// FileBackend implements LoggerInterface.
// It writes messages by lines limit, file size limit, or time frequency.
type FileBackend struct {
	sync.Mutex // write log order by order and  atomic incr maxLinesCurLines and maxSizeCurSize
	statusLock sync.RWMutex
	status     int8 // 0:close 1:run
	// The opened file
	Filename   string `json:"filename"`
	fileWriter *os.File

	// Rotate at line
	MaxLines         int `json:"maxlines"`
	maxLinesCurLines int

	// Rotate at size
	MaxSize        int `json:"maxsize"`
	maxSizeCurSize int

	// Rotate daily
	Daily         bool  `json:"daily"`
	MaxDays       int64 `json:"maxdays"`
	dailyOpenDate int

	Rotate bool `json:"rotate"`

	Perm os.FileMode `json:"perm"`

	fileNameOnly, suffix string // like "project.log", project is fileNameOnly and .log is suffix
	// Asynchronous output channels
	asyncMsgChan    chan []byte
	asyncSignalChan chan struct{}
}

// NewDefaultFileBackend create a FileLogWriter returning as LoggerInterface.
func NewDefaultFileBackend(filename string, asyncLen ...int) (*FileBackend, error) {
	if len(filename) == 0 {
		return nil, errors.New("FileBackend must have filename")
	}

	w := &FileBackend{
		Filename: filename,
		MaxLines: 1000000,
		MaxSize:  1 << 28, //256 MB
		Daily:    true,
		MaxDays:  7,
		Rotate:   true,
		Perm:     0660,
	}
	if len(asyncLen) > 0 && asyncLen[0] > 0 {
		w.asyncMsgChan = make(chan []byte, asyncLen[0])
		w.asyncSignalChan = make(chan struct{})
	}

	w.suffix = filepath.Ext(w.Filename)
	w.fileNameOnly = strings.TrimSuffix(w.Filename, w.suffix)
	if w.suffix == "" {
		w.suffix = ".log"
	}
	p, _ := filepath.Split(w.Filename)
	d, err := os.Stat(p)
	if err != nil || !d.IsDir() {
		os.MkdirAll(p, 0777)
	}
	err = w.startLogger()
	return w, err
}

// start file logger. create log file and set to locker-inside file writer.
func (w *FileBackend) startLogger() error {
	file, err := w.createLogFile()
	if err != nil {
		return err
	}
	if w.fileWriter != nil {
		w.fileWriter.Close()
	}
	w.fileWriter = file
	err = w.initFd()
	if err == nil {
		w.status = 1
		if w.asyncMsgChan != nil {
			go func() {
				for {
					select {
					case msg := <-w.asyncMsgChan:
						w.write(msg)
					case <-w.asyncSignalChan:
						return
					}
				}
			}()
		}
	}
	return err
}

func (w *FileBackend) needRotate(size int, day int) bool {
	return (w.MaxLines > 0 && w.maxLinesCurLines >= w.MaxLines) ||
		(w.MaxSize > 0 && w.maxSizeCurSize >= w.MaxSize) ||
		(w.Daily && day != w.dailyOpenDate)

}

var colorRegexp = regexp.MustCompile("\x1b\\[[0-9]{1,2}m")

// Log implements the Backend interface.
func (w *FileBackend) Log(calldepth int, rec *Record) {
	w.statusLock.RLock()
	if w.status == 0 {
		w.statusLock.RUnlock()
		return
	}
	msg := colorRegexp.ReplaceAll([]byte(rec.Formatted(calldepth+1, false)), []byte{})
	if msg[len(msg)-1] != '\n' {
		msg = append(msg, '\n')
	}
	d := rec.Time.Day()
	if w.Rotate {
		if w.needRotate(len(msg), d) {
			w.Lock()
			if w.needRotate(len(msg), d) {
				if err := w.doRotate(rec.Time); err != nil {
					fmt.Fprintf(os.Stderr, "FileLogWriter(%q): %s\n", w.Filename, err)
				}
			}
			w.Unlock()
		}
	}
	if w.asyncMsgChan != nil {
		w.asyncMsgChan <- msg
	} else {
		w.write(msg)
	}
	w.statusLock.RUnlock()
}

// Close close the file description, close file writer.
// Flush waits until all records in the buffered channel have been processed,
// and flushs file logger.
// there are no buffering messages in file logger in memory.
// flush file means sync file from disk.
func (w *FileBackend) Close() {
	w.statusLock.Lock()
	if w.status == 0 {
		w.statusLock.Unlock()
		return
	}
	w.status = 0
	w.statusLock.Unlock()
	if w.asyncSignalChan != nil {
		w.asyncSignalChan <- struct{}{}
		close(w.asyncSignalChan)
		close(w.asyncMsgChan)
		for msg := range w.asyncMsgChan {
			w.write(msg)
		}
	}
	w.fileWriter.Sync()
	w.fileWriter.Close()
}

func (w *FileBackend) write(msg []byte) {
	w.Lock()
	_, err := w.fileWriter.Write(msg)
	if err == nil {
		w.maxLinesCurLines++
		w.maxSizeCurSize += len(msg)
	}
	w.Unlock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to File Log msg:%s [error]%s\n", msg, err.Error())
	}
}

func (w *FileBackend) createLogFile() (*os.File, error) {
	// Open the log file
	fd, err := os.OpenFile(w.Filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, w.Perm)
	return fd, err
}

func (w *FileBackend) initFd() error {
	fd := w.fileWriter
	fInfo, err := fd.Stat()
	if err != nil {
		return fmt.Errorf("get stat err: %s\n", err)
	}
	w.maxSizeCurSize = int(fInfo.Size())
	w.dailyOpenDate = time.Now().Day()
	w.maxLinesCurLines = 0
	if fInfo.Size() > 0 {
		count, err := w.lines()
		if err != nil {
			return err
		}
		w.maxLinesCurLines = count
	}
	return nil
}

func (w *FileBackend) lines() (int, error) {
	fd, err := os.Open(w.Filename)
	if err != nil {
		return 0, err
	}
	defer fd.Close()

	buf := make([]byte, 32768) // 32k
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := fd.Read(buf)
		if err != nil && err != io.EOF {
			return count, err
		}

		count += bytes.Count(buf[:c], lineSep)

		if err == io.EOF {
			break
		}
	}

	return count, nil
}

const maxFileIndex = 999

// DoRotate means it need to write file in new file.
// new file name like xx.2013-01-01.log (daily) or xx.001.log (by line or size)
func (w *FileBackend) doRotate(logTime time.Time) error {
	_, err := os.Lstat(w.Filename)
	if err != nil {
		return err
	}
	// file exists
	// Find the next available number
	num := 1
	fName := ""
	modTime := logTime
	if w.Daily && logTime.Day() != w.dailyOpenDate {
		info, err := os.Lstat(w.Filename)
		if err != nil {
			return fmt.Errorf("Rotate: Cannot find free log number to rename %s\n", w.Filename)
		}
		modTime = info.ModTime()
	}

	for ; err == nil && num <= maxFileIndex; num++ {
		fName = w.fileNameOnly + fmt.Sprintf(".%s.%03d%s", modTime.Format("2006-01-02"), num, w.suffix)
		_, err = os.Lstat(fName)
	}

	// return error if the last file checked still existed
	if err == nil {
		return fmt.Errorf("Rotate: Cannot find free log number to rename %s\n", w.Filename)
	}

	// close fileWriter before rename
	w.fileWriter.Close()

	// Rename the file to its new found name
	// even if occurs error,we MUST guarantee to  restart new logger
	renameErr := os.Rename(w.Filename, fName)
	// re-start logger
	startLoggerErr := w.startLogger()
	go w.deleteOldLog()

	if startLoggerErr != nil {
		return fmt.Errorf("Rotate StartLogger: %s\n", startLoggerErr)
	}
	if renameErr != nil {
		return fmt.Errorf("Rotate: %s\n", renameErr)
	}
	return nil

}

func (w *FileBackend) deleteOldLog() {
	dir := filepath.Dir(w.Filename)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) (returnErr error) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "Unable to delete old log '%s', error: %v\n", path, r)
			}
		}()

		if !info.IsDir() && info.ModTime().Unix() < (time.Now().Unix()-60*60*24*w.MaxDays) {
			if strings.HasPrefix(filepath.Base(path), w.fileNameOnly) &&
				strings.HasSuffix(filepath.Base(path), w.suffix) {
				os.Remove(path)
			}
		}
		return
	})
}
