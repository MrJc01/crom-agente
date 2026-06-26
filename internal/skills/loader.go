// Package skills permite carregar arquivos .crom que definem regras, contextos e ferramentas
// específicas por framework (Vue, Go, Django, React, etc.).
//
// Cada arquivo .crom é um JSON com a seguinte estrutura:
//
//	{
//	  "name": "go-backend",
//	  "description": "Regras para projetos Go com Gin/Echo",
//	  "system_prompt_addon": "Você é um especialista em Go...",
//	  "preferred_tools": ["terminal_command", "grep_ast"],
//	  "file_patterns": ["*.go", "go.mod"],
//	  "rules": [
//	    "Sempre rode go vet antes de finalizar",
//	    "Use tabelas de teste (table-driven tests)"
//	  ]
//	}
package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill representa um conjunto de regras carregadas de um arquivo .crom
type Skill struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	SystemPromptAddon string   `json:"system_prompt_addon"`
	PreferredTools    []string `json:"preferred_tools"`
	FilePatterns      []string `json:"file_patterns"`
	Rules             []string `json:"rules"`
}

// LoadSkillsFromDir carrega todos os arquivos .crom de um diretório
func LoadSkillsFromDir(dir string) ([]Skill, error) {
	var skills []Skill

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Sem skills, sem erro
		}
		return nil, fmt.Errorf("falha ao ler diretório de skills: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".crom") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("falha ao ler skill %s: %w", entry.Name(), err)
		}

		var skill Skill
		if err := json.Unmarshal(data, &skill); err != nil {
			return nil, fmt.Errorf("falha ao parsear skill %s: %w", entry.Name(), err)
		}

		skills = append(skills, skill)
	}

	return skills, nil
}

// LoadSkillsForWorkspace carrega skills do diretório .crom/skills/ dentro do workspace
func LoadSkillsForWorkspace(workspacePath string) ([]Skill, error) {
	skillsDir := filepath.Join(workspacePath, ".crom", "skills")
	return LoadSkillsFromDir(skillsDir)
}

// MatchSkillsForFile retorna as skills que aplicam a um determinado arquivo
func MatchSkillsForFile(allSkills []Skill, filePath string) []Skill {
	var result []Skill
	for _, s := range allSkills {
		for _, pattern := range s.FilePatterns {
			if ok, _ := filepath.Match(pattern, filepath.Base(filePath)); ok {
				result = append(result, s)
				break
			}
		}
	}
	return result
}

// BuildPromptAddon constrói a string de adendo ao system prompt a partir das skills carregadas
func BuildPromptAddon(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Regras de Projeto (Skills Carregadas)\n")
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("\n### %s\n", s.Name))
		if s.Description != "" {
			sb.WriteString(s.Description + "\n")
		}
		if s.SystemPromptAddon != "" {
			sb.WriteString(s.SystemPromptAddon + "\n")
		}
		for _, rule := range s.Rules {
			sb.WriteString(fmt.Sprintf("- %s\n", rule))
		}
	}
	return sb.String()
}
