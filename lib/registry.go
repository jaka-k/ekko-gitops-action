package lib

import (
	"context"
	"fmt"
	"net/http/httptest"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ---------------------------------------------------------------------------
// pkg/authn — registry credentials
// ---------------------------------------------------------------------------

// RegistryAuth showcases authn.FromConfig, which wraps static credentials —
// this is how the action's docker-username / docker-password inputs become an
// authn.Authenticator usable with both crane and remote. For ghcr.io the
// password is the workflow's GITHUB_TOKEN (needs `packages: write`) or a PAT
// with packages scope; the username is not validated (github.actor by
// convention).
func RegistryAuth(username, password string) authn.Authenticator {
	return authn.FromConfig(authn.AuthConfig{
		Username: username,
		Password: password,
	})
}

// ---------------------------------------------------------------------------
// pkg/crane — high-level one-call operations
// ---------------------------------------------------------------------------

// GetDigest showcases crane.Digest: resolve a reference to its sha256 digest
// with a HEAD/GET against the registry, without pulling any layers. This is
// how the action can produce its docker-digest output.
func GetDigest(image string, auth authn.Authenticator) (string, error) {
	return crane.Digest(image, crane.WithAuth(auth))
}

// HeadImage showcases crane.Head: a cheap existence/metadata check returning
// the descriptor (digest, media type, size) via a HEAD request only.
func HeadImage(image string, auth authn.Authenticator) (*v1.Descriptor, error) {
	return crane.Head(image, crane.WithAuth(auth))
}

// RawManifest showcases crane.Manifest, returning the raw manifest bytes —
// either a single image manifest or a multi-arch index.
func RawManifest(image string, auth authn.Authenticator) ([]byte, error) {
	return crane.Manifest(image, crane.WithAuth(auth))
}

// ListTags showcases crane.ListTags, enumerating every tag in a repository.
func ListTags(repository string, auth authn.Authenticator) ([]string, error) {
	return crane.ListTags(repository, crane.WithAuth(auth))
}

// ---------------------------------------------------------------------------
// pkg/v1/remote — descriptor-level access (verify-architecture)
// ---------------------------------------------------------------------------

// ImagePlatforms showcases remote.Get and the descriptor API: fetch the
// manifest, detect whether it is a multi-arch index (MediaType.IsIndex), and
// walk IndexManifest().Manifests to collect each child's v1.Platform. This is
// exactly what the verify-architecture subcommand needs to compare against
// the docker-build-platforms input.
func ImagePlatforms(ctx context.Context, image string, auth authn.Authenticator) ([]string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", image, err)
	}

	desc, err := remote.Get(ref, remote.WithAuth(auth), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", image, err)
	}

	if !desc.MediaType.IsIndex() {
		// Single-platform image: the platform lives in the config file.
		img, err := desc.Image()
		if err != nil {
			return nil, err
		}
		cfg, err := img.ConfigFile()
		if err != nil {
			return nil, err
		}
		return []string{cfg.Platform().String()}, nil
	}

	idx, err := desc.ImageIndex()
	if err != nil {
		return nil, err
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}

	var platforms []string
	for _, m := range manifest.Manifests {
		// Skip attestation manifests (platform "unknown/unknown") that
		// buildx attaches alongside the real images.
		if m.Platform == nil || m.Platform.OS == "unknown" {
			continue
		}
		platforms = append(platforms, m.Platform.String()) // e.g. "linux/amd64"
	}
	return platforms, nil
}

// ---------------------------------------------------------------------------
// pkg/registry — in-memory registry for tests
// ---------------------------------------------------------------------------

// NewTestRegistry showcases registry.New: a compliant in-memory registry as
// an http.Handler. Unit tests for RetagImage/ImagePlatforms can push to and
// read from it without network or credentials. Caller must Close() the server.
func NewTestRegistry() (*httptest.Server, error) {
	srv := httptest.NewServer(registry.New())

	// References inside the test registry are just "<host:port>/<repo>:<tag>".
	ref, err := name.ParseReference(fmt.Sprintf("%s/test/myapp:v1", srv.Listener.Addr()))
	if err != nil {
		srv.Close()
		return nil, err
	}
	fmt.Println("test registry ref:", ref.Name())
	return srv, nil
}
