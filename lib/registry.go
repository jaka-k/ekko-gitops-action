package lib

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

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

var DefaultTransport http.RoundTripper = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,

	MaxIdleConnsPerHost: 50,
}

// ---------------------------------------------------------------------------
// pkg/name — parsing and building image references
// ---------------------------------------------------------------------------

// BuildReferences showcases the strict constructors name.NewTag and
// name.NewDigest, plus the accessors on the resulting reference:
// Context() (the repository), Identifier() (tag or digest part), and Name()
// (fully qualified form).
func BuildReferences() error {
	tag, err := name.NewTag("myorg/myapp:dev-abc1234")
	if err != nil {
		return fmt.Errorf("parsing tag: %w", err)
	}
	fmt.Println(tag.Context())    // index.docker.io/myorg/myapp
	fmt.Println(tag.Identifier()) // dev-abc1234
	fmt.Println(tag.Name())       // index.docker.io/myorg/myapp:dev-abc1234

	digest, err := name.NewDigest("myorg/myapp@sha256:deadbeef" +
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err != nil {
		return fmt.Errorf("parsing digest: %w", err)
	}
	fmt.Println(digest.DigestStr())

	// Options change the normalization defaults, e.g. for a private mirror.
	_, err = name.ParseReference("myorg/myapp", name.WithDefaultRegistry("registry.example.com"))
	return err
}

// KeychainAuth showcases authn.DefaultKeychain, which resolves credentials
// the way the docker CLI does (~/.docker/config.json, credential helpers).
// Useful locally after `docker login`; in the action we use explicit inputs.
func KeychainAuth(ref name.Reference) (authn.Authenticator, error) {
	return authn.DefaultKeychain.Resolve(ref.Context())
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

// RetagImage showcases crane.Copy: copy src to dst entirely server-side —
// blobs already present in the destination repo are skipped, so promoting
// dev → prod never pulls image layers to the runner. This is the core of the
// retag-image subcommand. crane.WithContext threads cancellation through, and
// crane.WithTransport shows where a custom transport (e.g. DefaultTransport
// above) plugs in.
func RetagImage(ctx context.Context, src, dst string, auth authn.Authenticator) error {
	return crane.Copy(src, dst,
		crane.WithAuth(auth),
		crane.WithContext(ctx),
		crane.WithTransport(DefaultTransport),
	)
}

// AddTag showcases crane.Tag: point an additional tag at an existing image in
// the same repository (e.g. also tag a release as "latest"). Only a manifest
// PUT — no blobs move at all.
func AddTag(image, newTag string, auth authn.Authenticator) error {
	return crane.Tag(image, newTag, crane.WithAuth(auth))
}

// PullAndInspect showcases crane.Pull, which fetches a full v1.Image, and the
// v1.Image accessors (Digest, ConfigFile). crane.WithPlatform selects one
// image out of a multi-arch index. Only needed when we must look inside an
// image — registry-level work should use Digest/Head/Copy instead.
func PullAndInspect(image string, auth authn.Authenticator) error {
	img, err := crane.Pull(image,
		crane.WithAuth(auth),
		crane.WithPlatform(&v1.Platform{OS: "linux", Architecture: "amd64"}),
	)
	if err != nil {
		return fmt.Errorf("pulling %s: %w", image, err)
	}

	digest, err := img.Digest()
	if err != nil {
		return err
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		return err
	}
	fmt.Printf("%s built %s for %s/%s\n", digest, cfg.Created, cfg.OS, cfg.Architecture)

	// The counterpart crane.Push(img, dst, ...) uploads a v1.Image.
	return nil
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
