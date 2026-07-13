package adapters

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/wyrd-company/lore/internal/rendering"
	"github.com/wyrd-company/lore/internal/synchronization"
)

type RepositoryOptions struct {
	Options
	Repository string
	Branch     string
}

func Repository(paths []string, options RepositoryOptions) (synchronization.Manifest, error) {
	manifest := newManifest(options.Options, "repository")
	files, err := collectFiles(paths)
	if err != nil {
		return manifest, err
	}
	root := commonRoot(files)
	repository, branch, gitRoot := deriveGit(root)
	if options.Repository != "" {
		repository = options.Repository
	}
	if options.Branch != "" {
		branch = options.Branch
	}
	if gitRoot != "" {
		root = gitRoot
	}
	if repository == "" {
		repository = filepath.Base(root)
	}
	if branch == "" {
		branch = "unknown"
	}

	for _, path := range files {
		source, err := os.ReadFile(path)
		if err != nil {
			return manifest, fmt.Errorf("read repository document %q: %w", path, err)
		}
		if !isText(source) {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(relative, "..") {
			relative = filepath.Base(path)
		}
		relative = filepath.ToSlash(relative)
		extension := strings.ToLower(filepath.Ext(path))
		var rendered rendering.Result
		var renderer string
		switch extension {
		case ".md", ".markdown":
			rendered, err = rendering.Markdown(source)
			renderer = "markdown"
		case ".yml", ".yaml":
			rendered, err = rendering.YAML(source)
			renderer = "yaml"
		default:
			rendered = rendering.Text(source, strings.TrimPrefix(extension, "."))
			renderer = "text"
		}
		if err != nil {
			return manifest, fmt.Errorf("render repository document %q: %w", path, err)
		}
		title := titleFromPath(path)
		if frontMatterTitle, ok := rendered.FrontMatter["title"].(string); ok && frontMatterTitle != "" {
			title = frontMatterTitle
		} else if len(rendered.Headings) > 0 {
			title = rendered.Headings[0].Text
		}
		identity := repository + "@" + branch + ":" + relative
		metadata := map[string]any{"repository": repository, "branch": branch, "path": relative, "format": renderer}
		document, err := makeDocument(identity, title, source, renderer, rendered, metadata, map[string]any{
			"path": path, "repository": repository, "branch": branch,
		}, stringSlice(rendered.FrontMatter["tags"]))
		if err != nil {
			return manifest, err
		}
		manifest.Documents = append(manifest.Documents, document)
	}
	return manifest, manifest.Validate()
}

func collectFiles(paths []string) ([]string, error) {
	seen := make(map[string]struct{})
	for _, requested := range paths {
		absolute, err := filepath.Abs(requested)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			seen[absolute] = struct{}{}
			continue
		}
		err = filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() && (entry.Name() == ".git" || strings.HasPrefix(entry.Name(), ".")) && path != absolute {
				return filepath.SkipDir
			}
			if !entry.IsDir() && entry.Type().IsRegular() {
				seen[path] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func commonRoot(files []string) string {
	if len(files) == 0 {
		return "."
	}
	root := filepath.Dir(files[0])
	for _, file := range files[1:] {
		for {
			relative, err := filepath.Rel(root, file)
			if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				return root
			}
			root = parent
		}
	}
	return root
}

func deriveGit(path string) (repository, branch, root string) {
	root = gitOutput(path, "rev-parse", "--show-toplevel")
	if root == "" {
		return "", "", ""
	}
	repository = gitOutput(root, "config", "--get", "remote.origin.url")
	if repository == "" {
		repository = filepath.Base(root)
	}
	branch = gitOutput(root, "branch", "--show-current")
	if branch == "" {
		branch = gitOutput(root, "rev-parse", "--short", "HEAD")
	}
	return repository, branch, root
}

func gitOutput(directory string, arguments ...string) string {
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func isText(contents []byte) bool {
	return !bytes.Contains(contents, []byte{0}) && utf8.Valid(contents)
}
