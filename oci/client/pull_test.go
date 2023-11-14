/*
Copyright 2023 The Flux authors

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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluxcd/pkg/oci"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	. "github.com/onsi/gomega"
)

func Test_PullAnyTarball(t *testing.T) {
	ctx := context.Background()
	c := NewClient(DefaultOptions())

	repo := "test-no-annotations" + randStringRunes(5)
	tests := []struct {
		name             string
		tag              string
		pushImage        func(url string, path string) error
		contentPath      string
		layerType        LayerType
		expectedMetadata Metadata
	}{
		{
			name:        "tarball artifact not pushed by flux",
			tag:         "not-flux",
			contentPath: "testdata/artifact",
			pushImage: func(url string, path string) error {
				artifact := filepath.Join(t.TempDir(), "artifact.tgz")
				err := build(artifact, path, nil)
				if err != nil {
					return err
				}

				img := mutate.MediaType(empty.Image, types.OCIManifestSchema1)
				img = mutate.ConfigMediaType(img, oci.CanonicalConfigMediaType)

				layer, err := tarball.LayerFromFile(artifact, tarball.WithMediaType("application/vnd.acme.some.content.layer.v1.tar+gzip"))
				if err != nil {
					return err
				}

				img, err = mutate.Append(img, mutate.Addendum{Layer: layer})
				if err != nil {
					return err
				}

				dst := fmt.Sprintf("%s/%s:%s", dockerReg, repo, "not-flux")
				err = crane.Push(img, dst, c.optionsWithContext(ctx)...)
				if err != nil {
					return err
				}
				return err
			},
			layerType: LayerTypeTarball,
		},
		{
			name:        "tarball artifact pushed by flux",
			tag:         "flux",
			contentPath: "testdata/artifact",
			pushImage: func(url string, path string) error {
				meta := Metadata{
					Created: "2023-11-14T21:42:50Z",
					Source:  "fluxcd/pkg",
				}
				_, err := c.Push(ctx, url, path, WithPushMetadata(meta))
				return err
			},
			expectedMetadata: Metadata{
				Created: "2023-11-14T21:42:50Z",
				Source:  "fluxcd/pkg",
			},
			layerType: LayerTypeTarball,
		},
		{
			name:        "static artifact pushed by flux",
			tag:         "flux",
			contentPath: "testdata/artifact/deployment.yaml",
			pushImage: func(url string, path string) error {
				meta := Metadata{
					Created: "2023-11-14T21:42:50Z",
					Source:  "fluxcd/pkg",
				}
				_, err := c.Push(ctx, url, path, WithPushLayerType(LayerTypeStatic), WithPushMetadata(meta))
				return err
			},
			expectedMetadata: Metadata{
				Created: "2023-11-14T21:42:50Z",
				Source:  "fluxcd/pkg",
			},
			layerType: LayerTypeStatic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			dst := fmt.Sprintf("%s/%s:%s", dockerReg, repo, tt.tag)
			g.Expect(tt.pushImage(dst, tt.contentPath)).To(Succeed())

			extractTo := filepath.Join(t.TempDir(), "artifact")

			m, err := c.Pull(ctx, dst, extractTo)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(m).ToNot(BeNil())
			g.Expect(m.ToAnnotations()).To(Equal(tt.expectedMetadata.ToAnnotations()))
			g.Expect(m.URL).To(Equal(dst))
			g.Expect(m.Digest).ToNot(BeEmpty())

			switch tt.layerType {
			case LayerTypeTarball:
				g.Expect(extractTo).To(BeADirectory())
				for _, entry := range []string{
					"deploy",
					"deploy/repo.yaml",
					"deployment.yaml",
					"ignore-dir",
					"ignore-dir/deployment.yaml",
					"ignore.txt",
					"somedir",
					"somedir/repo.yaml",
					"somedir/git/repo.yaml",
				} {
					g.Expect(extractTo + "/" + entry).To(Or(BeAnExistingFile(), BeADirectory()))
				}
			case LayerTypeStatic:
				g.Expect(extractTo).To(BeARegularFile())

				expected, err := os.ReadFile(tt.contentPath)
				g.Expect(err).ToNot(HaveOccurred())

				got, err := os.ReadFile(extractTo)
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(expected).To(Equal(got))

			}
		})
	}

}
