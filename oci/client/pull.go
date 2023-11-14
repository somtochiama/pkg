/*
Copyright 2022 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/fluxcd/pkg/tar"
)

// PullOptions contains options for pushing a layer.
type PullOptions struct {
	layerType LayerType
}

// PullOption is a function for configuring PushOptions.
type PullOption func(o *PullOptions)

// WithPullLayerType sets the layer type of the layer that is being pulled.
func WithPullLayerType(l LayerType) PullOption {
	return func(o *PullOptions) {
		o.layerType = l
	}
}

// Pull downloads an artifact from an OCI repository and extracts the content.
// It untar or copies the content to the given outPath depending on the layerType.
// If no layer type is given, it tries to determine the right type by checking compressed content of the layer
// for gzip headers
func (c *Client) Pull(ctx context.Context, url, outPath string, opts ...PullOption) (*Metadata, error) {
	o := &PullOptions{}
	for _, opt := range opts {
		opt(o)
	}
	ref, err := name.ParseReference(url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	img, err := crane.Pull(url, c.optionsWithContext(ctx)...)
	if err != nil {
		return nil, err
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("parsing digest failed: %w", err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		return nil, fmt.Errorf("parsing manifest failed: %w", err)
	}

	meta := MetadataFromAnnotations(manifest.Annotations)
	meta.URL = url
	meta.Digest = ref.Context().Digest(digest.String()).String()

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to list layers: %w", err)
	}

	if len(layers) < 1 {
		return nil, fmt.Errorf("no layers found in artifact")
	}

	var blob io.Reader
	blob, err = layers[0].Compressed()
	if err != nil {
		return nil, fmt.Errorf("extracting first layer failed: %w", err)
	}
	if o.layerType == "" {
		bufReader := bufio.NewReader(blob)
		blob = bufReader
		if ok, _ := isGzipBlob(bufReader); ok {
			o.layerType = LayerTypeTarball
		} else {
			o.layerType = LayerTypeStatic
		}
	}

	err = extractLayer(outPath, blob, o.layerType)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// extractLayer extracts the contents of a io.Reader to the given path.
// if the LayerType is LayerTypeTarball, it will untar to a directory,
// if the LayerType is LayerTypeStatic, it will copy to a file.
func extractLayer(path string, blob io.Reader, layerType LayerType) error {
	switch layerType {
	case LayerTypeTarball:
		return tar.Untar(blob, path, tar.WithMaxUntarSize(-1), tar.WithSkipSymlinks())
	case LayerTypeStatic:
		f, err := os.Create(path)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, blob)
		if err != nil {
			return fmt.Errorf("error copying layer content: %s", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported layer type: '%s'", layerType)
	}
}

// Peeker implements a Peek interface that returns the first n bytes without advancing
// some underlying pointer
type Peeker interface {
	Peek(n int) ([]byte, error)
}

// isGzipBlob reads the first two bytes from an io.Reader and
// checks that they are equal to the expected gzip file headers.
func isGzipBlob(buf Peeker) (bool, error) {
	// https://github.com/google/go-containerregistry/blob/a54d64203cffcbf94146e04069aae4a97f228ee2/internal/gzip/zip.go#L28
	var gzipMagicHeader = []byte{'\x1f', '\x8b'}

	b, err := buf.Peek(len(gzipMagicHeader))
	if err != nil {
		if err == io.EOF {
			return false, nil
		}
		return false, err
	}
	return bytes.Equal(b, gzipMagicHeader), nil
}
