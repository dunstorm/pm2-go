package utils

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// new logger using zerolog
func NewLogger() *zerolog.Logger {
	logger := log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	return &logger
}

// implement isProcessRunning
func IsProcessRunning(pid int) (*os.Process, bool) {
	// check if process is running by pid
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, false
	}
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return nil, false
	}
	return process, true
}

func ReadPidFile(pidFileName string) (int, error) {
	// read daemon.pid using go
	fileIO, err := os.OpenFile(path.Join(GetMainDirectory(), pidFileName), os.O_RDONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer fileIO.Close()
	rawBytes, err := ioutil.ReadAll(fileIO)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(rawBytes), "\n")
	val, err := strconv.Atoi(lines[0])
	if err != nil {
		return 0, err
	}
	return val, nil
}

func GetMainDirectory() string {
	dirname, err := os.UserHomeDir()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// add pm2-go directory
	dirname = dirname + "/.pm2-go"
	// if dirname doesnt exist create it
	if _, err := os.Stat(dirname); os.IsNotExist(err) {
		os.Mkdir(dirname, 0755)
		os.Mkdir(dirname+"/pids", 0755)
		os.Mkdir(dirname+"/logs", 0755)
	}
	// return dirname
	return dirname
}

func WritePidToFile(pidFilePath string, pid int) error {
	var fileIO *os.File
	var err error
	fileIO, err = os.OpenFile(pidFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)

	// write pid file
	if err != nil {
		return err
	}
	defer fileIO.Close()
	_, err = fileIO.WriteString(strconv.Itoa(pid))
	if err != nil {
		return err
	}
	return nil
}

func StringContains(s string, substr string) bool {
	return strings.Contains(s, substr)
}

func CopyFile(src string, dst string) error {
	// copy file to /tmp
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}
	// Change permissions Linux.
	err = os.Chmod(dst, 0777)
	if err != nil {
		return err
	}
	return nil
}

func Tail(logPrefix string, prefixColor func(a ...interface{}) string, filename string, out io.Writer) {
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	// skip to end of file
	f.Seek(0, 2)
	defer f.Close()
	r := bufio.NewReader(f)
	info, err := f.Stat()
	if err != nil {
		panic(err)
	}
	oldSize := info.Size()
	for {
		for line, prefix, err := r.ReadLine(); err != io.EOF; line, prefix, err = r.ReadLine() {
			if prefix {
				fmt.Fprint(out, prefixColor(logPrefix), string(line))
			} else {
				fmt.Fprintln(out, prefixColor(logPrefix), string(line))
			}
		}
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			panic(err)
		}
		for {
			time.Sleep(200 * time.Millisecond)
			newinfo, err := f.Stat()
			if err != nil {
				panic(err)
			}
			newSize := newinfo.Size()
			if newSize != oldSize {
				if newSize < oldSize {
					f.Seek(0, 0)
				} else {
					f.Seek(pos, io.SeekStart)
				}
				r = bufio.NewReader(f)
				oldSize = newSize
				break
			}
		}
	}
}

func GetLastModified(filename string) time.Time {
	info, err := os.Stat(filename)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func GetLogs(filename string, n int) ([]string, error) {
	var lines []string
	file, err := os.Open(filename)
	if err != nil {
		return lines, err
	}
	file.Seek(-1000, 2)
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return lines, err
	}
	if len(lines) < n {
		return lines, nil
	}
	return lines[len(lines)-n:], nil
}

// print logs from a array of string
func PrintLogs(logs []string, logPrefix string, color func(a ...interface{}) string) {
	for _, line := range logs {
		fmt.Println(color(logPrefix), line)
	}
}
