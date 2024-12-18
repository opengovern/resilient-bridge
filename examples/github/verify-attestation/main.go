package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/slsa-framework/slsa-verifier/v2/options"
	"github.com/slsa-framework/slsa-verifier/v2/verifiers"
	_ "github.com/slsa-framework/slsa-verifier/v2/verifiers/gha" // register GitHub Actions verifier
)

func main() {
	ctx := context.Background()

	// Image URI, e.g.: "ghcr.io/anchore/syft:v1.18.1-arm64v8"
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <image-ref>\n", os.Args[0])
	}
	imageRefStr := os.Args[1]

	ref, err := name.ParseReference(imageRefStr)
	if err != nil {
		log.Fatalf("invalid image reference %s: %v", imageRefStr, err)
	}

	// Set up auth if GITHUB_TOKEN is provided.
	// For public images, this may not be necessary.
	var remoteOpts []remote.Option
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		remoteOpts = append(remoteOpts, remote.WithAuth(&authn.Basic{
			Username: "oauth2",
			Password: token,
		}))
	}

	// Use remote.Head to resolve the descriptor, which includes the digest.
	desc, err := remote.Head(ref, remoteOpts...)
	if err != nil {
		log.Fatalf("failed to retrieve image descriptor for %s: %v", ref, err)
	}

	// Construct the immutable reference using the digest.
	immutableRef := ref.Context().Name() + "@" + desc.Digest.String()
	fmt.Printf("Resolved immutable image reference: %s\n", immutableRef)

	imgRef, err := name.ParseReference(immutableRef)
	if err != nil {
		log.Fatalf("could not parse immutable image reference: %v", err)
	}

	// Prepare SLSA verification options.
	// Adjust SourceURI and SourceTag as appropriate for your image.
	// For ghcr.io/anchore/syft:v1.18.1-arm64v8, the source repo is github.com/anchore/syft.
	opts := &options.ImageVerifyOptions{
		ImageRef:        imgRef,
		SourceURI:       "github.com/anchore/syft",
		SourceTag:       "v1.18.1-arm64v8",
		PrintProvenance: true,
	}

	// Perform the provenance verification.
	st, err := verifiers.VerifyImageProvenance(ctx, opts)
	if err != nil {
		log.Fatalf("image provenance verification failed: %v", err)
	}

	fmt.Println("SLSA provenance verification completed successfully.")
	fmt.Printf("Verified Statement Predicate Type: %s\n", st.PredicateType)
}
