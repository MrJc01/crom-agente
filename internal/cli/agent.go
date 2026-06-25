package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crom/crom-agente/internal/config"
	"github.com/crom/crom-agente/internal/config/topology"
	"github.com/spf13/cobra"
)

var (
	agentType         string
	agentExecPath     string
	agentURL          string
	agentDescription  string
	agentSystemPrompt string
	agentArgs         string
	agentToolIDs      string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Gerencia os agentes especialistas configurados na topologia",
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista todos os subagentes especialistas ativos na topologia do workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		topoPath := filepath.Join(workspacePath, ".crom", config.AgentsTopologyFile)
		topo, err := topology.LoadTopology(topoPath)
		if err != nil {
			if os.IsNotExist(err) {
				topo = topology.GetDefaultTopology()
			} else {
				return fmt.Errorf("erro ao carregar topologia: %w", err)
			}
		}

		cmd.Println("═══════════════════════════════════════════════════════════════════════════════════════")
		cmd.Println("  Agentes Especialistas Configurados")
		cmd.Println("═══════════════════════════════════════════════════════════════════════════════════════")
		specs := topo.GetSpecialists()
		if len(specs) == 0 {
			cmd.Println("  Nenhum especialista configurado.")
		} else {
			for _, spec := range specs {
				cmd.Printf("  Nome:        %s\n", spec.Name)
				cmd.Printf("  Tipo:        %s\n", spec.Type)
				if spec.Description != "" {
					cmd.Printf("  Descrição:   %s\n", spec.Description)
				}
				if spec.ExecPath != "" {
					cmd.Printf("  Caminho:     %s\n", spec.ExecPath)
				}
				if len(spec.Args) > 0 {
					cmd.Printf("  Argumentos:  %s\n", strings.Join(spec.Args, " "))
				}
				if spec.URL != "" {
					cmd.Printf("  URL SSE:     %s\n", spec.URL)
				}
				if len(spec.ToolIDs) > 0 {
					cmd.Printf("  Ferramentas: %s\n", strings.Join(spec.ToolIDs, ", "))
				}
				cmd.Println("  -------------------------------------------------------------------------------------")
			}
		}
		cmd.Println("═══════════════════════════════════════════════════════════════════════════════════════")
		return nil
	},
}

var agentAddCmd = &cobra.Command{
	Use:   "add [nome]",
	Short: "Adiciona um novo especialista à topologia local",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		topoPath := filepath.Join(workspacePath, ".crom", config.AgentsTopologyFile)
		_ = os.MkdirAll(filepath.Dir(topoPath), 0755)

		topo, err := topology.LoadTopology(topoPath)
		if err != nil {
			if os.IsNotExist(err) {
				topo = topology.GetDefaultTopology()
			} else {
				return fmt.Errorf("erro ao carregar topologia existente: %w", err)
			}
		}

		// Valida se já existe
		for _, spec := range topo.Specialists {
			if spec.Name == name {
				return fmt.Errorf("já existe um agente com o nome '%s'", name)
			}
		}

		var argList []string
		if agentArgs != "" {
			argList = strings.Split(agentArgs, ",")
			for i := range argList {
				argList[i] = strings.TrimSpace(argList[i])
			}
		}

		var toolsList []string
		if agentToolIDs != "" {
			toolsList = strings.Split(agentToolIDs, ",")
			for i := range toolsList {
				toolsList[i] = strings.TrimSpace(toolsList[i])
			}
		}

		newSpec := topology.SpecialistConfig{
			Name:         name,
			Type:         agentType,
			Description:  agentDescription,
			SystemPrompt: agentSystemPrompt,
			ExecPath:     agentExecPath,
			Args:         argList,
			URL:          agentURL,
			ToolIDs:      toolsList,
		}

		topo.Specialists = append(topo.Specialists, newSpec)
		if err := topology.SaveTopology(topoPath, topo); err != nil {
			return fmt.Errorf("falha ao salvar topologia atualizada: %w", err)
		}

		cmd.Printf("✓ Agente especialista '%s' (tipo: %s) adicionado com sucesso.\n", name, agentType)
		return nil
	},
}

var agentRemoveCmd = &cobra.Command{
	Use:   "remove [nome]",
	Short: "Remove um especialista da topologia local",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		topoPath := filepath.Join(workspacePath, ".crom", config.AgentsTopologyFile)
		topo, err := topology.LoadTopology(topoPath)
		if err != nil {
			if os.IsNotExist(err) {
				topo = topology.GetDefaultTopology()
			} else {
				return fmt.Errorf("erro ao carregar topologia: %w", err)
			}
		}

		found := false
		newSpecs := make([]topology.SpecialistConfig, 0, len(topo.Specialists))
		for _, spec := range topo.Specialists {
			if spec.Name == name {
				found = true
				continue
			}
			newSpecs = append(newSpecs, spec)
		}

		if !found {
			return fmt.Errorf("agente especialista '%s' não encontrado na topologia", name)
		}

		topo.Specialists = newSpecs
		if err := topology.SaveTopology(topoPath, topo); err != nil {
			return fmt.Errorf("falha ao salvar topologia atualizada: %w", err)
		}

		cmd.Printf("✓ Agente especialista '%s' removido com sucesso.\n", name)
		return nil
	},
}

var agentValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Valida a consistência do arquivo de topologia crom_agents.json",
	RunE: func(cmd *cobra.Command, args []string) error {
		topoPath := filepath.Join(workspacePath, ".crom", config.AgentsTopologyFile)
		if _, err := os.Stat(topoPath); os.IsNotExist(err) {
			cmd.Printf("⚠ O arquivo '%s' não existe. Usando topologia fallback padrão.\n", config.AgentsTopologyFile)
			return nil
		}

		topo, err := topology.LoadTopology(topoPath)
		if err != nil {
			return fmt.Errorf("topologia inválida: %w", err)
		}

		if len(topo.Specialists) == 0 {
			cmd.Println("⚠ A topologia não contém nenhum agente especialista configurado.")
		}

		for i, spec := range topo.Specialists {
			if spec.Name == "" {
				return fmt.Errorf("especialista no índice %d não possui campo 'name'", i)
			}
			if spec.Type != "native" && spec.Type != "mcp" && spec.Type != "external" {
				return fmt.Errorf("agente '%s': tipo inválido '%s'. Tipos válidos: native, mcp, external", spec.Name, spec.Type)
			}
			if spec.Type == "external" && spec.ExecPath == "" {
				return fmt.Errorf("agente '%s': tipo 'external' requer 'exec_path' configurado", spec.Name)
			}
			if spec.Type == "mcp" && spec.ExecPath == "" && spec.URL == "" {
				return fmt.Errorf("agente '%s': tipo 'mcp' requer 'exec_path' ou 'url' (SSE) configurado", spec.Name)
			}
		}

		cmd.Println("✓ O arquivo crom_agents.json é semanticamente válido e consistente.")
		return nil
	},
}

func init() {
	agentAddCmd.Flags().StringVar(&agentType, "type", "native", "Tipo do especialista (native|mcp|external)")
	agentAddCmd.Flags().StringVar(&agentExecPath, "exec-path", "", "Caminho executável para tipo external/mcp")
	agentAddCmd.Flags().StringVar(&agentURL, "url", "", "URL de conexão para servidores MCP SSE")
	agentAddCmd.Flags().StringVar(&agentDescription, "description", "", "Descrição sobre o que o especialista faz")
	agentAddCmd.Flags().StringVar(&agentSystemPrompt, "system-prompt", "", "System Prompt guia do especialista")
	agentAddCmd.Flags().StringVar(&agentArgs, "args", "", "Argumentos separados por vírgula para execução")
	agentAddCmd.Flags().StringVar(&agentToolIDs, "tools", "", "IDs de ferramentas adicionais que este especialista necessita separadas por vírgula")

	agentCmd.AddCommand(agentListCmd, agentAddCmd, agentRemoveCmd, agentValidateCmd)
	rootCmd.AddCommand(agentCmd)
}
