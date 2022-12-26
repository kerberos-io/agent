package websocket

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kerberos-io/agent/machinery/src/utils"
)

type Message struct {
	ClientID    string            `json:"client_id" bson:"client_id"`
	Group       string            `json:"group" bson:"group"`
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

func WebsocketHandler(c *gin.Context) {
	w := c.Writer
	r := c.Request
	conn, err := upgrader.Upgrade(w, r, nil)
	// error handling here
	if err == nil {
		defer conn.Close()
		// Register Global socket list for broadcasting.
		clientID := utils.RandStringBytesMaskImpr(5)
		if sockets[clientID] == nil {
			connection := new(Connection)
			connection.Socket = conn
			sockets[clientID] = connection
			sockets[clientID].Cancels = make(map[string]context.CancelFunc)
		}

		// Continuously read messages
		var message Message
		for {
			err = conn.ReadJSON(&message)
			if err != nil {
				break
			}

			switch message.MessageType {
			// Start logging for a specific container
			case "request-sd":
				m := message.Message
				_, exists := sockets[clientID].Cancels["log-"+m["pod_name"]]
				if exists {
					fmt.Println("Already streaming logs for " + m["pod_name"])
				} else {
					ctx, cancel := context.WithCancel(context.Background())
					sockets[clientID].Cancels["log-"+m["pod_name"]] = cancel
					go ForwardSDStream(ctx, sockets[clientID])
				}
			// Stop logging for a specific container
			case "request-hd":
				m := message.Message
				_, exists := sockets[clientID].Cancels["log-"+m["pod_name"]]
				if exists {
					sockets[clientID].Cancels["log-"+m["pod_name"]]()
					delete(sockets[clientID].Cancels, "log-"+m["pod_name"])
					fmt.Println("Cancel log request!")
				} else {
					fmt.Println("No cancel func found for " + clientID)
				}
			}

			fmt.Println(clientID + ": reading.")
			time.Sleep(time.Second * 2)
		}
		// If clientID is in sockets
		_, exists := sockets[clientID]
		if exists {
			delete(sockets, clientID)
			fmt.Println(clientID + ": terminated and disconnected websocket connection.")
		}
	}
}

func ForwardSDStream(ctx context.Context, connection *Connection) {
logreader:
	for {
		// Can be killed from the outside.
		select {
		case <-ctx.Done():
			//cancel()
			fmt.Println("KILLED from new message")
			break logreader
		default:
		}
	}
	fmt.Println("Stopped sending streaming...")
}
