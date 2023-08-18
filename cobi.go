package cobi

import (
	"errors"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}

	// Load or create a new mnemonic
	mnemonicPath := DefaultMnemonicPath()
	entropy, err := LoadMnemonic(mnemonicPath)
	if err != nil {
		if errors.Is(err, ErrMnemonicFileMissing) {
			entropy, err = NewMnemonic(mnemonicPath)
			if err != nil {
				return err
			}
		}
		return err
	}

	// Load the config file
	configPath := DefaultConfigPath()
	config := LoadConfigFromFile(configPath)

	// Initialise db
	db := sqlite.Open(filepath.Join(DefaultCobiDirectory(), "data.db"))
	store, err := NewStore(db, &gorm.Config{})
	if err != nil {
		return err
	}

	cmd.AddCommand(Create(entropy, store))
	cmd.AddCommand(Fill(entropy, store))
	cmd.AddCommand(Execute(entropy, store, config, logger))
	cmd.AddCommand(Retry(entropy, store))
	cmd.AddCommand(Accounts(entropy, config))
	cmd.AddCommand(List())
	cmd.AddCommand(Network(&config, logger))
	cmd.AddCommand(Update())

	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}
