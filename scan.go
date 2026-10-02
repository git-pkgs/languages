package languages

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
)

type ScanOptions struct {
	Bytes int64 // Zero reads each entire file; positive values limit bytes per file.
	// Exclude receives root-relative paths. Excluding a directory skips its descendants.
	Exclude func(string, fs.DirEntry) bool
}

// Scan summarizes regular files in fsys. Git metadata and symlinks are skipped.
// Use fs.Sub to select a subtree. On an error no partial tree is returned.
//
// Confinement depends on fsys: a file replaced after enumeration is opened
// as-is, and os.DirFS and fs.Sub follow the replacement. For a mutable or
// untrusted directory, pass os.Root.FS, subject to its documented platform
// guarantees.
func Scan(ctx context.Context, fsys fs.FS, options ScanOptions) (*Tree, error) {
	if options.Bytes < 0 {
		return nil, errors.New("bytes must be zero or greater")
	}
	var tree Tree
	var analysis Analysis
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name != "." && (path.Base(name) == ".git" || options.Exclude != nil && options.Exclude(name, entry)) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		size, err := analyzeFile(ctx, fsys, name, options.Bytes, &analysis)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return tree.Add(name, size, &analysis)
	})
	if err != nil {
		return nil, err
	}
	return &tree, nil
}

func analyzeFile(ctx context.Context, fsys fs.FS, name string, limit int64, analysis *Analysis) (int64, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return 0, err
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return 0, statErr
	}
	readErr := analyzeReader(ctx, file, ReadOptions{Bytes: limit, Filename: name}, analysis, info.Size())
	closeErr := file.Close()
	if readErr != nil {
		return 0, readErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	return info.Size(), nil
}
