package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/rpcclient"
	"github.com/spf13/cobra"
)

func SetConfig(rpcClient rpcclient.Client) *cobra.Command {

	var (
		configFilePath string
	)
	var cmd = &cobra.Command{
		Use:   "set-config",
		Short: "set-config",
		Run: func(c *cobra.Command, args []string) {
			configFile, err := os.ReadFile(configFilePath)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to read config file: %w", err))
			}

			config := types.SetConfig{}
			if err := json.Unmarshal(configFile, &config); err != nil {
				cobra.CheckErr(fmt.Errorf("failed to unmarshal config file: %w", err))
			}

			resp, err := rpcClient.SetConfig(config)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to set config: %w", err))
			}
			rpcClient.UpdateAuth(config.RpcUserName , config.RpcPassword)

			fmt.Println(string(resp))
		}}

	cmd.Flags().StringVar(&configFilePath, "config-file", "", "config file")
	cmd.MarkFlagRequired("config-file")
	return cmd
}
