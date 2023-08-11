package cobi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/spf13/cobra"
)

func Network(config *model.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "network",
		Short: "Add/Remove networks to COBI",
		Run: func(c *cobra.Command, args []string) {
			switch strings.ToLower(args[0]) {
			case "add":
				chain, err := model.ParseChain(args[1])
				if err != nil {
					cobra.CheckErr(fmt.Errorf("failed to parse network (%s): %v", args[1], err))
				}
				rpc, ok := config.RPC[chain]
				if ok {
					cobra.CheckErr(fmt.Errorf("network already exists (%s): %v", chain, rpc))
				} 
				config.RPC[chain] = args[2]
				fmt.Printf("Successfully added %s network with RPC %s", chain, args[2])
			case "remove":
				chain, err := model.ParseChain(args[1])
				if err != nil {
					cobra.CheckErr(fmt.Errorf("failed to parse network (%s): %v", args[1], err))
				}
				delete(config.RPC, chain)
				fmt.Printf("Successfully removed %s network", chain)
			case "update":
				chain, err := model.ParseChain(args[1])
				if err != nil {
					cobra.CheckErr(fmt.Errorf("failed to parse network (%s): %v", args[1], err))
				}
				fmt.Printf("Successfully updated %s network (%s)->(%s)", chain, config.RPC[chain], args[2])
				config.RPC[chain] = args[2]
			default:
				cobra.CheckErr(fmt.Sprintf("unsupported second command %s", args[0]))
				return
			}
			val, err := json.Marshal(config)
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("unable to marshal config %s", err))
				return
			}

			homeDir, err := os.UserHomeDir()
			if err != nil {
				cobra.CheckErr(fmt.Sprintf("unable to get home directory %s", err))
				return
			}
			if err := os.WriteFile(fmt.Sprintf("%v/.cobi/config.json", homeDir), val, 0755); err != nil {
				cobra.CheckErr(fmt.Sprintf("unable to write config file %s", err))
				return
			}
		},
		DisableAutoGenTag: true,
	}
	return cmd
}
