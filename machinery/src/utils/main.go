package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

const letterBytes = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func FindOldestFile(dir string) (oldestFile os.FileInfo, err error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return
	}

	oldestTime := time.Now()
	for _, file := range files {
		if file.Mode().IsRegular() && file.ModTime().Before(oldestTime) {
			oldestFile = file
			oldestTime = file.ModTime()
		}
	}

	if oldestFile == nil {
		err = os.ErrNotExist
	}
	return
}

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

func GetSortedDirectory(files []os.FileInfo) []os.FileInfo {
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().After(files[j].ModTime())
	})
	return files
}

func GetMediaFormatted(files []os.FileInfo, recordingDirectory string, configuration *models.Configuration, eventFilter models.EventFilter) []models.Media {
	filePaths := []models.Media{}
	count := 0
	for _, file := range files {
		fileName := file.Name()
		fileParts := strings.Split(fileName, "_")
		if len(fileParts) == 6 {
			timestamp := fileParts[0]
			timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
			if err == nil {

				// If we have an offset we will check if we should skip or not
				if eventFilter.TimestampOffsetEnd > 0 {
					// Medias are sorted from new to older. TimestampOffsetEnd holds the oldest
					// timestamp of the previous batch of events. By doing this check, we make sure
					// to skip the previous batch.
					if timestampInt >= eventFilter.TimestampOffsetEnd {
						continue
					}
				}

				loc, _ := time.LoadLocation(configuration.Config.Timezone)
				time := time.Unix(timestampInt, 0).In(loc)
				day := time.Format("02-01-2006")
				timeString := time.Format("15:04:05")
				shortDay := time.Format("Jan _2")

				media := models.Media{
					Key:        fileName,
					Path:       recordingDirectory + "/" + fileName,
					CameraName: configuration.Config.Name,
					CameraKey:  configuration.Config.Key,
					Day:        day,
					ShortDay:   shortDay,
					Time:       timeString,
					Timestamp:  timestamp,
				}
				filePaths = append(filePaths, media)
				count = count + 1
				if eventFilter.NumberOfElements > 0 && count >= eventFilter.NumberOfElements {
					break
				}
			}
		}
	}
	return filePaths
}

func GetDays(files []os.FileInfo, recordingDirectory string, configuration *models.Configuration) []string {
	days := []string{}
	for _, file := range files {
		fileName := file.Name()
		fileParts := strings.Split(fileName, "_")
		if len(fileParts) == 6 {
			timestamp := fileParts[0]
			timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
			if err == nil {
				loc, _ := time.LoadLocation(configuration.Config.Timezone)
				time := time.Unix(timestampInt, 0).In(loc)
				day := time.Format("02-01-2006")
				days = append(days, day)
			}
		}
	}
	uniqueDays := Unique(days)
	return uniqueDays
}

func Unique(intSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range intSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func NumberOfFilesInDirectory(path string) int {
	files, _ := ioutil.ReadDir(path)
	return len(files)
}

// Returns the count of all files with mp4 extension in current directory
func NumberOfMP4sInDirectory(path string) int {
	pattern := filepath.Join(path, "*.mp4")
	files, _ := filepath.Glob(pattern)
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
