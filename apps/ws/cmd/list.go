package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the workstream tree",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		if len(tree.Nodes) == 0 {
			fmt.Println("No workstreams. Create one with: ws add <name> --agent claude")
			return nil
		}

		roots := tree.Roots()
		sort.Slice(roots, func(i, j int) bool {
			return roots[i].ID < roots[j].ID
		})

		for _, root := range roots {
			printNode(tree, root, "", true)
		}
		return nil
	},
}

func printNode(tree *core.Tree, node *core.Node, prefix string, last bool) {
	connector := "├── "
	if last {
		connector = "└── "
	}
	if prefix == "" {
		connector = ""
	}

	status := statusIcon(node.Status)
	agent := ""
	if node.Agent != "" {
		agent = fmt.Sprintf(" [%s]", node.Agent)
	}

	remote := ""
	if node.Type == core.NodeTypeRemote {
		remoteName := node.Workspace
		if remoteName == "" {
			remoteName = node.Host
		}
		remote = fmt.Sprintf(" \033[90m@%s\033[0m", remoteName)
	}

	branchOrDir := node.Branch
	if branchOrDir == "" && node.Type == core.NodeTypeRemote {
		branchOrDir = node.WorkDir
	}

	fmt.Printf("%s%s%s %s%s%s (%s)\n", prefix, connector, status, node.Name, agent, remote, branchOrDir)

	children := tree.Children(node.ID)
	sort.Slice(children, func(i, j int) bool {
		return children[i].ID < children[j].ID
	})

	childPrefix := prefix
	if prefix != "" {
		if last {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range children {
		printNode(tree, child, childPrefix, i == len(children)-1)
	}
}

func statusIcon(s core.NodeStatus) string {
	switch s {
	case core.StatusRunning:
		return strings.TrimSpace("●")
	case core.StatusIdle:
		return "○"
	case core.StatusBlocked:
		return "◌"
	case core.StatusDone:
		return "✓"
	default:
		return "?"
	}
}

func init() {
	rootCmd.AddCommand(listCmd)
}
