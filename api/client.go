// Package api provides the daemon API.
package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"

	"github.com/donovanhide/eventsource"
	"github.com/satori/go.uuid"

	"github.com/manifoldco/torus-cli/config"
	"github.com/manifoldco/torus-cli/registry"
)

const daemonAPIVersion = "v1"

type blacklisted struct{}

// Client exposes the daemon API.
type Client struct {
	registry.Client

	// Endpoints with daemon specific overrides
	Users      *UsersClient
	Machines   *MachinesClient
	KeyPairs   *KeyPairsClient
	OrgInvites *OrgInvitesClient
	Version    *VersionClient

	// Daemon only endpoints
	Session     *SessionClient
	Credentials *CredentialsClient // this replaces the registry endpoint
	Worklog     *WorklogClient

	// Cryptography related registry endpoints that should be accessed
	// via the daemon.
	Tokens          blacklisted
	Keyring         blacklisted
	KeyringMember   blacklisted
	Claims          blacklisted
	ClaimTree       blacklisted
	CredentialGraph blacklisted
}

// NewClient returns a new Client.
func NewClient(cfg *config.Config) *Client {
	rt := &apiRoundTripper{
		DefaultRoundTripper: registry.DefaultRoundTripper{
			Client: &http.Client{
				Transport: &http.Transport{
					Dial: func(network, address string) (net.Conn, error) {
						return net.Dial("unix", cfg.SocketPath)
					},
				},
			},

			Host: "http://localhost",
		},
	}

	c := &Client{Client: *registry.NewClientWithRoundTripper(rt)}

	c.Users = newUsersClient(c.Client.Users, rt)
	c.Machines = newMachinesClient(c.Client.Machines, rt)
	c.KeyPairs = newKeyPairsClient(c.Client.KeyPairs, rt)
	c.OrgInvites = newOrgInvitesClient(c.Client.OrgInvites, rt)
	c.Version = newVersionClient(c.Client.Version, rt)

	c.Session = &SessionClient{client: rt}
	c.Credentials = &CredentialsClient{client: rt}
	c.Worklog = &WorklogClient{client: rt}

	return c
}

type apiRoundTripper struct {
	registry.DefaultRoundTripper
}

// NewDaemonRequest constructs a new http.Request, with a body containing the json
// representation of body, if provided. Daemon requests are handled directly
// by the torus daemon.
func (rt *apiRoundTripper) NewDaemonRequest(method, path string,
	query *url.Values, body interface{}) (*http.Request, string, error) {

	return rt.newRequest(method, daemonAPIVersion, path, query, body)
}

func (rt *apiRoundTripper) NewRequest(method string, path string,
	query *url.Values, body interface{}) (*http.Request, error) {

	req, _, err := rt.newRequest(method, "proxy", path, query, body)
	return req, err
}

// newRequest augments the default to set a unique request id
func (rt *apiRoundTripper) newRequest(method, prefix, path string,
	query *url.Values, body interface{}) (*http.Request, string, error) {

	requestID := uuid.NewV4().String()

	prefixed := "/" + prefix + path
	req, err := rt.DefaultRoundTripper.NewRequest(method, prefixed, query, body)
	if err != nil {
		return nil, requestID, err
	}

	req.Header.Set("X-Request-ID", requestID)

	return req, requestID, nil
}

// DoWithProgress executes the HTTP request like Do, in addition to
// connecting the provided ProgressFunc to any server-sent event progress
// messages.
func (rt *apiRoundTripper) DoWithProgress(ctx context.Context, r *http.Request, v interface{}, reqID string, progress ProgressFunc) (*http.Response, error) {
	done := make(chan bool)
	req, _, err := rt.newRequest("GET", daemonAPIVersion, "/observe", nil, nil)
	if err != nil {
		return nil, err
	}
	stream, err := eventsource.SubscribeWith("", rt.Client, req)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case <-done:
				return
			case ev := <-stream.Events:
				data := ev.Data()
				raw := []byte(data)

				event := Event{}
				event.MessageType = "message"
				err = json.Unmarshal(raw, &event)
				if err != nil {
					progress(nil, err)
					go func() {
						<-done
					}()
					return
				}
				if event.ID == reqID {
					progress(&event, nil)
				}
			case err := <-stream.Errors:
				progress(nil, err)
			}
		}
	}()

	defer func() { done <- true }()

	return rt.Do(ctx, r, v)
}
