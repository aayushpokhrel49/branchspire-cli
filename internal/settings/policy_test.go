// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.yml")
	policy, err := LoadPolicy(path)
	require.NoError(t, err)
	assert.Equal(t, Policy{}, policy, "a missing policy restricts nothing")
	require.NoError(t, os.WriteFile(path, []byte("extensions: disabled\nextension_hosts: [https://gitlab.com/aayushpokhrel49/branchspire]\nextension_owners: [https://gitlab.com/aayushpokhrel49/branchspire]\nextension_credentials: denied\n"), 0o600))
	policy, err = LoadPolicy(path)
	require.NoError(t, err)
	assert.Equal(t, Policy{Path: path, Extensions: "disabled", ExtensionHosts: []string{"https://gitlab.com/aayushpokhrel49/branchspire"}, ExtensionOwners: []string{"https://gitlab.com/aayushpokhrel49/branchspire"}, ExtensionCredentials: "denied"}, policy)
	for content, want := range map[string]string{
		"extension: disabled\n":                    "field extension not found",
		"extensions: off\n":                        "extensions must be enabled or disabled",
		"extension_credentials: maybe\n":           "extension_credentials must be allowed or denied",
		"extension_hosts: [branchspire.example]\n": "must be a normalized instance URL",
		"extension_owners: [https://gitlab.com/aayushpokhrel49/branchspire/]\n": "use HOST/OWNER",
	} {
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		_, err := LoadPolicy(path)
		require.ErrorContains(t, err, want, content)
	}
}

func TestParseExtensionOwner(t *testing.T) {
	host, owner, err := ParseExtensionOwner("HTTPS://BranchSpire.com:443/gitea")
	require.NoError(t, err)
	assert.Equal(t, "https://gitlab.com/aayushpokhrel49/branchspire", host)
	assert.Equal(t, "gitea", owner)
	host, owner, err = ParseExtensionOwner("https://example.com/git/team")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/git", host)
	assert.Equal(t, "team", owner)
	_, _, err = ParseExtensionOwner("gitea")
	require.Error(t, err)
}
