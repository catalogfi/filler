package cobi

import (
	"crypto/rand"
	"fmt"
	"os"

	"github.com/catalogfi/wbtc-garden/model"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Run(config model.Config) error {
	var cmd = &cobra.Command{
		Use: "COBI - Catalog Order Book clI",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
		DisableAutoGenTag: true,
	}

	entropy, err := readMnemonic()
	if err != nil {
		return err
	}

	store, err := NewStore(sqlite.Open("test.db"), &gorm.Config{})
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

	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}

func readMnemonic() ([]byte, error) {
	// homeDir, err := os.UserHomeDir()
	// if err != nil {
	// 	return nil, err
	// }

	if data, err := os.ReadFile("./cob/MNEMONIC"); err == nil {
		return bip39.EntropyFromMnemonic(string(data))
	}
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

	file, err := os.Create("./cob/MNEMONIC")
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
