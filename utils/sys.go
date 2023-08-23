package utils

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/fatih/color"
	"github.com/tyler-smith/go-bip39"
)

var HomeDir string

var ErrMnemonicFileMissing = errors.New("mnemonic file missing")

func init() {
	var err error
	HomeDir, err = os.UserHomeDir()
	if err != nil {
		log.Fatal("failed to get $HOME value")
	}
}

func DefaultCobiDirectory() string {
	return filepath.Join(HomeDir, ".cobi")
}

func DefaultMnemonicPath() string {
	return filepath.Join(HomeDir, ".cobi", "MNEMONIC")
}

func DefaultConfigPath() string {
	return filepath.Join(HomeDir, ".cobi", "config.json")
}

func DefaultStrategyPath() string {
	return filepath.Join(HomeDir, ".cobi", "strategy.json")
}

func DefaultStorePath() string {
	return filepath.Join(HomeDir, ".cobi", "data.db")
}

func LoadMnemonic(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrMnemonicFileMissing
		}
		return nil, err
	}
	return bip39.EntropyFromMnemonic(string(data))
}

func NewMnemonic(path string) ([]byte, error) {
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}
	color.Green("Generating new mnemonic:\n[ %v ]", mnemonic)
	return entropy[:], nil
}

func LoadConfigFromFile(file string) model.Config {
	config := model.Config{}
	configFile, err := os.ReadFile(file)
	if err != nil {
		return config
	}
	json.Unmarshal(configFile, &config)
	return config
}

type Config struct {
	Network    model.Config
	Strategies json.RawMessage
	Mnemonic   string
	OrderBook  string
	DB         string
	Sentry     string
}

func LoadExtendedConfig(path string) (Config, error) {
	config := Config{}
	configFile, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(configFile, &config)
	}

	if config.Mnemonic == "" {
		entropy := make([]byte, 32)
		if _, err := rand.Read(entropy[:]); err != nil {
			return config, err
		}
		mnemonic, err := bip39.NewMnemonic(entropy)
		if err != nil {
			return config, err
		}
		config.Mnemonic = string(mnemonic)
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return config, err
		}
		if err := os.WriteFile(path, data, 0755); err != nil {
			return config, err
		}
	}
	return config, nil
}
