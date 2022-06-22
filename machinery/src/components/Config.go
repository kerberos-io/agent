package components

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/InVisionApp/conjungo"
	"github.com/kerberos-io/agent/machinery/src/database"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"gopkg.in/mgo.v2/bson"
)

func GetSnapshot() string {
	var snapshot string
	files, err := ioutil.ReadDir("./data/snapshots")
	if err == nil && len(files) > 1 {
		sort.Slice(files, func(i, j int) bool {
			return files[i].ModTime().Before(files[j].ModTime())
		})
		f, _ := os.Open("./data/snapshots/" + files[1].Name())
		defer f.Close()
		// Read entire JPG into byte slice.
		reader := bufio.NewReader(f)
		content, _ := ioutil.ReadAll(reader)
		// Encode as base64.
		snapshot = base64.StdEncoding.EncodeToString(content)
	}
	return snapshot
}

// ReadUserConfig Reads the user configuration of the Kerberos Open Source instance.
// This will return a models.User struct including the username, password,
// selected language, and if the installation was completed or not.
func ReadUserConfig() (userConfig models.User) {
	for {
		jsonFile, err := os.Open("./data/config/user.json")
		if err != nil {
			fmt.Println(err)
			fmt.Println("Config file is not found " + "./data/config/user.json" + ", trying again in 5s.")
			time.Sleep(5 * time.Second)
		} else {
			fmt.Println("Successfully Opened user.json")
			byteValue, _ := ioutil.ReadAll(jsonFile)
			err = json.Unmarshal(byteValue, &userConfig)
			if err != nil {
				fmt.Println("JSON file not valid: " + err.Error())
			} else {
				jsonFile.Close()
				break
			}
			time.Sleep(5 * time.Second)
		}
		jsonFile.Close()
	}

	return
}

func OpenConfig(configuration *models.Configuration) {

	// We are checking which deployment this is running, so we can load
	// into the configuration as expected.

	if os.Getenv("DEPLOYMENT") == "" || os.Getenv("DEPLOYMENT") == "agent" {

		// Local deployment means we do a stand-alone installation
		// Configuration is stored into a json file, and there is only 1 agent.

		// Open device config
		for {
			jsonFile, err := os.Open("./data/config/config.json")
			if err != nil {
				log.Log.Error("Config file is not found " + "./data/config/config.json" + ", trying again in 5s.")
				time.Sleep(5 * time.Second)
			} else {
				log.Log.Info("Successfully Opened config.json from " + configuration.Name)
				byteValue, _ := ioutil.ReadAll(jsonFile)
				err = json.Unmarshal(byteValue, &configuration.Config)
				jsonFile.Close()
				if err != nil {
					fmt.Println("JSON file not valid: " + err.Error())
				} else {
					err = json.Unmarshal(byteValue, &configuration.CustomConfig)
					if err != nil {
						fmt.Println("JSON file not valid: " + err.Error())
					} else {
						break
					}
				}
				time.Sleep(5 * time.Second)
			}
			jsonFile.Close()
		}

	} else if os.Getenv("DEPLOYMENT") == "factory" || os.Getenv("MACHINERY_ENVIRONMENT") == "kubernetes" {

		// Factory deployment means that configuration is stored in MongoDB
		// Multiple agents have there configuration stored, and can benefit from
		// the concept of a global concept.

		session := database.New().Copy()
		defer session.Close()
		db := session.DB(database.DatabaseName)
		collection := db.C("configuration")

		collection.Find(bson.M{
			"type": "global",
		}).One(&configuration.GlobalConfig)

		collection.Find(bson.M{
			"type": "config",
			"name": os.Getenv("DEPLOYMENT_NAME"),
		}).One(&configuration.CustomConfig)

		// We will merge both configs in a single config file.
		// Read again from database but this store overwrite the same object.

		opts := conjungo.NewOptions()
		opts.SetTypeMergeFunc(
			reflect.TypeOf(""),
			func(t, s reflect.Value, o *conjungo.Options) (reflect.Value, error) {
				targetStr, _ := t.Interface().(string)
				sourceStr, _ := s.Interface().(string)
				finalStr := targetStr
				if sourceStr != "" {
					finalStr = sourceStr
				}
				return reflect.ValueOf(finalStr), nil
			},
		)

		// Merge Config toplevel
		conjungo.Merge(&configuration.Config, configuration.GlobalConfig, opts)
		conjungo.Merge(&configuration.Config, configuration.CustomConfig, opts)

		// Merge Kerberos Vault settings
		var kerberosvault models.KStorage
		conjungo.Merge(&kerberosvault, configuration.GlobalConfig.KStorage, opts)
		conjungo.Merge(&kerberosvault, configuration.CustomConfig.KStorage, opts)
		configuration.Config.KStorage = &kerberosvault

		// Merge Kerberos S3 settings
		var s3 models.S3
		conjungo.Merge(&s3, configuration.GlobalConfig.S3, opts)
		conjungo.Merge(&s3, configuration.CustomConfig.S3, opts)
		configuration.Config.S3 = &s3

	}
	return
}
