package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/workato-devs/wk-cli-beta/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server management and testing",
	}
	cmd.AddCommand(newMCPTestCmd())
	cmd.AddCommand(newMCPToolsCmd())
	return cmd
}

func newMCPTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <url>",
		Short: "Test MCP server connectivity",
		Example: `  wk mcp test https://app.workato.com/mcp/abcdef
  wk mcp test https://app.workato.com/mcp/abcdef --json`,
		Args:  requireArgs(1, "server URL is required, e.g.: wk mcp test <url>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			client := mcp.NewClient(args[0])
			info, err := client.Initialize(cmd.Context())
			if err != nil {
				return fmt.Errorf("MCP server test failed: %w", err)
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, info)
			}

			fmt.Fprintf(os.Stdout, "Server:   %s\n", info.Name)
			fmt.Fprintf(os.Stdout, "Version:  %s\n", info.Version)
			fmt.Fprintf(os.Stdout, "Protocol: %s\n", info.ProtocolVersion)
			if len(info.Capabilities) > 0 {
				capJSON, _ := json.Marshal(info.Capabilities)
				fmt.Fprintf(os.Stdout, "Capabilities: %s\n", string(capJSON))
			}
			return nil
		},
	}
}

func newMCPToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <url>",
		Short: "List tools exposed by an MCP server",
		Example: `  wk mcp tools https://app.workato.com/mcp/abcdef
  wk mcp tools https://app.workato.com/mcp/abcdef --json`,
		Args:  requireArgs(1, "server URL is required, e.g.: wk mcp tools <url>"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rctx, err := BuildRunContext(cmd)
			if err != nil {
				return err
			}

			client := mcp.NewClient(args[0])
			if _, err := client.Initialize(cmd.Context()); err != nil {
				return fmt.Errorf("MCP initialize failed: %w", err)
			}

			tools, err := client.ListTools(cmd.Context())
			if err != nil {
				return err
			}

			if flagJSON {
				return rctx.Formatter.Format(os.Stdout, tools)
			}

			headers := []string{"NAME", "DESCRIPTION"}
			var rows [][]string
			for _, t := range tools {
				desc := t.Description
				if len(desc) > 80 {
					desc = desc[:77] + "..."
				}
				rows = append(rows, []string{t.Name, desc})
			}

			fmt.Fprintf(os.Stderr, "%d tools available\n\n", len(tools))
			return rctx.Formatter.FormatList(os.Stdout, headers, rows)
		},
	}
}
