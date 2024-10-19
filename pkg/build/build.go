package build

import (
	"context"
	"fmt"
	"io"

	"github.com/yarlson/ftl/pkg/console"
)

type Executor interface {
	RunCommand(ctx context.Context, command string, args ...string) (io.Reader, error)
}

type Build struct {
	executor Executor
}

func NewBuild(executor Executor) *Build {
	return &Build{executor: executor}
}

func (b *Build) Build(ctx context.Context, image string, path string) error {
	err := console.ProgressSpinner(ctx, "Building image", "Image built", []func() error{
		func() error { return b.buildImage(ctx, image, path) },
	})
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	err = console.ProgressSpinner(ctx, "Pushing image", "Image pushed", []func() error{
		func() error { return b.pushImage(ctx, image) },
	})
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	return nil
}

func (b *Build) buildImage(ctx context.Context, image, path string) error {
	_, err := b.executor.RunCommand(ctx, "docker", "build", "-t", image, "--platform", "linux/amd64", path)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	return nil
}

func (b *Build) pushImage(ctx context.Context, image string) error {
	_, err := b.executor.RunCommand(ctx, "docker", "push", image)
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	return nil
}
