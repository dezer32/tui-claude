package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultClaudeDir returns the default Claude projects directory.
func DefaultClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// LoadAll loads sessions from all projects in the Claude projects directory.
func LoadAll(claudeDir string) ([]Session, []Project, error) {
	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		return nil, nil, err
	}

	var allSessions []Session
	projectMap := make(map[string]*Project)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		indexPath := filepath.Join(claudeDir, entry.Name(), "sessions-index.json")
		sessions, err := loadIndex(indexPath)
		if err != nil {
			continue // skip broken index files
		}

		for i := range sessions {
			sessions[i].ProjectName = decodeProjectName(entry.Name())

			p, exists := projectMap[sessions[i].ProjectPath]
			if !exists {
				p = &Project{
					Name: sessions[i].ProjectName,
					Path: sessions[i].ProjectPath,
				}
				projectMap[sessions[i].ProjectPath] = p
			}
			p.Sessions = append(p.Sessions, sessions[i])
		}

		allSessions = append(allSessions, sessions...)
	}

	// Sort by modified date descending
	sort.Slice(allSessions, func(i, j int) bool {
		return allSessions[i].Modified.After(allSessions[j].Modified)
	})

	// Build projects list
	var projects []Project
	for _, p := range projectMap {
		projects = append(projects, *p)
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return allSessions, projects, nil
}

func loadIndex(path string) ([]Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var index SessionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}

	return index.Entries, nil
}

// decodeProjectName converts encoded directory name back to readable project name.
// e.g. "-Users-vladislav-k-Code-FxBO-crm" → "crm"
func decodeProjectName(encoded string) string {
	parts := strings.Split(encoded, "-")
	if len(parts) == 0 {
		return encoded
	}
	// Return the last non-empty segment
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return encoded
}

// LoadArchived loads archived sessions from all projects in the Claude projects directory.
func LoadArchived(claudeDir string) ([]Session, []Project, error) {
	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		return nil, nil, err
	}

	var allSessions []Session
	projectMap := make(map[string]*Project)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(claudeDir, entry.Name())
		archiveIndexFile := filepath.Join(projectDir, "archive-index.json")

		var sessions []Session

		// Try loading from archive-index.json first
		indexed, err := loadIndex(archiveIndexFile)
		if err == nil {
			sessions = indexed
		}

		// Fallback: scan archive/ directory for files not in the index
		scanned := scanArchiveDir(projectDir, sessions)
		sessions = append(sessions, scanned...)

		for i := range sessions {
			sessions[i].ProjectName = decodeProjectName(entry.Name())
			sessions[i].IsArchived = true

			p, exists := projectMap[sessions[i].ProjectPath]
			if !exists {
				p = &Project{
					Name: sessions[i].ProjectName,
					Path: sessions[i].ProjectPath,
				}
				projectMap[sessions[i].ProjectPath] = p
			}
			p.Sessions = append(p.Sessions, sessions[i])
		}

		allSessions = append(allSessions, sessions...)
	}

	sort.Slice(allSessions, func(i, j int) bool {
		return allSessions[i].Modified.After(allSessions[j].Modified)
	})

	var projects []Project
	for _, p := range projectMap {
		projects = append(projects, *p)
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	return allSessions, projects, nil
}

// scanArchiveDir finds archived JSONL files not already present in the index.
func scanArchiveDir(projectDir string, indexed []Session) []Session {
	archiveDir := filepath.Join(projectDir, "archive")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return nil
	}

	// Build set of known session IDs from index
	known := make(map[string]bool)
	for _, s := range indexed {
		known[s.SessionID] = true
	}

	// Try to get ProjectPath from sessions-index.json
	projectPath := ""
	if active, err := loadIndex(filepath.Join(projectDir, "sessions-index.json")); err == nil && len(active) > 0 {
		projectPath = active[0].ProjectPath
	}
	projectName := decodeProjectName(filepath.Base(projectDir))

	var sessions []Session
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		sessionID := strings.TrimSuffix(name, ".jsonl")
		if known[sessionID] {
			continue
		}

		fullPath := filepath.Join(archiveDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		firstPrompt, summary, msgCount := ExtractMeta(fullPath)

		sessions = append(sessions, Session{
			SessionID:   sessionID,
			FullPath:    fullPath,
			Modified:    info.ModTime(),
			Created:     info.ModTime(),
			FirstPrompt: firstPrompt,
			Summary:     summary,
			MsgCount:    msgCount,
			ProjectName: projectName,
			ProjectPath: projectPath,
			IsArchived:  true,
		})
	}

	return sessions
}

// SortSessions sorts sessions by the given field.
func SortSessions(sessions []Session, field SortField) {
	switch field {
	case SortByDate:
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].Modified.After(sessions[j].Modified)
		})
	case SortByProject:
		sort.Slice(sessions, func(i, j int) bool {
			if sessions[i].ProjectName == sessions[j].ProjectName {
				return sessions[i].Modified.After(sessions[j].Modified)
			}
			return sessions[i].ProjectName < sessions[j].ProjectName
		})
	case SortByMessages:
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].MsgCount > sessions[j].MsgCount
		})
	}
}

// FilterByWorkDir returns sessions matching the given working directory.
// Resolves symlinks for robust comparison on macOS.
func FilterByWorkDir(sessions []Session, workDir string) []Session {
	var filtered []Session
	for _, s := range sessions {
		if cleanPath(s.ProjectPath) == workDir {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func cleanPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(p)
}

// FilterByProject returns sessions matching the given project path.
func FilterByProject(sessions []Session, projectPath string) []Session {
	if projectPath == "" {
		return sessions
	}
	var filtered []Session
	for _, s := range sessions {
		if s.ProjectPath == projectPath {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
