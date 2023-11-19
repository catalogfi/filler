package cli

import (
	"github.com/TheZeroSlave/zapsentry"
	"github.com/catalogfi/cobi/cli/commands"
	"github.com/catalogfi/cobi/rpcclient"
	"github.com/catalogfi/cobi/utils"
	"github.com/getsentry/sentry-go"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Run(version string) error {
	var cmd = &cobra.Command{
		Use: "COBI - Catalog Order Book clI",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
		Version:           version,
		DisableAutoGenTag: true,
	}

	envConfig, err := utils.LoadExtendedConfig(utils.DefaultConfigPath())
	if err != nil {
		return err
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}
	if envConfig.Sentry != "" {
		client, err := sentry.NewClient(sentry.ClientOptions{Dsn: envConfig.Sentry})
		if err != nil {
			return err
		}
		cfg := zapsentry.Configuration{
			Level: zapcore.ErrorLevel,
		}
		core, err := zapsentry.NewCore(cfg, zapsentry.NewSentryClientFromClient(client))
		if err != nil {
			return err
		}
		logger = zapsentry.AttachCoreToLogger(core, logger)
		defer logger.Sync()
	}

	protocol := "https"
	if envConfig.NoTLS {
		protocol = "http"
	}

	rpcClient := rpcclient.NewClient(envConfig.RpcUserName, envConfig.RpcPassword, protocol, envConfig.RPCServer)

	cmd.AddCommand(commands.Create(rpcClient))
	cmd.AddCommand(commands.Fill(rpcClient))
	cmd.AddCommand(commands.Accounts(rpcClient))
	// cmd.AddCommand(Retry(envConfig.OrderBook, keys, envConfig.Network, str, logger, envConfig.DB))
	cmd.AddCommand(commands.List(rpcClient))
	cmd.AddCommand(commands.Deposit(rpcClient))
	cmd.AddCommand(commands.Transfer(rpcClient))
	cmd.AddCommand(commands.SetConfig(rpcClient))
	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}
