package main

import (
	"context"
	"fmt"
	"os"
	"strings"

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

	registry := os.Getenv("REGISTRY")
	if registry == "" {
		registry = "ghcr.io"
	}

	repoOwner := os.Getenv("GITHUB_REPOSITORY_OWNER")
	repoName := os.Getenv("GITHUB_REPOSITORY")
	if repoName != "" {
		parts := strings.Split(repoName, "/")
		if len(parts) == 2 {
			repoName = parts[1]
		}
	}

	if repoOwner == "" || repoName == "" {
		fmt.Println("Skipping push: GITHUB_REPOSITORY_OWNER or GITHUB_REPOSITORY not set")
		return nil
	}

	imageRef := fmt.Sprintf("%s/%s/%s:latest", registry, strings.ToLower(repoOwner), repoName)

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		fmt.Println("Skipping push: GITHUB_TOKEN not set")
		return nil
	}

	fmt.Printf("Pushing image to %s...\n", imageRef)

	registrySecret := client.SetSecret("github-token", githubToken)

	publishedRef, err := finalImage.
		WithRegistryAuth(registry, repoOwner, registrySecret).
		Publish(ctx, imageRef)

	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	fmt.Printf("Successfully pushed image: %s\n", publishedRef)
	return nil
}
