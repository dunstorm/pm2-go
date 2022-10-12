package utils

import (
	"strconv"
	"strings"
)

func ParseBool(str string) bool {
	return str == "true"
}

func ParseInt(str string) int {
	i, err := strconv.Atoi(str)
	if err != nil {
		panic(err)
	}
	return i
}

// 10K, 10M, 1G
func ParseSize(str string) int {
	var size int
	var err error
	if strings.HasSuffix(str, "K") {
		size, err = strconv.Atoi(strings.TrimSuffix(str, "K"))
		if err != nil {
			panic(err)
		}
		size = size * 1024
	} else if strings.HasSuffix(str, "M") {
		size, err = strconv.Atoi(strings.TrimSuffix(str, "M"))
		if err != nil {
			panic(err)
		}
		size = size * 1024 * 1024
	} else if strings.HasSuffix(str, "G") {
		size, err = strconv.Atoi(strings.TrimSuffix(str, "G"))
		if err != nil {
			panic(err)
		}
		size = size * 1024 * 1024 * 1024
	} else {
		size, err = strconv.Atoi(str)
		if err != nil {
			panic(err)
		}
	}
	return size
}
