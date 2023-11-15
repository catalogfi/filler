package cobictl

import (
	"errors"
	"fmt"

	"github.com/catalogfi/cobi/cobid/handlers"
	"github.com/spf13/cobra"
)

type StartPayload struct {
	ServiceType     handlers.Service `json:"service" binding:"required"`
	Account         uint             `json:"userAccount"`
	IsInstantWallet bool             `json:"isInstantWallet"`
}

func StartService(rpcClient Client) *cobra.Command {
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

			StartService := StartPayload{
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
	cmd.Flags().Uint32Var(&account, "account", 0, "Account to be used (default: 0)")
	cmd.Flags().Var(&service, "service", "allowed: \"executor\", \"autofiller\", \"autocreator\"")
	cmd.MarkFlagRequired("service")
	return cmd
}
