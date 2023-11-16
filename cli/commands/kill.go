package commands

import (
	"errors"
	"fmt"

	"github.com/catalogfi/cobi/cobid/handlers"
	"github.com/catalogfi/cobi/rpcclient"
	"github.com/spf13/cobra"
)

func KillService(rpcClient rpcclient.Client) *cobra.Command {
	var (
		service handlers.Service
		account uint32
	)
	var cmd = &cobra.Command{
		Use:   "kill",
		Short: "kills a running service in daemon",
		Run: func(c *cobra.Command, args []string) {
			if service != handlers.Executor && service != handlers.Autofiller && service != handlers.AutoCreator {
				cobra.CheckErr(errors.New("invalid service type"))
			}

			KillService := handlers.KillSerivce{
				ServiceType: service,
				Account:     uint(account),
			}

			resp, err := rpcClient.KillService(KillService)
			if err != nil {
				cobra.CheckErr(fmt.Errorf("failed to send request: %w", err))
			}

			fmt.Println(string(resp))
		}}
	cmd.Flags().Uint32Var(&account, "account", 0, "Account to be used (default: 0)")
	cmd.Flags().Var(&service, "service", "allowed: \"executor\", \"autofiller\", \"autocreator\"")
	cmd.MarkFlagRequired("service")
	return cmd
}
