package mqtt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/machinery/src/models"
	log "github.com/sirupsen/logrus"
)

const (
	remoteAccessEnvironment = "AGENT_REMOTE_ACCESS_ENABLED"
	remoteHistoryLimit      = 500
	remoteSessionLimit      = 5
	remoteOutputChunkSize   = 4096
	remoteInputLimit        = 64 * 1024
	remoteSessionLifetime   = time.Hour
)

type remoteSession struct {
	id     string
	kind   string
	pty    *os.File
	cancel context.CancelFunc
	logs   chan string
	done   chan struct{}
	timer  *time.Timer
}

type remoteAccessManager struct {
	mu       sync.Mutex
	sessions map[string]*remoteSession
	history  []string
}

var (
	remoteAccess             = newRemoteAccessManager()
	remoteHookOnce           sync.Once
	errRemoteDisabled        = errors.New("remote access is disabled on this agent")
	errRemoteUnauthenticated = errors.New("remote access requires encrypted MQTT")
	errRemoteSessionExists   = errors.New("remote session already exists")
	errRemoteSessionLimit    = errors.New("remote session limit reached")
)

func newRemoteAccessManager() *remoteAccessManager {
	return &remoteAccessManager{sessions: make(map[string]*remoteSession)}
}

func installRemoteAccessHook() {
	remoteHookOnce.Do(func() {
		log.AddHook(remoteAccess)
	})
}

func (manager *remoteAccessManager) Levels() []log.Level {
	return log.AllLevels
}

func (manager *remoteAccessManager) Fire(entry *log.Entry) error {
	line, err := json.Marshal(map[string]interface{}{
		"timestamp": entry.Time.Format(time.RFC3339Nano),
		"level":     entry.Level.String(),
		"message":   entry.Message,
		"fields":    entry.Data,
	})
	if err != nil {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString(append(line, '\n'))

	manager.mu.Lock()
	manager.history = append(manager.history, encoded)
	if len(manager.history) > remoteHistoryLimit {
		manager.history = manager.history[len(manager.history)-remoteHistoryLimit:]
	}
	for _, session := range manager.sessions {
		if session.kind != "logs" {
			continue
		}
		select {
		case session.logs <- encoded:
		default:
		}
	}
	manager.mu.Unlock()
	return nil
}

func remoteAccessEnabled() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(remoteAccessEnvironment)))
	return err == nil && enabled
}

func validateRemoteAccess(authenticated bool) error {
	if !authenticated {
		return errRemoteUnauthenticated
	}
	if !remoteAccessEnabled() {
		return errRemoteDisabled
	}
	return nil
}

func decodeRemotePayload(payload models.Payload) (models.RemoteSessionPayload, error) {
	data, err := json.Marshal(payload.Value)
	if err != nil {
		return models.RemoteSessionPayload{}, err
	}
	var request models.RemoteSessionPayload
	if err := json.Unmarshal(data, &request); err != nil {
		return models.RemoteSessionPayload{}, err
	}
	if request.SessionID == "" || len(request.SessionID) > 128 {
		return models.RemoteSessionPayload{}, errors.New("invalid remote session id")
	}
	return request, nil
}

func normalizeTerminalSize(rows uint16, columns uint16) (uint16, uint16) {
	if rows < 5 {
		rows = 24
	}
	if rows > 200 {
		rows = 200
	}
	if columns < 20 {
		columns = 80
	}
	if columns > 400 {
		columns = 400
	}
	return rows, columns
}

func HandleRemoteSessionOpen(client paho.Client, hubKey string, payload models.Payload, authenticated bool, configuration *models.Configuration) {
	request, err := decodeRemotePayload(payload)
	if err != nil {
		return
	}
	if accessErr := validateRemoteAccess(authenticated); accessErr != nil {
		publishRemoteStatus(client, hubKey, configuration, request.SessionID, request.Kind, "error", accessErr.Error())
		return
	}

	switch request.Kind {
	case "logs":
		err = remoteAccess.openLogs(client, hubKey, configuration, request)
	case "shell":
		err = remoteAccess.openShell(client, hubKey, configuration, request)
	default:
		err = errors.New("unsupported remote session kind")
	}
	if err != nil {
		if errors.Is(err, errRemoteSessionExists) {
			publishRemoteStatus(client, hubKey, configuration, request.SessionID, request.Kind, "opened", "")
			return
		}
		publishRemoteStatus(client, hubKey, configuration, request.SessionID, request.Kind, "error", err.Error())
	}
}

func HandleRemoteSessionInput(client paho.Client, hubKey string, payload models.Payload, authenticated bool, configuration *models.Configuration) {
	if validateRemoteAccess(authenticated) != nil {
		return
	}
	request, err := decodeRemotePayload(payload)
	if err != nil || len(request.Data) > remoteInputLimit*2 {
		return
	}
	data, err := base64.StdEncoding.DecodeString(request.Data)
	if err != nil || len(data) > remoteInputLimit {
		return
	}
	remoteAccess.mu.Lock()
	session := remoteAccess.sessions[request.SessionID]
	remoteAccess.mu.Unlock()
	if session == nil || session.kind != "shell" || session.pty == nil {
		publishRemoteStatus(client, hubKey, configuration, request.SessionID, "shell", "error", "remote session is not open")
		return
	}
	if _, err := session.pty.Write(data); err != nil {
		publishRemoteStatus(client, hubKey, configuration, request.SessionID, "shell", "error", "failed to write terminal input")
	}
}

func HandleRemoteSessionResize(payload models.Payload, authenticated bool) {
	if validateRemoteAccess(authenticated) != nil {
		return
	}
	request, err := decodeRemotePayload(payload)
	if err != nil {
		return
	}
	rows, columns := normalizeTerminalSize(request.Rows, request.Columns)
	remoteAccess.mu.Lock()
	session := remoteAccess.sessions[request.SessionID]
	remoteAccess.mu.Unlock()
	if session != nil && session.kind == "shell" && session.pty != nil {
		_ = pty.Setsize(session.pty, &pty.Winsize{Rows: rows, Cols: columns})
	}
}

func HandleRemoteSessionClose(payload models.Payload, authenticated bool) {
	if !authenticated {
		return
	}
	request, err := decodeRemotePayload(payload)
	if err == nil {
		remoteAccess.close(request.SessionID)
	}
}

func (manager *remoteAccessManager) reserve(session *remoteSession) ([]string, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if _, exists := manager.sessions[session.id]; exists {
		return nil, errRemoteSessionExists
	}
	if len(manager.sessions) >= remoteSessionLimit {
		return nil, errRemoteSessionLimit
	}
	manager.sessions[session.id] = session
	history := append([]string(nil), manager.history...)
	return history, nil
}

func (manager *remoteAccessManager) expire(client paho.Client, hubKey string, configuration *models.Configuration, session *remoteSession) {
	session.timer = time.AfterFunc(remoteSessionLifetime, func() {
		manager.close(session.id)
		if session.kind == "logs" {
			publishRemoteStatus(client, hubKey, configuration, session.id, session.kind, "closed", "session lifetime reached")
		}
	})
}

func (manager *remoteAccessManager) openLogs(client paho.Client, hubKey string, configuration *models.Configuration, request models.RemoteSessionPayload) error {
	session := &remoteSession{
		id:   request.SessionID,
		kind: "logs",
		logs: make(chan string, 256),
		done: make(chan struct{}),
	}
	history, err := manager.reserve(session)
	if err != nil {
		return err
	}
	manager.expire(client, hubKey, configuration, session)
	tail := request.Tail
	if tail <= 0 || tail > remoteHistoryLimit {
		tail = 200
	}
	if len(history) > tail {
		history = history[len(history)-tail:]
	}

	publishRemoteStatus(client, hubKey, configuration, session.id, session.kind, "opened", "")
	go func() {
		for _, line := range history {
			publishRemoteOutput(client, hubKey, configuration, session.id, session.kind, line)
		}
		for {
			select {
			case line := <-session.logs:
				publishRemoteOutput(client, hubKey, configuration, session.id, session.kind, line)
			case <-session.done:
				return
			}
		}
	}()
	return nil
}

func (manager *remoteAccessManager) openShell(client paho.Client, hubKey string, configuration *models.Configuration, request models.RemoteSessionPayload) error {
	rows, columns := normalizeTerminalSize(request.Rows, request.Columns)
	ctx, cancel := context.WithCancel(context.Background())
	session := &remoteSession{
		id:     request.SessionID,
		kind:   "shell",
		cancel: cancel,
		done:   make(chan struct{}),
	}
	if _, err := manager.reserve(session); err != nil {
		cancel()
		return err
	}
	manager.expire(client, hubKey, configuration, session)
	command := exec.CommandContext(ctx, "/bin/sh")
	command.Env = append(os.Environ(), "TERM=xterm-256color", "HISTFILE=/dev/null")
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: rows, Cols: columns})
	if err != nil {
		manager.remove(session.id)
		cancel()
		return err
	}
	session.pty = terminal

	publishRemoteStatus(client, hubKey, configuration, session.id, session.kind, "opened", "")
	go manager.forwardShell(client, hubKey, configuration, session, command)
	return nil
}

func (manager *remoteAccessManager) forwardShell(client paho.Client, hubKey string, configuration *models.Configuration, session *remoteSession, command *exec.Cmd) {
	buffer := make([]byte, remoteOutputChunkSize)
	for {
		count, err := session.pty.Read(buffer)
		if count > 0 {
			publishRemoteOutput(client, hubKey, configuration, session.id, session.kind, base64.StdEncoding.EncodeToString(buffer[:count]))
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				publishRemoteStatus(client, hubKey, configuration, session.id, session.kind, "error", "terminal stream closed unexpectedly")
			}
			break
		}
	}
	_ = command.Wait()
	manager.remove(session.id)
	publishRemoteStatus(client, hubKey, configuration, session.id, session.kind, "closed", "")
}

func (manager *remoteAccessManager) close(sessionID string) {
	manager.mu.Lock()
	session := manager.sessions[sessionID]
	delete(manager.sessions, sessionID)
	manager.mu.Unlock()
	if session == nil {
		return
	}
	if session.cancel != nil {
		session.cancel()
	}
	if session.pty != nil {
		_ = session.pty.Close()
	}
	if session.logs != nil {
		close(session.done)
	}
	if session.timer != nil {
		session.timer.Stop()
	}
}

func (manager *remoteAccessManager) remove(sessionID string) {
	manager.mu.Lock()
	session := manager.sessions[sessionID]
	delete(manager.sessions, sessionID)
	manager.mu.Unlock()
	if session != nil && session.timer != nil {
		session.timer.Stop()
	}
}

func publishRemoteStatus(client paho.Client, hubKey string, configuration *models.Configuration, sessionID string, kind string, state string, errorMessage string) {
	status := models.RemoteSessionStatus{
		Timestamp: time.Now().Unix(),
		SessionID: sessionID,
		Kind:      kind,
		State:     state,
		Error:     errorMessage,
	}
	value, _ := json.Marshal(status)
	var statusValue map[string]interface{}
	_ = json.Unmarshal(value, &statusValue)
	publishRemote(client, hubKey, configuration, "remote-session-status", statusValue, 1)
}

func publishRemoteOutput(client paho.Client, hubKey string, configuration *models.Configuration, sessionID string, kind string, data string) {
	publishRemote(client, hubKey, configuration, "remote-session-output", map[string]interface{}{
		"timestamp":  time.Now().Unix(),
		"session_id": sessionID,
		"kind":       kind,
		"data":       data,
	}, 0)
}

func publishRemote(client paho.Client, hubKey string, configuration *models.Configuration, action string, value map[string]interface{}, qos byte) {
	message := models.Message{Payload: models.Payload{
		Action:   action,
		DeviceId: configuration.Config.Key,
		Value:    value,
	}}
	payload, err := models.PackageMQTTMessage(configuration, message)
	if err == nil {
		client.Publish("kerberos/hub/"+hubKey, qos, false, payload)
	}
}
