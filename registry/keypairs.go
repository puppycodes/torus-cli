package registry

import (
	"context"
	"log"
	"net/url"

	"github.com/manifoldco/torus-cli/apitypes"
	"github.com/manifoldco/torus-cli/envelope"
	"github.com/manifoldco/torus-cli/identity"
)

// ClaimedKeyPair contains a public/private keypair, and all the Claims made
// against it (system and user signatures).
type ClaimedKeyPair struct {
	apitypes.PublicKeySegment
	PrivateKey *envelope.PrivateKey `json:"private_key"`
}

// KeyPairsClient represents the `/keypairs` registry endpoint, used for
// accessing users' signing and encryption keypairs.
type KeyPairsClient struct {
	client RoundTripper
}

// Create creates a new keypair on the registry.
//
// The keypair includes the user's public key, private key, and a self-signed
// claim on the public key.
//
// keys may be either signing or encryption keys.
func (k *KeyPairsClient) Create(ctx context.Context, pubKey *envelope.PublicKey,
	privKey *envelope.PrivateKey, claim *envelope.Claim) (*envelope.PublicKey,
	*envelope.PrivateKey, []envelope.Claim, error) {

	req, err := k.client.NewRequest("POST", "/keypairs", nil,
		ClaimedKeyPair{
			PublicKeySegment: apitypes.PublicKeySegment{
				PublicKey: pubKey,
				Claims:    []envelope.Claim{*claim},
			},
			PrivateKey: privKey,
		})
	if err != nil {
		log.Printf("Error building http request: %s", err)
		return nil, nil, nil, err
	}

	resp := ClaimedKeyPair{}
	_, err = k.client.Do(ctx, req, &resp)
	if err != nil {
		log.Printf("Failed to create signing keypair: %s", err)
		return nil, nil, nil, err
	}

	return resp.PublicKey, resp.PrivateKey, resp.Claims, nil
}

// List returns all KeyPairs for the logged in user in the given, or all orgs
// if orgID is nil.
func (k *KeyPairsClient) List(ctx context.Context, orgID *identity.ID) ([]ClaimedKeyPair, error) {
	query := &url.Values{}
	if orgID != nil {
		query.Set("org_id", orgID.String())
	}

	req, err := k.client.NewRequest("GET", "/keypairs", query, nil)
	if err != nil {
		log.Printf("Error building http request: %s", err)
		return nil, err
	}

	resp := []ClaimedKeyPair{}
	_, err = k.client.Do(ctx, req, &resp)
	if err != nil {
		log.Printf("Failed to retrieve keypairs: %s", err)
		return nil, err
	}

	return resp, nil
}
