// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package extensions

import (
	"bufio"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Limits of what an install downloads.
const (
	MaxAssetSize     = 256 << 20
	MaxChecksumsSize = 1 << 20
)

// ChecksumsAsset is the release asset that lists the digests of the other assets.
const ChecksumsAsset = "checksums.txt"

var (
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
)

// ValidDigest reports whether digest is a lowercase hexadecimal SHA-256.
func ValidDigest(digest string) bool { return digestPattern.MatchString(digest) }

// ValidCommit reports whether commit is a full lowercase SHA-1 or SHA-256 commit ID.
func ValidCommit(commit string) bool { return commitPattern.MatchString(commit) }

// FileSHA256 returns the hexadecimal SHA-256 of the file at path.
func FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ChecksumFor returns the digest that checksums, in sha256sum format, lists for asset.
func ChecksumFor(checksums io.Reader, asset string) (string, error) {
	scanner := bufio.NewScanner(io.LimitReader(checksums, MaxChecksumsSize))
	found := ""
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		digest, name, ok := strings.Cut(text, " ")
		// sha256sum marks binary mode with "*" before the name.
		name = strings.TrimPrefix(strings.TrimLeft(name, " "), "*")
		digest = strings.ToLower(digest)
		if !ok || !ValidDigest(digest) || name == "" {
			return "", fmt.Errorf("%s line %d is not in sha256sum format", ChecksumsAsset, line)
		}
		if name != asset {
			continue
		}
		if found != "" && found != digest {
			return "", fmt.Errorf("%s lists %s twice with different digests", ChecksumsAsset, asset)
		}
		found = digest
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%s does not list %s", ChecksumsAsset, asset)
	}
	return found, nil
}

// SelectAsset returns the index of the asset of repo built for goos/goarch, or -1. It prefers one
// named REPO-OS-ARCH or NAME-OS-ARCH, with .exe on Windows, and else takes one whose name ends in
// OS-ARCH, as GitHub CLI extensions name them. On Apple silicon an amd64 build is taken when no
// arm64 one exists, as Rosetta runs it.
func SelectAsset(names []string, repo, name, goos, goarch string) int {
	try := func(arch string) int {
		platform := goos + "-" + arch
		if goos == "windows" {
			platform += ".exe"
		}
		for i, asset := range names {
			if strings.EqualFold(asset, repo+"-"+platform) || strings.EqualFold(asset, name+"-"+platform) {
				return i
			}
		}
		return slices.IndexFunc(names, func(asset string) bool {
			return len(asset) >= len(platform) && strings.EqualFold(asset[len(asset)-len(platform):], platform)
		})
	}
	index := try(goarch)
	if index < 0 && goos == "darwin" && goarch == "arm64" {
		index = try("amd64")
	}
	return index
}

// Older reports whether version is older than minimum by semantic versioning precedence, so a
// pre-release such as 2.0.0-rc1 is older than 2.0.0 and build metadata is ignored. A development
// build or an unparsable version is never older.
func Older(version, minimum string) bool {
	have, havePre, ok := parseVersion(version)
	want, wantPre, wantOK := parseVersion(minimum)
	if !ok || !wantOK {
		return false
	}
	if c := slices.Compare(have[:], want[:]); c != 0 {
		return c < 0
	}
	return comparePrerelease(havePre, wantPre) < 0
}

func parseVersion(version string) (parts [3]int, prerelease string, ok bool) {
	version, _, _ = strings.Cut(strings.TrimPrefix(version, "v"), "+")
	version, prerelease, _ = strings.Cut(version, "-")
	fields := strings.Split(version, ".")
	if len(fields) > 3 {
		return parts, "", false
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return parts, "", false
		}
		parts[i] = n
	}
	return parts, prerelease, true
}

func comparePrerelease(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range min(len(as), len(bs)) {
		an, aErr := strconv.ParseUint(as[i], 10, 64)
		bn, bErr := strconv.ParseUint(bs[i], 10, 64)
		var c int
		switch {
		case aErr == nil && bErr == nil:
			c = cmp.Compare(an, bn)
		case aErr == nil:
			c = -1 // numeric identifiers sort before alphanumeric ones
		case bErr == nil:
			c = 1
		default:
			c = strings.Compare(as[i], bs[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}

// ErrLocalMissing reports a local extension whose executable no longer exists.
var ErrLocalMissing = errors.New("its executable no longer exists")

// CheckLocal refuses to run a local extension's executable at path, after following symbolic
// links, unless it is a regular file that no other user can replace or change.
func CheckLocal(path string) error {
	target, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrLocalMissing, path)
	}
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", target)
	}
	dir, err := os.Stat(filepath.Dir(target))
	if err != nil {
		return err
	}
	return checkAccess(target, info, dir)
}
