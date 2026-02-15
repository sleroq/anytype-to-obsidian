package exportfs

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	anytypedomain "github.com/sleroq/anytype-to-obsidian/internal/domain/anytype"
)

func CopyDir(src, dst string) (int, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read dir %s: %w", src, err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}

	copied := 0
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		inPath := filepath.Join(src, ent.Name())
		outPath := filepath.Join(dst, ent.Name())
		if err := copyFile(inPath, outPath); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, nil
}

func NormalizeExportedFileObjectPaths(inputDir, outputDir string, fileObjects map[string]string) error {
	rewrittenPaths := map[string]string{}
	for _, sourceRelPath := range fileObjects {
		sourceRelPath = filepath.ToSlash(strings.TrimSpace(sourceRelPath))
		if sourceRelPath == "" || filepath.Ext(sourceRelPath) != "" {
			continue
		}
		if _, seen := rewrittenPaths[sourceRelPath]; seen {
			continue
		}

		ext := DetectFileExtensionFromContent(filepath.Join(inputDir, filepath.FromSlash(sourceRelPath)))
		if ext == "" {
			continue
		}
		rewrittenPaths[sourceRelPath] = sourceRelPath + ext
	}

	for sourceRelPath, rewrittenRelPath := range rewrittenPaths {
		sourceAbsPath := filepath.Join(outputDir, filepath.FromSlash(sourceRelPath))
		rewrittenAbsPath := filepath.Join(outputDir, filepath.FromSlash(rewrittenRelPath))

		if _, err := os.Stat(sourceAbsPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat copied file %s: %w", sourceRelPath, err)
		}
		if _, err := os.Stat(rewrittenAbsPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat rewritten file %s: %w", rewrittenRelPath, err)
		}

		if err := os.Rename(sourceAbsPath, rewrittenAbsPath); err != nil {
			return fmt.Errorf("rename copied file %s -> %s: %w", sourceRelPath, rewrittenRelPath, err)
		}
	}

	for objectID, relPath := range fileObjects {
		relPath = filepath.ToSlash(strings.TrimSpace(relPath))
		if rewrittenRelPath, ok := rewrittenPaths[relPath]; ok {
			fileObjects[objectID] = rewrittenRelPath
		}
	}

	return nil
}

func DetectFileExtensionFromContent(path string) string {
	content, err := os.ReadFile(path)
	if err != nil || len(content) == 0 {
		return ""
	}

	sniffLen := len(content)
	if sniffLen > 512 {
		sniffLen = 512
	}

	mimeType := strings.TrimSpace(http.DetectContentType(content[:sniffLen]))
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	mimeType = strings.ToLower(mimeType)
	if mimeType == "" || mimeType == "application/octet-stream" {
		return ""
	}

	preferredExt := map[string]string{
		"image/jpeg":       ".jpg",
		"image/png":        ".png",
		"image/gif":        ".gif",
		"image/webp":       ".webp",
		"image/svg+xml":    ".svg",
		"image/x-icon":     ".ico",
		"application/pdf":  ".pdf",
		"application/json": ".json",
		"text/plain":       ".txt",
	}
	if ext, ok := preferredExt[mimeType]; ok {
		return ext
	}

	exts, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	sort.Strings(exts)
	return exts[0]
}

func ApplyExportedFileTimes(path string, details map[string]any, createdDateKeys []string, changedDateKeys []string, modifiedDateKeys []string, setFileCreationTime func(path string, created time.Time) error) error {
	createdTime, hasCreated := anytypedomain.FirstParsedTimestamp(details, createdDateKeys)
	atime, mtime, ok := anytypedomain.AnytypeTimestamps(details, createdDateKeys, changedDateKeys, modifiedDateKeys)
	if !ok {
		return nil
	}
	if err := os.Chtimes(path, atime, mtime); err != nil {
		return err
	}
	if hasCreated {
		if err := setFileCreationTime(path, createdTime); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
