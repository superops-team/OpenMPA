package usage

import (
	"archive/zip"
	"bytes"
	"errors"
	"gopkg.in/yaml.v3"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxSkillArchive = 20 << 20

var skillPackageName = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
var skillDescriptionMarkup = regexp.MustCompile(`<[^>]+>`)

type skillArchive struct {
	Name, Description string
	Data              []byte
	Files             int
}

var errSkillArchive = errors.New("invalid skill package")

func validateSkillArchive(filename string, data []byte) (skillArchive, error) {
	out := skillArchive{}
	if len(data) == 0 || len(data) > maxSkillArchive || strings.ContainsAny(filename, "/\\\x00") {
		return out, errSkillArchive
	}
	if strings.EqualFold(filename, "SKILL.md") {
		buffer := new(bytes.Buffer)
		archive := zip.NewWriter(buffer)
		file, _ := archive.Create("SKILL.md")
		if _, err := file.Write(data); err != nil {
			return out, errSkillArchive
		}
		if archive.Close() != nil {
			return out, errSkillArchive
		}
		data = buffer.Bytes()
	} else if !strings.HasSuffix(strings.ToLower(filename), ".zip") {
		return out, errSkillArchive
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return out, errSkillArchive
	}
	if len(archive.File) > 1000 {
		return out, errSkillArchive
	}
	files := map[string]*zip.File{}
	seen := map[string]bool{}
	var total uint64
	for _, file := range archive.File {
		name := file.Name
		clean := path.Clean(name)
		mode := file.Mode()
		if !utf8.ValidString(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00:") || len(name) > 512 || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains("/"+name+"/", "/../") || seen[strings.ToLower(clean)] || mode&fs.ModeSymlink != 0 || (!file.FileInfo().IsDir() && !mode.IsRegular()) {
			return out, errSkillArchive
		}
		seen[strings.ToLower(clean)] = true
		if file.FileInfo().IsDir() {
			continue
		}
		if strings.HasPrefix(clean, "__MACOSX/") || path.Base(clean) == ".DS_Store" {
			continue
		}
		total += file.UncompressedSize64
		if total > maxSkillArchive || len(files) >= 100 || (file.CompressedSize64 > 0 && file.UncompressedSize64/file.CompressedSize64 > 200) {
			return out, errSkillArchive
		}
		files[clean] = file
	}
	document := "SKILL.md"
	if files[document] == nil {
		root := ""
		for name := range files {
			parts := strings.Split(name, "/")
			if len(parts) < 2 {
				return out, errSkillArchive
			}
			if root == "" {
				root = parts[0]
			} else if root != parts[0] {
				return out, errSkillArchive
			}
		}
		document = root + "/SKILL.md"
	}
	if files[document] == nil {
		return out, errSkillArchive
	}
	var content []byte
	for name, file := range files {
		reader, e := file.Open()
		if e != nil {
			return out, errSkillArchive
		}
		raw, e := io.ReadAll(io.LimitReader(reader, maxSkillArchive+1))
		reader.Close()
		if e != nil || len(raw) > maxSkillArchive {
			return out, errSkillArchive
		}
		if name == document {
			content = raw
		}
	}
	if !utf8.Valid(content) {
		return out, errSkillArchive
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return out, errSkillArchive
	}
	end := 0
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == 0 {
		return out, errSkillArchive
	}
	var meta map[string]any
	if yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &meta) != nil {
		return out, errSkillArchive
	}
	name, ok := meta["name"].(string)
	if !ok || !skillPackageName.MatchString(name) || strings.Contains(name, "agentkit") {
		return out, errSkillArchive
	}
	description, ok := meta["description"].(string)
	if !ok || strings.TrimSpace(description) == "" || utf8.RuneCountInString(description) > 1024 || skillDescriptionMarkup.MatchString(description) {
		return out, errSkillArchive
	}
	return skillArchive{Name: name, Description: description, Data: data, Files: len(files)}, nil
}
