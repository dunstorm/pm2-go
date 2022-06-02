package utils

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
)

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
		log.Fatal(err)
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
		log.Println(err)
	}
	return nil
}
