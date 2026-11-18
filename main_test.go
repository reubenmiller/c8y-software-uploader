package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckMode(t *testing.T) {
	cases := []struct {
		mode    string
		replace bool
		wantErr bool
	}{
		{mode: "prod", wantErr: true},
		{mode: "PROD", wantErr: true},
		{mode: "qual"},
		{mode: "qual", replace: true, wantErr: true},
		{mode: "dev", replace: true},
		{mode: "ci", replace: true},
	}
	for _, tc := range cases {
		err := checkMode(tc.mode, tc.replace)
		if tc.wantErr {
			assert.Error(t, err, "mode=%s replace=%v", tc.mode, tc.replace)
		} else {
			assert.NoError(t, err, "mode=%s replace=%v", tc.mode, tc.replace)
		}
	}
}

func TestParseTypeMappings(t *testing.T) {
	mappings, err := parseTypeMappings([]string{"*.bin=firmware", "a=b=apt"})
	require.NoError(t, err)
	assert.Equal(t, []TypeMapping{
		{Pattern: "*.bin", SoftwareType: "firmware"},
		{Pattern: "a=b", SoftwareType: "apt"},
	}, mappings)

	for _, invalid := range []string{"firmware", "=firmware", "*.bin="} {
		_, err := parseTypeMappings([]string{invalid})
		assert.Error(t, err, invalid)
	}
}

func TestFindMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a_1.0_arm64.deb", "sub/b-1.0-1.x86_64.rpm", "notes.txt"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
	}

	files, err := findMatchingFiles(dir, []string{"*.deb", "*.rpm", "*.deb"})
	require.NoError(t, err)
	for i := range files {
		files[i], _ = filepath.Rel(dir, files[i])
	}
	sort.Strings(files)
	assert.Equal(t, []string{"a_1.0_arm64.deb", filepath.Join("sub", "b-1.0-1.x86_64.rpm")}, files)

	_, err = findMatchingFiles(dir, []string{"["})
	assert.Error(t, err)
}
