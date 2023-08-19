package cobi

import (
	"errors"
	"path/filepath"

	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Run(version string) error {
	var (
		orderbook string
	)

	var cmd = &cobra.Command{
		Use: "COBI - Catalog Order Book clI",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
		Version:           version,
		DisableAutoGenTag: true,
	}
	cmd.Flags().StringVar(&orderbook, "orderbook", "production", "url of the orderbook")
	if orderbook == "production" {
		orderbook = ""
	} else if orderbook == "staging" {
		orderbook = ""
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}

	// Load or create a new mnemonic
	mnemonicPath := utils.DefaultMnemonicPath()
	entropy, err := utils.LoadMnemonic(mnemonicPath)
	if err != nil {
		if errors.Is(err, utils.ErrMnemonicFileMissing) {
			entropy, err = utils.NewMnemonic(mnemonicPath)
			if err != nil {
				return err
			}
		}
		return err
	}

	// Load the config file
	configPath := utils.DefaultConfigPath()
	config := utils.LoadConfigFromFile(configPath)

	// Load keys
	keys := utils.NewKeys(entropy)

	// Initialise db
	db := sqlite.Open(filepath.Join(utils.DefaultCobiDirectory(), "data.db"))
	store, err := store.NewStore(db, &gorm.Config{})
	if err != nil {
		return err
	}

	cmd.AddCommand(Create(keys, store))
	cmd.AddCommand(Fill(keys, store))
	cmd.AddCommand(Start(keys, store, config, logger))
	cmd.AddCommand(Retry(store))
	cmd.AddCommand(Accounts(keys, config))
	cmd.AddCommand(List())
	cmd.AddCommand(Network(config, logger))
	cmd.AddCommand(Update())

	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}
