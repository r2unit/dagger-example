package main

import (
	"context"
	"fmt"
	"os"

	"dagger.io/dagger"
)

func main() {
	ctx := context.Background()

	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		panic(err)
	}
	defer client.Close()

	if err := build(ctx, client); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func build(ctx context.Context, client *dagger.Client) error {
	fmt.Println("Building Docker image with Dagger...")

	src := client.Host().Directory(".", dagger.HostDirectoryOpts{
		Exclude: []string{"ci/", ".git/", ".github/"},
	})

	container := client.Container().
		From("golang:1.21-alpine").
		WithDirectory("/app", src).
		WithWorkdir("/app").
		WithExec([]string{"go", "mod", "download"}).
		WithExec([]string{"go", "build", "-o", "main", "."})

	_, err := container.Stdout(ctx)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	finalImage := client.Container().
		From("alpine:latest").
		WithExec([]string{"apk", "--no-cache", "add", "ca-certificates"}).
		WithFile("/root/main", container.File("/app/main")).
		WithWorkdir("/root").
		WithExposedPort(8080).
		WithEntrypoint([]string{"./main"})

	_, err = finalImage.Sync(ctx)
	if err != nil {
		return fmt.Errorf("final image build failed: %w", err)
	}

	fmt.Println("Build successful!")
	return nil
}
