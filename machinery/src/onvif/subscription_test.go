package onvif

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/kerberos-io/onvif"
)

// newSubscriptionTestDevice starts a fake ONVIF device and returns a connected
// device plus the request bodies it received on the subscription endpoint.
func newSubscriptionTestDevice(t *testing.T, response string) (*onvif.Device, string, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var subscriptionBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/onvif/subscription" {
			mu.Lock()
			subscriptionBodies = append(subscriptionBodies, string(body))
			mu.Unlock()
			_, _ = w.Write([]byte(response))
			return
		}
		_, _ = w.Write([]byte(`<Envelope><Body/></Envelope>`))
	}))
	t.Cleanup(srv.Close)

	dev, err := onvif.NewDevice(onvif.DeviceParams{Xaddr: strings.TrimPrefix(srv.URL, "http://")})
	if err != nil {
		t.Fatalf("NewDevice() error = %v", err)
	}
	subscriptionAddress := srv.URL + "/onvif/subscription?subscription=7&idx=2"
	return dev, subscriptionAddress, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), subscriptionBodies...)
	}
}

func assertSubscriptionAddressing(t *testing.T, body string, action string) {
	t.Helper()

	for _, want := range []string{
		`<wsa:Action xmlns:wsa="http://www.w3.org/2005/08/addressing">` + action + `</wsa:Action>`,
		`<wsa:To xmlns:wsa="http://www.w3.org/2005/08/addressing">`,
		`/onvif/subscription?subscription=7&amp;idx=2</wsa:To>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("request body does not contain %q:\n%s", want, body)
		}
	}
}

// Some devices identify a pull-point subscription by wsa:To and reject
// PullMessages without it as NotAuthorized.
func TestGetEventMessagesSendsSubscriptionAddressingHeaders(t *testing.T) {
	dev, subscriptionAddress, bodies := newSubscriptionTestDevice(t, `<Envelope><Body><PullMessagesResponse/></Body></Envelope>`)

	if _, err := GetEventMessages(dev, subscriptionAddress); err != nil {
		t.Fatalf("GetEventMessages() error = %v", err)
	}

	received := bodies()
	if len(received) != 1 {
		t.Fatalf("subscription requests = %d, want 1", len(received))
	}
	assertSubscriptionAddressing(t, received[0], pullMessagesAction)
}

func TestUnsubscribePullPointSendsSubscriptionAddressingHeaders(t *testing.T) {
	dev, subscriptionAddress, bodies := newSubscriptionTestDevice(t, `<Envelope><Body><UnsubscribeResponse/></Body></Envelope>`)

	if err := UnsubscribePullPoint(dev, subscriptionAddress); err != nil {
		t.Fatalf("UnsubscribePullPoint() error = %v", err)
	}

	received := bodies()
	if len(received) != 1 {
		t.Fatalf("subscription requests = %d, want 1", len(received))
	}
	assertSubscriptionAddressing(t, received[0], unsubscribeAction)
}
