package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	sourceRoot = "adapters/skills-src"
	claudeRoot = "adapters/claude-code/skills"
	codexRoot  = "adapters/codex/skills"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := os.Stat(sourceRoot); err != nil {
		return fmt.Errorf("source root %s: %w", sourceRoot, err)
	}
	if err := cleanTarget(claudeRoot); err != nil {
		return err
	}
	if err := cleanTarget(codexRoot); err != nil {
		return err
	}
	if err := emitTarget(targetClaude, claudeRoot); err != nil {
		return err
	}
	if err := emitTarget(targetCodex, codexRoot); err != nil {
		return err
	}
	return nil
}

func cleanTarget(root string) error {
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("clean %s: %w", root, err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", root, err)
	}
	return nil
}

func emitTarget(tgt target, targetRoot string) error {
	return filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", path, err)
		}
		if rel == "." {
			return nil
		}

		targetPath := filepath.Join(targetRoot, rel)
		if entry.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create directory %s: %w", targetPath, err)
			}
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		if filepath.Ext(path) == ".md" {
			content, err = transform(content, tgt)
			if err != nil {
				return fmt.Errorf("transform %s for %s: %w", path, tgt, err)
			}
		}

		if err := os.WriteFile(targetPath, content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", targetPath, err)
		}
		return nil
	})
}
