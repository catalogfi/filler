package cobi

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Run() error {
	var cmd = &cobra.Command{
		Use: "COBI - Catalog Order Book clI",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
		DisableAutoGenTag: true,
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	entropy, err := readMnemonic(homeDir)
	if err != nil {
		return err
	}

	config := LoadConfiguration(fmt.Sprintf("%s/config.json", homeDir))
	if config.RPC == nil {
		config.RPC = map[model.Chain]string{}
	}

	store, err := NewStore(sqlite.Open(fmt.Sprintf("%s/test.db", homeDir)), &gorm.Config{})
	if err != nil {
		return err
	}

	// cmd.AddCommand(Accounts(entropy))
	cmd.AddCommand(Create(entropy, store))
	cmd.AddCommand(Fill(entropy, store))
	cmd.AddCommand(Execute(entropy, store, config))
	cmd.AddCommand(Retry(entropy, store))
	cmd.AddCommand(Accounts(entropy, config))
	cmd.AddCommand(List())
	cmd.AddCommand(AutoFill(entropy, store, config))
	cmd.AddCommand(AutoCreate(entropy, store, config))
	cmd.AddCommand(Network(&config))

	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}

func readMnemonic(homeDir string) ([]byte, error) {
	data, err := os.ReadFile(fmt.Sprintf("%v/.cobi/MNEMONIC", homeDir))
	if err == nil {
		return bip39.EntropyFromMnemonic(string(data))
	}
	fmt.Println("error", err)

	fmt.Println("Generating new mnemonic")
	entropy := [32]byte{}

	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	mnemonic, err := bip39.NewMnemonic(entropy[:])
	if err != nil {
		return nil, err
	}
	fmt.Println(mnemonic)

	file, err := os.Create(fmt.Sprintf("%v/.cobi/MNEMONIC", homeDir))
	if err != nil {
		fmt.Println("error above", err)
		return nil, err
	}
	defer file.Close()

	_, err = file.WriteString(mnemonic)
	if err != nil {
		fmt.Println("error here", err)
		return nil, err
	}
	return entropy[:], nil
}

func LoadConfiguration(file string) model.Config {
	var config model.Config
	configFile, err := os.ReadFile(file)
	if err != nil {
		return model.Config{}
	}
	if err := json.Unmarshal(configFile, &config); err != nil {
		return model.Config{}
	}
	return config
}
