package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
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

// check if process is running by pid
func IsProcessRunning(pid int32) (*os.Process, bool) {
	process, err := os.FindProcess(int(pid))
	if err != nil {
		return nil, false // Process lookup failed
	}

	// On Windows, os.FindProcess always returns a process object and nil error,
	// even if the PID doesn't exist. Sending a signal is not reliable.
	// A common way to check is to try to send a 0 signal on Unix.
	// For Windows, this check is less definitive without using Windows-specific APIs.
	// If Signal(syscall.Signal(0)) fails, it means the process is not running or not owned by us.
	if runtime.GOOS != "windows" {
		err = process.Signal(syscall.Signal(0))
		if err != nil {
			return nil, false
		}
	} else {
		// On Windows, a successful os.FindProcess means the PID *was* valid at some point.
		// To check if it's *still* running, more complex checks are needed (e.g., via tasklist or OpenProcess).
		// For now, we'll consider it "found" but this isn't a guarantee it's actively running without error.
		// A more robust check might involve trying to query process information.
		// However, to avoid cgo or platform-specific libraries, we'll rely on os.FindProcess.
		// If the process truly doesn't exist, subsequent operations on it would likely fail.
	}
	return process, true
}

// get pm2-go main directory
func GetMainDirectory() string {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting user home directory:", err)
		os.Exit(1)
	}
	// add pm2-go directory
	dirname := filepath.Join(userHomeDir, ".pm2-go")
	// if dirname doesnt exist create it
	if _, err := os.Stat(dirname); os.IsNotExist(err) {
		// os.ModePerm (0777) is often used for user-specific config dirs,
		// but be mindful of security if multiple users share the system.
		// 0755 is also a good default.
		os.Mkdir(dirname, 0755)
		os.Mkdir(filepath.Join(dirname, "pids"), 0755)
		os.Mkdir(filepath.Join(dirname, "logs"), 0755)
	}
	// return dirname
	return dirname
}

// read pid file
func ReadPidFile(pidFileName string) (int32, error) {
	// read daemon.pid using go
	filePath := filepath.Join(GetMainDirectory(), "pids", pidFileName) // Assume pid files are in pids subdir
	// Ensure the pidFileName itself is just a name, not a path.
	// If pidFileName could be like "daemon.pid" or "pids/daemon.pid", this needs adjustment.
	// Based on app/daemon.go, it's just "daemon.pid".
	// However, shared/spawn.go stores PIDs like "pids/name.pid" relative to main dir.
	// Let's assume pidFileName is the base name and construct path accordingly.
	if filepath.Base(pidFileName) == pidFileName { // It's a base name
		filePath = filepath.Join(GetMainDirectory(), "pids", pidFileName)
	} else { // It's already a relative path from main dir (e.g. "pids/myprocess.pid")
		filePath = filepath.Join(GetMainDirectory(), pidFileName)
	}

	fileIO, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
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
	return int32(val), nil
}

// write pid to a file
func WritePidToFile(pidFilePath string, pid int) error {
	var fileIO *os.File
	var err error
	// Ensure the directory exists, as pidFilePath might be like "pids/name.pid"
	dir := filepath.Dir(pidFilePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// This check might be redundant if GetMainDirectory already creates "pids"
		// but good for robustness if WritePidToFile is called with arbitrary paths.
		// For now, assume pidFilePath is relative to GetMainDirectory() or absolute.
		// The paths from spawn.go are like GetMainDirectory()/pids/name.pid
	}

	fileIO, err = os.OpenFile(pidFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
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

// tail a file
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

// get last modified time of a file
func GetLastModified(filename string) time.Time {
	info, err := os.Stat(filename)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// given path, get first n lines of file
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

// exit pid
func ExitPid(pid int32, timeout time.Duration) {
	var exitState bool
	var process *os.Process
	var ok bool

	interval := 50 * time.Millisecond

	for ; timeout > 0; timeout -= interval {
		process, ok = IsProcessRunning(pid)
		if !ok {
			exitState = true
			break
		}
		time.Sleep(interval)
	}

	if !exitState {
		process.Kill()
	}
}

// remove contents of a file
func RemoveFileContents(filename string) error {
	var file *os.File
	var err error
	file, err = os.OpenFile(filename, os.O_RDWR, 0644) // Changed permissions from 0755 to 0644 for a file
	if err != nil {
		return err
	}
	defer file.Close()
	file.Truncate(0)
	return nil
}

// check if port is open
func IsPortOpen(port int) bool {
	// For Windows, "localhost" might resolve to IPv6 "::1" first,
	// which might not be what's listened on if the server binds to "0.0.0.0" or "127.0.0.1".
	// Explicitly checking 127.0.0.1 is often more reliable.
	address := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", address, 1*time.Second) // Added timeout
	if err != nil {
		// Try localhost as a fallback, though less common to be different for TCP.
		address = fmt.Sprintf("localhost:%d", port)
		conn, err = net.DialTimeout("tcp", address, 1*time.Second)
		if err != nil {
			return false
		}
	}
	defer conn.Close()
	return true
}

// get dump file path
func GetDumpFilePath(filename string) string {
	return filepath.Join(GetMainDirectory(), filename) // Uses GetMainDirectory now
}

// dump the current processses to a file
func SaveObject(filename string, object interface{}) error {
	var file *os.File
	var err error
	file, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644) // Changed 0640 to 0644
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	// The previous SaveObject function had an error where it was trying to use 'conn'
	// which is not in its scope, and also returning 'false' or 'true' instead of 'error'.
	// Correcting that:
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(object)
	if err != nil {
		return err
	}
	return nil
}

// load a file into a object
func LoadObject(filename string, object interface{}) error {
	var file *os.File
	var err error
	file, err = os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(object)
	if err != nil {
		return err
	}
	return nil
}

// get process
func GetProcess(pid int32) (*os.Process, bool) {
	return IsProcessRunning(pid)
}

// check if process is child process
func IsChildProcess(process *os.Process) bool {
	return process.Pid == os.Getpid()
}

// rename a file
func RenameFile(oldname string, newname string) error {
	return os.Rename(oldname, newname)
}

func FileSize(filename string) int64 {
	info, err := os.Stat(filename)
	if err != nil {
		return 0
	}
	return info.Size()
}
