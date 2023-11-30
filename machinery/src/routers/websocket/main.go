package websocket

import (
	"context"
	"encoding/base64"
	"image"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/computervision"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

type Message struct {
	ClientID    string            `json:"client_id" bson:"client_id"`
	MessageType string            `json:"message_type" bson:"message_type"`
	Message     map[string]string `json:"message" bson:"message"`
}

type Connection struct {
	Socket  *websocket.Conn
	mu      sync.Mutex
	Cancels map[string]context.CancelFunc
}

// Concurrency handling - sending messages
func (c *Connection) WriteJson(message Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Socket.WriteJSON(message)
}

func (c *Connection) WriteMessage(bytes []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Socket.WriteMessage(websocket.TextMessage, bytes)
}

var sockets = make(map[string]*Connection)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WebsocketHandler(c *gin.Context, communication *models.Communication, captureDevice *capture.Capture) {
	w := c.Writer
	r := c.Request
	conn, err := upgrader.Upgrade(w, r, nil)

	// error handling here
	if err == nil {
		defer conn.Close()

		var message Message
		err = conn.ReadJSON(&message)
		clientID := message.ClientID
		if sockets[clientID] == nil {
			connection := new(Connection)
			connection.Socket = conn
			sockets[clientID] = connection
			sockets[clientID].Cancels = make(map[string]context.CancelFunc)
		}

		// Continuously read messages
		for {
			switch message.MessageType {
			case "hello":
				m := message.Message
				bePolite := Message{
					ClientID:    clientID,
					MessageType: "hello-back",
					Message: map[string]string{
						"message": "Hello " + m["client_id"] + "!",
					},
				}
				sockets[clientID].WriteJson(bePolite)

			case "stop-sd":
				_, exists := sockets[clientID].Cancels["stream-sd"]
				if exists {
					sockets[clientID].Cancels["stream-sd"]()
				} else {
					log.Log.Error("Streaming sd does not exists for " + clientID)
				}

			case "stream-sd":
				if communication.CameraConnected {
					_, exists := sockets[clientID].Cancels["stream-sd"]
					if exists {
						log.Log.Info("Already streaming sd for " + clientID)
					} else {
						startStream := Message{
							ClientID:    clientID,
							MessageType: "stream-sd",
							Message: map[string]string{
								"message": "Start streaming low resolution",
							},
						}
						sockets[clientID].WriteJson(startStream)

						ctx, cancel := context.WithCancel(context.Background())
						sockets[clientID].Cancels["stream-sd"] = cancel
						go ForwardSDStream(ctx, clientID, sockets[clientID], communication, captureDevice)
					}
				}
			}

			err = conn.ReadJSON(&message)
			if err != nil {
				break
			}
		}
		// If clientID is in sockets
		_, exists := sockets[clientID]
		if exists {
			delete(sockets, clientID)
			log.Log.Info("WebsocketHandler: " + clientID + ": terminated and disconnected websocket connection.")
		}
	}
}

func ForwardSDStream(ctx context.Context, clientID string, connection *Connection, communication *models.Communication, captureDevice *capture.Capture) {

	queue := communication.Queue
	cursor := queue.Latest()

logreader:
	for {
		var encodedImage string
		rtspClient := captureDevice.RTSPClient
		if queue != nil && cursor != nil && rtspClient != nil {
			pkt, err := cursor.ReadPacket()
			if err == nil {
				if !pkt.IsKeyFrame {
					continue
				}

				var img image.YCbCr
				rtspSubClient := captureDevice.RTSPSubClient
				if rtspSubClient != nil {
					img, err = (*rtspSubClient).DecodePacket(pkt)
				} else {
					img, err = (*rtspSubClient).DecodePacket(pkt)
				}

				if err == nil {
					bytes, _ := computervision.ImageToBytes(&img)
					encodedImage = base64.StdEncoding.EncodeToString(bytes)
				}

			} else {
				log.Log.Error("ForwardSDStream:" + err.Error())
				break logreader
			}
		}

		startStrean := Message{
			ClientID:    clientID,
			MessageType: "image",
			Message: map[string]string{
				"base64": encodedImage,
			},
		}
		err := connection.WriteJson(startStrean)
		if err != nil {
			log.Log.Error("ForwardSDStream:" + err.Error())
			break logreader
		}
		select {
		case <-ctx.Done():
			break logreader
		default:
		}
	}

	//frame.Free()

	// Close socket for streaming
	_, exists := connection.Cancels["stream-sd"]
	if exists {
		delete(connection.Cancels, "stream-sd")
	} else {
		log.Log.Error("Streaming sd does not exists for " + clientID)
	}

	// Send stop streaming message
	log.Log.Info("ForwardSDStream: stop sending streaming over websocket")
}
