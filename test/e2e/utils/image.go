package e2e_utils

import (
    "context"
    "fmt"
    "os"
    "strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
)

func PrepareImage(ctx context.Context, imageENV string) string {
	if v, ok := os.LookupEnv(imageENV); ok {
		return v
	}

	image, err := random.Image(1024, 8)
	if err != nil {
		panic(err.Error())
	}

	targetImageName := fmt.Sprintf("ttl.sh/%s:15m", uuid.New().String())
	ref, err := name.ParseReference(targetImageName)
	if err != nil {
		panic(err.Error())
	}

	pusher, err := remote.NewPusher()
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
