package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[1:])
		}
	}
	return p
}

func realPath(p string) (string, error) {
	abs, err := filepath.Abs(expandHome(p))
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// AllowedPath resolves p (following symlinks) and checks that it lies inside
// one of the allowed folders.
func AllowedPath(allowed []string, p string) (string, error) {
	if len(allowed) == 0 {
		return "", fmt.Errorf("no folders are allowed in the config (allowed_folders)")
	}
	real, err := realPath(p)
	if err != nil {
		return "", err
	}
	for _, f := range allowed {
		root, err := realPath(f)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, real)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("path is outside the allowed folders")
}

func filesSearchTool(env *Env) Tool {
	return &funcTool{
		name: "files_search",
		desc: "Ищет файлы по части имени в разрешённых папках.",
		props: map[string]any{
			"query":  str("часть имени файла"),
			"folder": str("папка для поиска, по умолчанию все разрешённые"),
		},
		required: []string{"query"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ Query, Folder string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			roots := env.Config.AllowedFolders
			if a.Folder != "" {
				p, err := AllowedPath(roots, a.Folder)
				if err != nil {
					return "", err
				}
				roots = []string{p}
			} else if len(roots) == 0 {
				return "", fmt.Errorf("no folders are allowed in the config (allowed_folders)")
			}
			q := strings.ToLower(a.Query)
			var found []string
			visited := 0
			for _, r := range roots {
				root, err := realPath(r)
				if err != nil {
					continue
				}
				_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
					if err != nil || ctx.Err() != nil || len(found) >= 20 || visited > 50000 {
						return filepath.SkipAll
					}
					visited++
					if d.IsDir() && path != root && strings.HasPrefix(d.Name(), ".") {
						return filepath.SkipDir
					}
					if !d.IsDir() && strings.Contains(strings.ToLower(d.Name()), q) {
						found = append(found, path)
					}
					return nil
				})
			}
			if len(found) == 0 {
				return "Файлы не найдены.", nil
			}
			return strings.Join(found, "\n"), nil
		},
	}
}

const maxFileRunes = 8000

func fileReadTool(env *Env) Tool {
	return &funcTool{
		name:     "file_read",
		desc:     "Читает текстовый файл из разрешённой папки.",
		props:    map[string]any{"path": str("полный путь к файлу")},
		required: []string{"path"},
		run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct{ Path string }
			if err := decode(args, &a); err != nil {
				return "", err
			}
			p, err := AllowedPath(env.Config.AllowedFolders, a.Path)
			if err != nil {
				return "", err
			}
			st, err := os.Stat(p)
			if err != nil {
				return "", err
			}
			if st.IsDir() {
				return "", fmt.Errorf("this is a folder")
			}
			f, err := os.Open(p)
			if err != nil {
				return "", err
			}
			defer f.Close()
			buf := make([]byte, 64<<10)
			n, _ := f.Read(buf)
			buf = buf[:n]
			if !utf8.Valid(buf) && n == len(buf) {
				// may have cut a rune in half; trim to the last valid boundary
				for len(buf) > 0 && !utf8.Valid(buf) {
					buf = buf[:len(buf)-1]
				}
			}
			if !utf8.Valid(buf) {
				return "", fmt.Errorf("file is not a text file")
			}
			return truncate(string(buf), maxFileRunes), nil
		},
	}
}
