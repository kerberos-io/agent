package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"

	"github.com/kerberos-io/agent/machinery/src/log"
)

const letterBytes = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func RandStringBytesMaskImpr(n int) string {
	b := make([]byte, n)
	// A rand.Int63() generates 63 random bits, enough for letterIdxMax letters!
	for i, cache, remain := n-1, rand.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = rand.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func CountDigits(i int64) (count int) {
	for i != 0 {
		i /= 10
		count = count + 1
	}
	return count
}

func ReadDirectory(directory string) ([]os.FileInfo, error) {
	ff, err := ioutil.ReadDir(directory)
	if err != nil {
		log.Log.Error(err.Error())
		return []os.FileInfo{}, nil
	}
	return ff, err
}

func NumberOfFilesInDirectory(path string) int {
	files, _ := ioutil.ReadDir(path)
	return len(files)
}

func RandStringBytesRmndr(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
	}
	return string(b)
}

func CreateFragmentedMP4(fullName string, fragmentedDuration int64) {
	path, _ := os.Getwd()
	duration := fragmentedDuration * 1000
	cmd := exec.Command("mp4fragment", "--fragment-duration", strconv.FormatInt(duration, 10), fullName, fullName+"f.mp4")
	cmd.Dir = path
	log.Log.Info(cmd.String())
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		log.Log.Error(fmt.Sprint(err) + ": " + stderr.String())
	} else {
		log.Log.Info("Created Fragmented: " + out.String())
	}

	// We will swap the files.
	os.Remove(fullName)
	os.Rename(fullName+"f.mp4", fullName)
}
