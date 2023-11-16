package cobictl

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/catalogfi/cobi/utils"
	"github.com/spf13/cobra"
)

func SetConfig(rpcClient Client) *cobra.Command {

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

			config := utils.Config{}
			if err := json.Unmarshal(configFile, &config); err != nil {
				cobra.CheckErr(fmt.Errorf("failed to unmarshal config file: %w", err))
			}

			resp, err := rpcClient.SetConfig(config)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to set config: %w", err))
			}

			fmt.Println(string(resp))
		}}

	cmd.Flags().StringVar(&configFilePath, "config-file", "", "config file")
	cmd.MarkFlagRequired("config-file")
	return cmd
}
