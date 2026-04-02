package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// discoverSkills scans the bjornjee-skills plugin cache for skill names.
// It finds the latest version directory, then lists subdirectories under skills/.
// Returns nil if the directory doesn't exist or contains no skills.
func discoverSkills(pluginCacheDir string) []string {
	versionsDir := filepath.Join(pluginCacheDir, "bjornjee-skills", "skills")
	versions, err := os.ReadDir(versionsDir)
	if err != nil {
		return nil
	}

	// Collect version directory names and pick the latest (lexicographic sort
	// works for semver when zero-padded, but we sort properly below).
	var versionNames []string
	for _, v := range versions {
		if v.IsDir() {
			versionNames = append(versionNames, v.Name())
		}
	}
	if len(versionNames) == 0 {
		return nil
	}
	sort.Slice(versionNames, func(i, j int) bool {
		return compareSemver(versionNames[i], versionNames[j]) < 0
	})
	latest := versionNames[len(versionNames)-1]

	skillsDir := filepath.Join(versionsDir, latest, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	var skills []string
	for _, e := range entries {
		if e.IsDir() {
			skills = append(skills, e.Name())
		}
	}
	sort.Strings(skills)
	if len(skills) == 0 {
		return nil
	}
	return skills
}

// buildSkillList returns a display list with "(none)" prepended to the skills.
func buildSkillList(skills []string) []string {
	if len(skills) == 0 {
		return nil
	}
	list := make([]string, 0, len(skills)+1)
	list = append(list, "(none)")
	list = append(list, skills...)
	return list
}

// compareSemver compares two semver-like strings (e.g. "0.9.0" vs "0.22.1").
// Returns -1, 0, or 1.
func compareSemver(a, b string) int {
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	maxLen := len(ap)
	if len(bp) > maxLen {
		maxLen = len(bp)
	}
	for i := 0; i < maxLen; i++ {
		var ai, bi int
		if i < len(ap) {
			ai = atoiSafe(ap[i])
		}
		if i < len(bp) {
			bi = atoiSafe(bp[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// atoiSafe converts a string to int, returning 0 on failure.
func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
