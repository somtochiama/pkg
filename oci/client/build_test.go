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
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluxcd/pkg/tar"
	. "github.com/onsi/gomega"
)

func TestBuild(t *testing.T) {
	g := NewWithT(t)
	c := NewLocalClient()

	absPath := fmt.Sprintf("%s/deployment.yaml", t.TempDir())
	err := copyFile(absPath, "testdata/artifact/deployment.yaml")
	g.Expect(err).To(BeNil())

	tests := []struct {
		name       string
		path       string
		testDir    string
		ignorePath []string
		expectErr  bool
		checkPaths []string
	}{
		{
			name:      "non-existent path",
			path:      "testdata/non-existent",
			expectErr: true,
		},
		{
			name:       "existing path",
			path:       "testdata/artifact",
			ignorePath: []string{"ignore.txt", "ignore-dir/", "!/deploy", "somedir/git"},
			checkPaths: []string{"ignore.txt", "ignore-dir/", "!/deploy", "somedir/git"},
		},
		{
			name:       "existing path with leading slash",
			path:       "./testdata/artifact",
			ignorePath: []string{"ignore.txt", "ignore-dir/", "!/deploy", "somedir/git"},
			checkPaths: []string{"ignore.txt", "ignore-dir/", "!/deploy", "somedir/git"},
		},
		{
			name:       "current directory",
			path:       ".",
			ignorePath: []string{"/*", "!/internal"},
			checkPaths: []string{"/testdata", "!internal/", "build.go", "meta.go"},
		},
		{
			name:       "relative file path",
			path:       "testdata/artifact/deployment.yaml",
			testDir:    "./",
			checkPaths: []string{"!deployment.yaml"},
		},
		{
			name:       "absolute file path",
			path:       absPath,
			testDir:    "./",
			checkPaths: []string{"!deployment.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			tmpDir := t.TempDir()
			artifactPath := filepath.Join(tmpDir, "files.tar.gz")

			err := c.Build(artifactPath, tt.path, tt.ignorePath)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).To(Not(HaveOccurred()))

			_, err = os.Stat(artifactPath)
			g.Expect(err).ToNot(HaveOccurred())

			b, err := os.ReadFile(artifactPath)
			g.Expect(err).ToNot(HaveOccurred())

			untarDir := t.TempDir()
			err = tar.Untar(bytes.NewReader(b), untarDir, tar.WithMaxUntarSize(-1))
			g.Expect(err).To(BeNil())

			testDir := tt.path
			if tt.testDir != "" {
				testDir = tt.testDir
			}
			checkPathExists(t, untarDir, testDir, tt.checkPaths)
		})
	}
}

func checkPathExists(t *testing.T, dir, testDir string, paths []string) {
	g := NewWithT(t)

	for _, path := range paths {
		var shouldExist bool
		if strings.HasPrefix(path, "!") {
			shouldExist = true
			path = path[1:]
		}

		fullPath := filepath.Join(dir, testDir, path)
		_, err := os.Stat(fullPath)
		if shouldExist {
			g.Expect(err).To(BeNil())
			continue
		}
		g.Expect(err).ToNot(BeNil())
		g.Expect(os.IsNotExist(err)).To(BeTrue())
	}
}

func copyFile(dst, src string) error {
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("unable to create file: %w", err)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	_, err = io.Copy(f, source)
	return err
}
