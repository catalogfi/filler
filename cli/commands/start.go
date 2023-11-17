package commands

import (
	"errors"
	"fmt"

	"github.com/catalogfi/cobi/daemon/rpc/handlers"
	"github.com/catalogfi/cobi/rpcclient"
	"github.com/spf13/cobra"
)

func StartService(rpcClient rpcclient.Client) *cobra.Command {
	var (
		service         handlers.Service
		account         uint32
		isInstantWallet bool
	)
	var cmd = &cobra.Command{
		Use:   "start",
		Short: "starts a service in daemon",
		Run: func(c *cobra.Command, args []string) {
			if service != handlers.Executor && service != handlers.Autofiller && service != handlers.AutoCreator {
				cobra.CheckErr(errors.New("invalid service type"))
			}

			StartService := rpcclient.StartService{
				ServiceType:     service,
				Account:         uint(account),
				IsInstantWallet: isInstantWallet,
			}

			resp, err := rpcClient.StartService(StartService)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			fmt.Println(string(resp))
		}}
	cmd.Flags().Uint32Var(&account, "account", 0, "account to be used (default: 0)")
	cmd.Flags().Var(&service, "service", "allowed: \"executor\", \"autofiller\", \"autocreator\"")
	cmd.MarkFlagRequired("service")
	cmd.Flags().BoolVarP(&isInstantWallet, "isIw", "i", false, "set to run service with instant wallet")
	return cmd
}
