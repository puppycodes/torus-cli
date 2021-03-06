package api

import (
	"context"
	"net/url"

	"github.com/manifoldco/torus-cli/apitypes"
	"github.com/manifoldco/torus-cli/identity"
)

// WorklogClient views and resolves worklog items in the daemon.
type WorklogClient struct {
	client *apiRoundTripper
}

// List returns the list of all worklog items in the given org.
func (w *WorklogClient) List(ctx context.Context, orgID *identity.ID) ([]apitypes.WorklogItem, error) {
	v := &url.Values{}
	if orgID != nil {
		v.Set("org_id", orgID.String())
	}

	req, _, err := w.client.NewDaemonRequest("GET", "/worklog", v, nil)
	if err != nil {
		return nil, err
	}

	var resp []apitypes.WorklogItem

	_, err = w.client.Do(ctx, req, &resp)
	return resp, err
}

// Get returns the worklog item with the given id in the given org.
func (w *WorklogClient) Get(ctx context.Context, orgID *identity.ID, ident *apitypes.WorklogID) (*apitypes.WorklogItem, error) {
	v := &url.Values{}
	if orgID != nil {
		v.Set("org_id", orgID.String())
	}

	req, _, err := w.client.NewDaemonRequest("GET", "/worklog/"+ident.String(), v, nil)
	if err != nil {
		return nil, err
	}

	var entry apitypes.WorklogItem

	_, err = w.client.Do(ctx, req, &entry)
	return &entry, err
}

// Resolve resolves the worklog item with the given id in the given org.
func (w *WorklogClient) Resolve(ctx context.Context, orgID *identity.ID, ident *apitypes.WorklogID) (*apitypes.WorklogResult, error) {
	v := &url.Values{}
	if orgID != nil {
		v.Set("org_id", orgID.String())
	}

	req, _, err := w.client.NewDaemonRequest("POST", "/worklog/"+ident.String(), v, nil)
	if err != nil {
		return nil, err
	}

	var res apitypes.WorklogResult

	_, err = w.client.Do(ctx, req, &res)
	return &res, err
}
