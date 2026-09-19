// Package snapshot streams thin volumes to and from the snapshot store.
// Blob is the store (Azure Blob in production, a directory in tests and
// on a dev host); Streamer is the dd|zstd pipeline (or a byte copy against
// the fake LVM).
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
)

// Blob is a snapshot store.
type Blob interface {
	// Upload stores r at path with metadata and returns the bytes written.
	Upload(ctx context.Context, path string, r io.Reader, meta map[string]string) (uint64, error)
	// Download streams path into w.
	Download(ctx context.Context, path string, w io.Writer) error
	// Exists reports whether path is stored.
	Exists(ctx context.Context, path string) (bool, error)
}

type countingReader struct {
	r io.Reader
	n uint64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += uint64(n)
	return n, err
}

// FileBlob stores blobs under a directory. It is the test backend and the
// break-glass target on a host without Blob credentials.
type FileBlob struct {
	Dir string
}

func (f *FileBlob) full(path string) (string, error) {
	if strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("blob path %q not allowed", path)
	}
	return filepath.Join(f.Dir, filepath.FromSlash(path)), nil
}

// Upload implements Blob.
func (f *FileBlob) Upload(_ context.Context, path string, r io.Reader, meta map[string]string) (uint64, error) {
	p, err := f.full(path)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return 0, err
	}
	tmp := p + ".part"
	w, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	cr := &countingReader{r: r}
	if _, err := io.Copy(w, cr); err != nil {
		_ = w.Close()      // the copy error is what matters
		_ = os.Remove(tmp) // partial upload is discarded
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, p); err != nil {
		return 0, err
	}
	var sb strings.Builder
	for k, v := range meta {
		fmt.Fprintf(&sb, "%s=%s\n", k, v)
	}
	if err := os.WriteFile(p+".meta", []byte(sb.String()), 0o644); err != nil {
		return 0, err
	}
	return cr.n, nil
}

// Download implements Blob.
func (f *FileBlob) Download(_ context.Context, path string, w io.Writer) error {
	p, err := f.full(path)
	if err != nil {
		return err
	}
	r, err := os.Open(p)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }() // read only
	_, err = io.Copy(w, r)
	return err
}

// Exists implements Blob.
func (f *FileBlob) Exists(_ context.Context, path string) (bool, error) {
	p, err := f.full(path)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// AzureBlob uploads to a container with the host's managed identity.
type AzureBlob struct {
	client    *azblob.Client
	container string
}

// NewAzureBlob authenticates with the managed identity (or the default
// credential chain when clientID is empty and no identity is present) and
// targets serviceURL/container.
func NewAzureBlob(serviceURL, container, clientID string) (*AzureBlob, error) {
	var cred azcore.TokenCredential
	var err error
	if clientID != "" {
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{ID: azidentity.ClientID(clientID)})
	} else {
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	}
	if err != nil {
		return nil, fmt.Errorf("blob credential: %w", err)
	}
	c, err := azblob.NewClient(serviceURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("blob client: %w", err)
	}
	return &AzureBlob{client: c, container: container}, nil
}

// Upload implements Blob with block-blob UploadStream, 8 MiB blocks, 4 in
// flight; azcopy is not used because it cannot read from a pipe.
func (a *AzureBlob) Upload(ctx context.Context, path string, r io.Reader, meta map[string]string) (uint64, error) {
	md := map[string]*string{}
	for k, v := range meta {
		v := v
		md[k] = &v
	}
	cr := &countingReader{r: r}
	_, err := a.client.UploadStream(ctx, a.container, path, cr, &azblob.UploadStreamOptions{BlockSize: 8 << 20, Concurrency: 4, Metadata: md})
	if err != nil {
		return cr.n, fmt.Errorf("blob upload: %w", err)
	}
	return cr.n, nil
}

// Download implements Blob.
func (a *AzureBlob) Download(ctx context.Context, path string, w io.Writer) error {
	resp, err := a.client.DownloadStream(ctx, a.container, path, nil)
	if err != nil {
		return fmt.Errorf("blob download: %w", err)
	}
	defer resp.Body.Close()
	_, err = io.Copy(w, resp.Body)
	return err
}

// Exists implements Blob.
func (a *AzureBlob) Exists(ctx context.Context, path string) (bool, error) {
	_, err := a.client.ServiceClient().NewContainerClient(a.container).NewBlobClient(path).GetProperties(ctx, &blob.GetPropertiesOptions{})
	if err != nil {
		var re *azcore.ResponseError
		if errors.As(err, &re) && re.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// MemBlob is an in-memory store for tests.
type MemBlob struct {
	mu    sync.Mutex
	Blobs map[string][]byte
	Meta  map[string]map[string]string
	Fail  error
}

// NewMemBlob returns an empty store.
func NewMemBlob() *MemBlob {
	return &MemBlob{Blobs: map[string][]byte{}, Meta: map[string]map[string]string{}}
}

func (m *MemBlob) Upload(_ context.Context, path string, r io.Reader, meta map[string]string) (uint64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return 0, m.Fail
	}
	m.Blobs[path] = b
	m.Meta[path] = meta
	return uint64(len(b)), nil
}

func (m *MemBlob) Download(_ context.Context, path string, w io.Writer) error {
	m.mu.Lock()
	b, ok := m.Blobs[path]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("blob %s not found", path)
	}
	_, err := w.Write(b)
	return err
}

func (m *MemBlob) Exists(_ context.Context, path string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.Blobs[path]
	return ok, nil
}
