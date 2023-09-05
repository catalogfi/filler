package screener

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	"gorm.io/gorm"
)

const (
	TrmLabUrl = "https://api.trmlabs.com/public/v2/screening/addresses"
)

type AddressScreeningRequest struct {
	Address           string `json:"address"`
	Chain             string `json:"chain"`
	AccountExternalID string `json:"accountExternalId"`
}

type AddressScreeningResponse struct {
	AddressRiskIndicators []AddressRiskIndicator `json:"addressRiskIndicators"`
	Address               string                 `json:"address"`
	AddressSubmitted      string                 `json:"addressSubmitted"`
	Entities              []Entity               `json:"entities"`
}

type AddressRiskIndicator struct {
	Category                    string `json:"category"`
	CategoryID                  string `json:"categoryId"`
	CategoryRiskScoreLevel      int    `json:"categoryRiskScoreLevel"`
	CategoryRiskScoreLevelLabel string `json:"categoryRiskScoreLevelLabel"`
	RiskType                    string `json:"riskType"`
}

type Entity struct {
	Category             string `json:"category"`
	CategoryID           string `json:"categoryId"`
	ConfidenceScoreLabel string `json:"confidenceScoreLabel"`
	Entity               string `json:"entity"`
	RiskScoreLevel       int    `json:"riskScoreLevel"`
	RiskScoreLevelLabel  string `json:"riskScoreLevelLabel"`
}

type screener struct {
	db  *gorm.DB
	key string
}

type Screener interface {
	IsBlacklisted(addrs map[string]model.Chain) (bool, error)
}

func NewScreener(db *gorm.DB, key string) screener {
	screener := screener{
		db:  db,
		key: key,
	}
	return screener
}

func (screener screener) IsBlacklisted(addrs map[string]model.Chain) (bool, error) {

	// If the key is not set, we skip this check. Usually happens to testnet.
	if screener.key == "" {
		return false, nil
	}

	// First check if the address has been blacklisted in the db
	blacklisted, err := screener.isBlacklistedFromDB(addrs)
	if err != nil {
		return false, err
	}
	if blacklisted {
		return true, nil
	}

	// Check against external API
	blacklisted, err = screener.isBlacklistedFromAPI(addrs)
	if err != nil {
		return false, err
	}
	return blacklisted, nil
}

func (screener screener) isBlacklistedFromDB(addrs map[string]model.Chain) (bool, error) {
	if screener.db == nil {
		return false, nil
	}
	addrSlice := make([]string, 0, len(addrs))
	for addr := range addrs {
		addrSlice = append(addrSlice, FormatAddress(addr))
	}

	var blacklist model.Blacklist
	if err := screener.db.Where("address in ?", addrSlice).First(&blacklist).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (screener screener) isBlacklistedFromAPI(addrs map[string]model.Chain) (bool, error) {
	// Generate the request body
	client := new(http.Client)
	requestData := make([]AddressScreeningRequest, 0, len(addrs))
	for addr, chain := range addrs {
		chainIdentifier := trmIdentifier(chain)
		if chainIdentifier == "" {
			continue
		}
		requestData = append(requestData, AddressScreeningRequest{
			Address:           addr,
			Chain:             chainIdentifier,
			AccountExternalID: fmt.Sprintf("%v_%v", chainIdentifier, addr),
		})
	}
	data, err := json.Marshal(requestData)
	if err != nil {
		return false, fmt.Errorf("[screener] unable to marshal request, err = %v", err)
	}
	input := bytes.NewBuffer(data)

	// Construct the request
	request, err := http.NewRequest("POST", TrmLabUrl, input)
	if err != nil {
		return false, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(screener.key, screener.key)

	// Send the request and parse the response
	response, err := client.Do(request)
	if err != nil {
		return false, fmt.Errorf("[screener] error sending request, err = %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		return false, fmt.Errorf("[screener] invalid status code, expect 201, got %v", response.StatusCode)
	}

	// Parse the response
	var resps []AddressScreeningResponse
	if err := json.NewDecoder(response.Body).Decode(&resps); err != nil {
		return false, fmt.Errorf("[screener] unexpected response, %v", err)
	}
	defer response.Body.Close()

	if len(resps) != len(requestData) {
		return false, fmt.Errorf("[screener] invalid number of reponse, expected %v , got %v", len(requestData), len(resps))
	}

	// Check the response
	// 1) when the address has a entity with at least 15 risk score
	// or
	// 2) when the address falls in one of the category has 15 risk score.
	blacklisted := false
Responses:
	for _, resp := range resps {
		for _, entity := range resp.Entities {
			if entity.RiskScoreLevel >= 15 {
				blacklisted = true
				if err := screener.addToDB(resp.AddressSubmitted); err != nil {
					return false, err
				}
				continue Responses
			}
		}
		for _, indicator := range resp.AddressRiskIndicators {
			if indicator.CategoryRiskScoreLevel >= 15 {
				blacklisted = true
				if err := screener.addToDB(resp.AddressSubmitted); err != nil {
					return false, err
				}
				continue Responses
			}
		}
	}

	return blacklisted, nil
}

func (screener screener) addToDB(addr string) error {
	addr = FormatAddress(addr)
	blacklist := model.Blacklist{
		Address: addr,
	}
	return screener.db.Create(&blacklist).Error
}

func trmIdentifier(chain model.Chain) string {
	switch chain {
	case model.Bitcoin:
		return "bitcoin"
	case model.Ethereum:
		return "ethereum"
	default:
		return ""
	}
}

func FormatAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimPrefix(addr, "0x")
	addr = strings.ToLower(addr)
	addr = strings.TrimSpace(addr)
	return addr
}
