package cobi

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/jedib0t/go-pretty/table"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Network(config *model.Config, logger *zap.Logger) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "network",
		Short: "Configure supported chains and RPC URLs on COBI",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
		DisableAutoGenTag: true,
	}
	childLogger := logger.With(zap.String("command", "network"))
	cmd.AddCommand(networkAdd(config, childLogger))
	cmd.AddCommand(networkRemove(config, childLogger))
	cmd.AddCommand(networkUpdate(config, childLogger))
	cmd.AddCommand(networkList(config))
	return cmd
}

func networkRemove(config *model.Config, logger *zap.Logger) *cobra.Command {
	var (
		chain string
	)
	var cmd = &cobra.Command{
		Use:   "remove",
		Short: "Remove a chain",
		Run: func(c *cobra.Command, args []string) {
			chain, err := model.ParseChain(chain)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to parse network (%s): %v", chain, err))
			}
			delete(config.RPC, chain)
			if err := writeConfig(config); err != nil {
				cobra.CheckErr(fmt.Errorf("failed to write config to file: %v", err))
			}
			fmt.Printf("successfully removed %s network\n", chain)
		},
	}
	cmd.Flags().StringVar(&chain, "chain", "", "Chain ID")
	cmd.MarkFlagRequired("chain")
	return cmd
}

func networkList(config *model.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "list",
		Short: "List all supported chains",
		Run: func(c *cobra.Command, args []string) {
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"Chain", "RPC URL"})
			rows := make([]table.Row, len(config.RPC))
			i := 0
			for chain, rpc := range config.RPC {
				rows[i] = table.Row{chain, rpc}
				i++
			}
			t.AppendRows(rows)
			t.Render()
		},
	}
	return cmd
}

func networkAdd(config *model.Config, logger *zap.Logger) *cobra.Command {
	var (
		chain string
		rpc   string
	)
	var cmd = &cobra.Command{
		Use:   "add",
		Short: "Add a chain",
		Run: func(c *cobra.Command, args []string) {
			chain, err := model.ParseChain(chain)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to parse network (%s): %v", chain, err))
			}
			oldRPC, ok := config.RPC[chain]
			if ok {
				cobra.CheckErr(fmt.Errorf("network already exists (%s): %v", chain, oldRPC))
			}
			config.RPC[chain] = rpc
			if err := writeConfig(config); err != nil {
				cobra.CheckErr(fmt.Errorf("failed to write config to file: %v", err))
			}
			fmt.Printf("successfully added %s network with RPC %s\n", chain, rpc)
		},
	}
	cmd.Flags().StringVar(&chain, "chain", "", "Chain ID")
	cmd.MarkFlagRequired("chain")
	cmd.Flags().StringVar(&rpc, "rpc", "", "RPC URL")
	cmd.MarkFlagRequired("rpc")
	return cmd
}

func networkUpdate(config *model.Config, logger *zap.Logger) *cobra.Command {
	var (
		chain string
		rpc   string
	)
	var cmd = &cobra.Command{
		Use:   "update",
		Short: "Update a chain's RPC URL",
		Run: func(c *cobra.Command, args []string) {
			chain, err := model.ParseChain(chain)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to parse network (%s): %v", chain, err))
			}
			oldRPC, ok := config.RPC[chain]
			if !ok {
				cobra.CheckErr(fmt.Errorf("network entry does not exist"))
			}
			config.RPC[chain] = rpc
			if err := writeConfig(config); err != nil {
				cobra.CheckErr(fmt.Errorf("failed to write config to file: %v", err))
			}
			fmt.Printf("successfully updated %s network (%s)->(%s)", chain, oldRPC, rpc)
		},
	}
	cmd.Flags().StringVar(&chain, "chain", "", "Chain ID")
	cmd.MarkFlagRequired("chain")
	cmd.Flags().StringVar(&rpc, "rpc", "", "RPC URL")
	cmd.MarkFlagRequired("rpc")
	return cmd
}

func writeConfig(config *model.Config) error {
	val, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("unable to marshal config %s", err)
	}
	if err := os.WriteFile(DefaultConfigPath(), val, 0755); err != nil {
		return fmt.Errorf("unable to write config file %s", err)
	}
	return nil
}
