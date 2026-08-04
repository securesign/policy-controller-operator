package e2e_utils

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
)

func PrepareImage(ctx context.Context, imageENV string) string {
	if v, ok := os.LookupEnv(imageENV); ok {
		return v
	}

	img, err := random.Image(1024, 8)
	if err != nil {
		panic(err.Error())
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		panic(err.Error())
	}
	cfg.Config.Labels = map[string]string{"quay.expires-after": "2h"}
	image, err := mutate.ConfigFile(img, cfg)
	if err != nil {
		panic(err.Error())
	}

	targetImageName := fmt.Sprintf("quay.io/securesign/e2e-tests:%s", uuid.New().String())
	ref, err := name.ParseReference(targetImageName)
	if err != nil {
		panic(err.Error())
	}

	pusher, err := remote.NewPusher(remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		panic(err.Error())
	}

	err = pusher.Push(ctx, ref, image)
	if err != nil {
		panic(err.Error())
	}
	if err = os.Setenv(imageENV, targetImageName); err != nil {
		panic(err.Error())
	}
	return targetImageName
}

func ImageRepoPrefix(image string) string {
	if idx := strings.IndexAny(image, "@:"); idx != -1 {
		return image[:idx]
	}
	return image
}

