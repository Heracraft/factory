package secrets

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

// AzureKV wraps DEKs with an RSA key in Azure Key Vault using RSA-OAEP-256
// (DECISIONS R3-10). Credentials come from the environment through
// azidentity (client secret or certificate, or a managed identity).
type AzureKV struct {
	client  *azkeys.Client
	keyName string
}

// NewAzureKV connects to the vault at vaultURL and uses keyName.
func NewAzureKV(vaultURL, keyName string, cred azcore.TokenCredential) (*AzureKV, error) {
	if cred == nil {
		c, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("azure credential: %w", err)
		}
		cred = c
	}
	client, err := azkeys.NewClient(vaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("key vault client: %w", err)
	}
	return &AzureKV{client: client, keyName: keyName}, nil
}

// CurrentVersion implements KeyVault.
func (a *AzureKV) CurrentVersion(ctx context.Context) (string, error) {
	resp, err := a.client.GetKey(ctx, a.keyName, "", nil)
	if err != nil {
		return "", err
	}
	if resp.Key.KID == nil {
		return "", fmt.Errorf("key vault: key %s has no id", a.keyName)
	}
	return resp.Key.KID.Version(), nil
}

// Wrap implements KeyVault.
func (a *AzureKV) Wrap(ctx context.Context, dek []byte) ([]byte, string, error) {
	version, err := a.CurrentVersion(ctx)
	if err != nil {
		return nil, "", err
	}
	alg := azkeys.EncryptionAlgorithmRSAOAEP256
	resp, err := a.client.WrapKey(ctx, a.keyName, version, azkeys.KeyOperationParameters{Algorithm: &alg, Value: dek}, nil)
	if err != nil {
		return nil, "", err
	}
	return resp.Result, version, nil
}

// Unwrap implements KeyVault.
func (a *AzureKV) Unwrap(ctx context.Context, wrapped []byte, version string) ([]byte, error) {
	alg := azkeys.EncryptionAlgorithmRSAOAEP256
	resp, err := a.client.UnwrapKey(ctx, a.keyName, version, azkeys.KeyOperationParameters{Algorithm: &alg, Value: wrapped}, nil)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}
