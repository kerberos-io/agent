package config

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/InVisionApp/conjungo"
	"github.com/kerberos-io/agent/machinery/src/database"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"go.mongodb.org/mongo-driver/bson"
)

// ReadUserConfig Reads the user configuration of the Kerberos Open Source instance.
// This will return a models.User struct including the username, password,
// selected language, and if the installation was completed or not.
func ReadUserConfig(configDirectory string) (userConfig models.User) {
	for {
		jsonFile, err := os.Open(configDirectory + "/data/config/user.json")
		if err != nil {
			log.Log.Error("Config file is not found " + configDirectory + "/data/config/user.json, trying again in 5s: " + err.Error())
			time.Sleep(5 * time.Second)
		} else {
			log.Log.Info("Successfully Opened user.json")
			byteValue, _ := ioutil.ReadAll(jsonFile)
			err = json.Unmarshal(byteValue, &userConfig)
			if err != nil {
				log.Log.Error("JSON file not valid: " + err.Error())
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

func OpenConfig(configDirectory string, configuration *models.Configuration) {

	// We are checking which deployment this is running, so we can load
	// into the configuration as expected.

	if os.Getenv("DEPLOYMENT") == "factory" || os.Getenv("MACHINERY_ENVIRONMENT") == "kubernetes" {

		// Factory deployment means that configuration is stored in MongoDB
		// Multiple agents have there configuration stored, and can benefit from
		// the concept of a global concept.

		// Write to mongodb
		client := database.New()

		db := client.Client.Database(database.DatabaseName)
		collection := db.Collection("configuration")

		var globalConfig models.Config
		res := collection.FindOne(context.Background(), bson.M{
			"type": "global",
		})

		if res.Err() != nil {
			log.Log.Error("Could not find global configuration, using default configuration.")
			panic("Could not find global configuration, using default configuration.")
		}
		err := res.Decode(&globalConfig)
		if err != nil {
			log.Log.Error("Could not find global configuration, using default configuration.")
			panic("Could not find global configuration, using default configuration.")
		}
		if globalConfig.Type != "global" {
			log.Log.Error("Could not find global configuration, might missed the mongodb connection.")
			panic("Could not find global configuration, might missed the mongodb connection.")
		}

		configuration.GlobalConfig = globalConfig

		var customConfig models.Config
		deploymentName := os.Getenv("DEPLOYMENT_NAME")
		res = collection.FindOne(context.Background(), bson.M{
			"type": "config",
			"name": deploymentName,
		})
		if res.Err() != nil {
			log.Log.Error("Could not find configuration for " + deploymentName + ", using global configuration.")
		}
		err = res.Decode(&customConfig)
		if err != nil {
			log.Log.Error("Could not find configuration for " + deploymentName + ", using global configuration.")
		}

		if customConfig.Type != "config" {
			log.Log.Error("Could not find custom configuration, might missed the mongodb connection.")
			panic("Could not find custom configuration, might missed the mongodb connection.")
		}
		configuration.CustomConfig = customConfig

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

		// Reset main configuration Config.
		configuration.Config = models.Config{}

		// Merge the global settings in the main config
		conjungo.Merge(&configuration.Config, configuration.GlobalConfig, opts)

		// Now we might override some settings with the custom config
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

		// Merge Encryption settings
		var encryption models.Encryption
		conjungo.Merge(&encryption, configuration.GlobalConfig.Encryption, opts)
		conjungo.Merge(&encryption, configuration.CustomConfig.Encryption, opts)
		configuration.Config.Encryption = &encryption

		// Merge timetable manually because it's an array
		configuration.Config.Timetable = configuration.CustomConfig.Timetable

		// Cleanup
		opts = nil

	} else if os.Getenv("DEPLOYMENT") == "" || os.Getenv("DEPLOYMENT") == "agent" {

		// Local deployment means we do a stand-alone installation
		// Configuration is stored into a json file, and there is only 1 agent.

		// Open device config
		for {
			jsonFile, err := os.Open(configDirectory + "/data/config/config.json")
			if err != nil {
				log.Log.Error("Config file is not found " + configDirectory + "/data/config/config.json" + ", trying again in 5s.")
				time.Sleep(5 * time.Second)
			} else {
				log.Log.Info("Successfully Opened config.json from " + configuration.Name)
				byteValue, _ := ioutil.ReadAll(jsonFile)
				err = json.Unmarshal(byteValue, &configuration.Config)
				jsonFile.Close()
				if err != nil {
					log.Log.Error("JSON file not valid: " + err.Error())
				} else {
					err = json.Unmarshal(byteValue, &configuration.CustomConfig)
					if err != nil {
						log.Log.Error("JSON file not valid: " + err.Error())
					} else {
						break
					}
				}
				time.Sleep(5 * time.Second)
			}
			jsonFile.Close()
		}

	}

	return
}

// This function will override the configuration with environment variables.
func OverrideWithEnvironmentVariables(configuration *models.Configuration) {
	environmentVariables := os.Environ()
	for _, env := range environmentVariables {
		if strings.Contains(env, "AGENT_") {
			key := strings.Split(env, "=")[0]
			value := os.Getenv(key)
			switch key {

			/* General configuration */
			case "AGENT_KEY":
				configuration.Config.Key = value
				break
			case "AGENT_NAME":
				configuration.Config.FriendlyName = value
				break
			case "AGENT_TIMEZONE":
				configuration.Config.Timezone = value
				break
			case "AGENT_OFFLINE":
				configuration.Config.Offline = value
				break
			case "AGENT_AUTO_CLEAN":
				configuration.Config.AutoClean = value
				break
			case "AGENT_AUTO_CLEAN_MAX_SIZE":
				size, err := strconv.ParseInt(value, 10, 64)
				if err == nil {
					configuration.Config.MaxDirectorySize = size
				}
				break

			/* Camera configuration */
			case "AGENT_CAPTURE_IPCAMERA_RTSP":
				configuration.Config.Capture.IPCamera.RTSP = value
				break
			case "AGENT_CAPTURE_IPCAMERA_SUB_RTSP":
				configuration.Config.Capture.IPCamera.SubRTSP = value
				break

				/* ONVIF connnection settings */
			case "AGENT_CAPTURE_IPCAMERA_ONVIF":
				configuration.Config.Capture.IPCamera.ONVIF = value
				break
			case "AGENT_CAPTURE_IPCAMERA_ONVIF_XADDR":
				configuration.Config.Capture.IPCamera.ONVIFXAddr = value
				break
			case "AGENT_CAPTURE_IPCAMERA_ONVIF_USERNAME":
				configuration.Config.Capture.IPCamera.ONVIFUsername = value
				break
			case "AGENT_CAPTURE_IPCAMERA_ONVIF_PASSWORD":
				configuration.Config.Capture.IPCamera.ONVIFPassword = value
				break

			/* Recording mode */
			case "AGENT_CAPTURE_RECORDING":
				configuration.Config.Capture.Recording = value
				break
			case "AGENT_CAPTURE_CONTINUOUS":
				configuration.Config.Capture.Continuous = value
				break
			case "AGENT_CAPTURE_LIVEVIEW":
				configuration.Config.Capture.Liveview = value
				break
			case "AGENT_CAPTURE_MOTION":
				configuration.Config.Capture.Motion = value
				break
			case "AGENT_CAPTURE_SNAPSHOTS":
				configuration.Config.Capture.Snapshots = value
				break
			case "AGENT_CAPTURE_PRERECORDING":
				duration, err := strconv.ParseInt(value, 10, 64)
				if err == nil {
					configuration.Config.Capture.PreRecording = duration
				}
				break
			case "AGENT_CAPTURE_POSTRECORDING":
				duration, err := strconv.ParseInt(value, 10, 64)
				if err == nil {
					configuration.Config.Capture.PostRecording = duration
				}
				break
			case "AGENT_CAPTURE_MAXLENGTH":
				duration, err := strconv.ParseInt(value, 10, 64)
				if err == nil {
					configuration.Config.Capture.MaxLengthRecording = duration
				}
				break
			case "AGENT_CAPTURE_PIXEL_CHANGE":
				count, err := strconv.Atoi(value)
				if err == nil {
					configuration.Config.Capture.PixelChangeThreshold = count
				}
				break
			case "AGENT_CAPTURE_FRAGMENTED":
				configuration.Config.Capture.Fragmented = value
				break
			case "AGENT_CAPTURE_FRAGMENTED_DURATION":
				duration, err := strconv.ParseInt(value, 10, 64)
				if err == nil {
					configuration.Config.Capture.FragmentedDuration = duration
				}
				break

			/* Conditions */

			case "AGENT_TIME":
				configuration.Config.Time = value
				break
			case "AGENT_TIMETABLE":
				var timetable []*models.Timetable

				// Convert value to timetable array with (start1, end1, start2, end2)
				// Where days are limited by ; and time by ,
				// su;mo;tu;we;th;fr;sa
				// 0,43199,43200,86400;0,43199,43200,86400

				// Split days
				daysString := strings.Split(value, ";")
				for _, dayString := range daysString {
					// Split time
					timeString := strings.Split(dayString, ",")
					if len(timeString) == 4 {
						start1, err := strconv.ParseInt(timeString[0], 10, 64)
						if err != nil {
							continue
						}
						end1, err := strconv.ParseInt(timeString[1], 10, 64)
						if err != nil {
							continue
						}
						start2, err := strconv.ParseInt(timeString[2], 10, 64)
						if err != nil {
							continue
						}
						end2, err := strconv.ParseInt(timeString[3], 10, 64)
						if err != nil {
							continue
						}
						timetable = append(timetable, &models.Timetable{
							Start1: int(start1),
							End1:   int(end1),
							Start2: int(start2),
							End2:   int(end2),
						})
					}
				}
				configuration.Config.Timetable = timetable
				break

			case "AGENT_REGION_POLYGON":
				var coordinates []models.Coordinate

				// Convert value to coordinates array
				// 0,0;1,1;2,2;3,3
				coordinatesString := strings.Split(value, ";")
				for _, coordinateString := range coordinatesString {
					coordinate := strings.Split(coordinateString, ",")
					if len(coordinate) == 2 {
						x, err := strconv.ParseFloat(coordinate[0], 64)
						if err != nil {
							continue
						}
						y, err := strconv.ParseFloat(coordinate[1], 64)
						if err != nil {
							continue
						}
						coordinates = append(coordinates, models.Coordinate{
							X: x,
							Y: y,
						})
					}
				}

				configuration.Config.Region.Polygon = []models.Polygon{
					{
						Coordinates: coordinates,
						ID:          "0",
					},
				}
				break

			/* MQTT settings for bi-directional communication */
			case "AGENT_MQTT_URI":
				configuration.Config.MQTTURI = value
				break
			case "AGENT_MQTT_USERNAME":
				configuration.Config.MQTTUsername = value
				break
			case "AGENT_MQTT_PASSWORD":
				configuration.Config.MQTTPassword = value
				break

			/* Real-time streaming of keyframes to a MQTT topic */
			case "AGENT_REALTIME_PROCESSING":
				configuration.Config.RealtimeProcessing = value
				break
			case "AGENT_REALTIME_PROCESSING_TOPIC":
				configuration.Config.RealtimeProcessingTopic = value
				break

			/* WebRTC settings for live-streaming (remote) */
			case "AGENT_STUN_URI":
				configuration.Config.STUNURI = value
				break
			case "AGENT_FORCE_TURN":
				configuration.Config.ForceTurn = value
				break
			case "AGENT_TURN_URI":
				configuration.Config.TURNURI = value
				break
			case "AGENT_TURN_USERNAME":
				configuration.Config.TURNUsername = value
				break
			case "AGENT_TURN_PASSWORD":
				configuration.Config.TURNPassword = value
				break

			/* Cloud settings for persisting recordings */
			case "AGENT_CLOUD":
				configuration.Config.Cloud = value
				break

			case "AGENT_REMOVE_AFTER_UPLOAD":
				configuration.Config.RemoveAfterUpload = value
				break

			/* When connected and storing in Kerberos Hub (SAAS) */
			case "AGENT_HUB_ENCRYPTION":
				configuration.Config.HubEncryption = value
				break
			case "AGENT_HUB_URI":
				configuration.Config.HubURI = value
				break
			case "AGENT_HUB_KEY":
				configuration.Config.HubKey = value
				break
			case "AGENT_HUB_PRIVATE_KEY":
				configuration.Config.HubPrivateKey = value
				break
			case "AGENT_HUB_SITE":
				configuration.Config.HubSite = value
				break
			case "AGENT_HUB_REGION":
				configuration.Config.S3.Region = value
				break

			/* When storing in a Kerberos Vault */
			case "AGENT_KERBEROSVAULT_URI":
				configuration.Config.KStorage.URI = value
				break
			case "AGENT_KERBEROSVAULT_ACCESS_KEY":
				configuration.Config.KStorage.AccessKey = value
				break
			case "AGENT_KERBEROSVAULT_SECRET_KEY":
				configuration.Config.KStorage.SecretAccessKey = value
				break
			case "AGENT_KERBEROSVAULT_PROVIDER":
				configuration.Config.KStorage.Provider = value
				break
			case "AGENT_KERBEROSVAULT_DIRECTORY":
				configuration.Config.KStorage.Directory = value
				break

			/* When storing in dropbox */
			case "AGENT_DROPBOX_ACCESS_TOKEN":
				configuration.Config.Dropbox.AccessToken = value
				break
			case "AGENT_DROPBOX_DIRECTORY":
				configuration.Config.Dropbox.Directory = value
				break

			/* When encryption is enabled */
			case "AGENT_ENCRYPTION":
				configuration.Config.Encryption.Enabled = value
				break
			case "AGENT_ENCRYPTION_RECORDINGS":
				configuration.Config.Encryption.Recordings = value
				break
			case "AGENT_ENCRYPTION_FINGERPRINT":
				configuration.Config.Encryption.Fingerprint = value
				break
			case "AGENT_ENCRYPTION_PRIVATE_KEY":
				encryptionPrivateKey := strings.ReplaceAll(value, "\\n", "\n")
				configuration.Config.Encryption.PrivateKey = encryptionPrivateKey
				break
			case "AGENT_ENCRYPTION_SYMMETRIC_KEY":
				configuration.Config.Encryption.SymmetricKey = value
				break
			}
		}
	}
}

func SaveConfig(configDirectory string, config models.Config, configuration *models.Configuration, communication *models.Communication) error {
	if !communication.IsConfiguring.IsSet() {
		communication.IsConfiguring.Set()

		err := StoreConfig(configDirectory, config)
		if err != nil {
			communication.IsConfiguring.UnSet()
			return err
		}

		if communication.CameraConnected {
			select {
			case communication.HandleBootstrap <- "restart":
				log.Log.Info("config.main.SaveConfig(): update config, restart agent.")
			case <-time.After(1 * time.Second):
				log.Log.Info("config.main.SaveConfig(): update config, restart agent.")
			}
		}

		communication.IsConfiguring.UnSet()

		return nil
	} else {
		return errors.New("☄ Already reconfiguring")
	}
}

func StoreConfig(configDirectory string, config models.Config) error {

	// Encryption key can be set wrong.
	if config.Encryption != nil {
		encryptionPrivateKey := config.Encryption.PrivateKey
		// Replace \\n by \n
		encryptionPrivateKey = strings.ReplaceAll(encryptionPrivateKey, "\\n", "\n")
		config.Encryption.PrivateKey = encryptionPrivateKey
	}

	// Save into database
	if os.Getenv("DEPLOYMENT") == "factory" || os.Getenv("MACHINERY_ENVIRONMENT") == "kubernetes" {
		// Write to mongodb
		client := database.New()

		db := client.Database(database.DatabaseName)
		collection := db.Collection("configuration")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := collection.UpdateOne(ctx, bson.M{
			"type": "config",
			"name": os.Getenv("DEPLOYMENT_NAME"),
		}, bson.M{"$set": config})

		return err

		// Save into file
	} else if os.Getenv("DEPLOYMENT") == "" || os.Getenv("DEPLOYMENT") == "agent" {
		res, _ := json.MarshalIndent(config, "", "\t")
		err := ioutil.WriteFile(configDirectory+"/data/config/config.json", res, 0644)
		return err
	}

	return errors.New("Not able to update config")
}
