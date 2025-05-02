package utils

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

const VERSION = "3.3.5"

const letterBytes = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// MaxUint8 - maximum value which can be held in an uint8
const MaxUint8 = ^uint8(0)

// MinUint8 - minimum value which can be held in an uint8
const MinUint8 = 0

// MaxUint16 - maximum value which can be held in an uint16
const MaxUint16 = ^uint16(0)

// MinUint16 - minimum value which can be held in an uint8
const MinUint16 = 0

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func PrintASCIIArt() {
	asciiArt := `	 _  __         _                          _       
	| |/ /___ _ __| |__   ___ _ __ ___  ___  (_) ___  
	| ' // _ \ '__| '_ \ / _ \ '__/ _ \/ __| | |/ _ \ 
	| . \  __/ |  | |_) |  __/ | | (_) \__ \_| | (_) |
	|_|\_\___|_|  |_.__/ \___|_|  \___/|___(_)_|\___/ 
													  
	`
	fmt.Println(asciiArt)
}

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

func CheckDataDirectoryPermissions(configDirectory string) error {
	recordingsDirectory := configDirectory + "/data/recordings"
	configurationDirectory := configDirectory + "/data/config"
	snapshotsDirectory := configDirectory + "/data/snapshots"
	cloudDirectory := configDirectory + "/data/cloud"

	err := CheckDirectoryPermissions(recordingsDirectory)
	if err == nil {
		err = CheckDirectoryPermissions(configurationDirectory)
		if err == nil {
			err = CheckDirectoryPermissions(snapshotsDirectory)
			if err == nil {
				err = CheckDirectoryPermissions(cloudDirectory)
			}
		}
	}

	if err != nil {
		log.Log.Error("Checking data directory permissions: " + err.Error())
		return err
	}

	log.Log.Info("Checking data directory permissions: OK")
	return nil
}

func CheckDirectoryPermissions(directory string) error {
	// Check if the directory exists
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return errors.New("Directory does not exist, " + directory)
	}

	// Try to create a file
	file := directory + "/.test"
	f, err := os.Create(file)
	if f != nil {
		defer f.Close()
	}

	// We will remove the file if it was created
	if err == nil {
		err := os.Remove(file)
		if err == nil {
			return nil
		} else {
			return errors.New("Problem deleting a file: " + err.Error())
		}
	}
	return errors.New("Problem creating a file: " + err.Error())
}

func ReadDirectory(directory string) ([]os.FileInfo, error) {
	ff, err := ioutil.ReadDir(directory)
	if err != nil {
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

// NumberOfMP4sInDirectory returns the count of all files with mp4 extension in current directory
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
	// This timescale is crucial, as it should be the same as the one defined in JOY4.
	cmd := exec.Command("mp4fragment", "--timescale", "10000000", "--fragment-duration", strconv.FormatInt(duration, 10), fullName, fullName+"f.mp4")
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

func PrintEnvironmentVariables() {
	// Print environment variables that include "AGENT_" as a prefix.
	environmentVariables := ""
	for _, e := range os.Environ() {
		if strings.Contains(e, "AGENT_") {
			pair := strings.Split(e, "=")
			environmentVariables = environmentVariables + pair[0] + "=" + pair[1] + " "
		}
	}
	log.Log.Info("Printing out environmentVariables (AGENT_...): " + environmentVariables)
}

func PrintConfiguration(configuration *models.Configuration) {
	// We will print out the struct.
	if configuration == nil {
		log.Log.Info("Configuration is nil")
		return
	}
	config := configuration.Config
	// Iterate over the struct and printout the values.
	v := reflect.ValueOf(config)
	typeOfS := v.Type()
	configurationVariables := ""
	for i := 0; i < v.NumField(); i++ {
		key := typeOfS.Field(i).Name
		value := v.Field(i).Interface()
		// Convert to string.
		configurationVariables = configurationVariables + key + ": " + fmt.Sprintf("%v", value) + " "
	}
	log.Log.Info("Printing our configuration (config.json): " + configurationVariables)
}

func Decrypt(directoryOrFile string, symmetricKey []byte) {
	// Check if file or directory
	fileInfo, err := os.Stat(directoryOrFile)
	if err != nil {
		log.Log.Fatal(err.Error())
		return
	}

	var files []string
	if fileInfo.IsDir() {
		// Create decrypted directory
		err = os.MkdirAll(directoryOrFile+"/decrypted", 0755)
		if err != nil {
			log.Log.Fatal(err.Error())
			return
		}
		dir, err := os.ReadDir(directoryOrFile)
		if err != nil {
			log.Log.Fatal(err.Error())
			return
		}
		for _, file := range dir {
			// Check if file is not a directory
			if !file.IsDir() {
				// Check if an mp4 file
				if strings.HasSuffix(file.Name(), ".mp4") {
					files = append(files, directoryOrFile+"/"+file.Name())
				}
			}
		}
	} else {
		files = append(files, directoryOrFile)
	}

	// We'll loop over all files and decrypt them one by one.
	for _, file := range files {

		// Read file
		content, err := os.ReadFile(file)
		if err != nil {
			log.Log.Fatal(err.Error())
			return
		}
		// Decrypt using AES key
		decrypted, err := encryption.AesDecrypt(content, string(symmetricKey))
		if err != nil {
			log.Log.Fatal("Something went wrong while decrypting: " + err.Error())
			return
		}

		// Write decrypted content to file with appended .decrypted
		// Get filename split by / and get last element.
		fileParts := strings.Split(file, "/")
		fileName := fileParts[len(fileParts)-1]
		pathToFile := strings.Join(fileParts[:len(fileParts)-1], "/")

		err = os.WriteFile(pathToFile+"/decrypted/"+fileName, []byte(decrypted), 0644)
		if err != nil {
			log.Log.Fatal(err.Error())
			return
		}
	}
}

func ImageToBytes(img image.Image) ([]byte, error) {
	buffer := new(bytes.Buffer)
	w := bufio.NewWriter(buffer)
	err := jpeg.Encode(w, img, &jpeg.Options{Quality: 15})
	return buffer.Bytes(), err
}

func RandomUint32() uint32 {
	// Generate a random uint32 value
	return uint32(rand.Int31())
}
