package rest

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/dgrijalva/jwt-go"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spruceid/siwe-go"
)

type Client interface {
	FillOrder(orderID uint, sendAddress, receiveAddress string) error
	CreateOrder(sendAddress, receiveAddress, orderPair, sendAmount, receiveAmount, secretHash string) (uint, error)
	GetOrder(id uint) (model.Order, error)
	GetOrders(filter GetOrdersFilter) ([]model.Order, error)
	GetFollowerInitiateOrders() ([]model.Order, error)
	GetFollowerRedeemOrders() ([]model.Order, error)
	GetInitiatorInitiateOrders() ([]model.Order, error)

	GetFollowerRefundedOrders() ([]model.Order, error)
	FollowerWaitForRedeemOrders() ([]model.Order, error)
	InitiatorWaitForInitiateOrders() ([]model.Order, error)
	GetInitiatorRedeemOrders() ([]model.Order, error)
	GetLockedValue(user string, chain string) (int64, error)
	SetJwt(token string) error
	Health() (string, error)
	GetNonce() (string, error)
	Login() (string, error)
}

type client struct {
	url     string
	privKey string

	JwtToken string
	id       string
}

func NewClient(url, privKey string) Client {
	return &client{url: url, privKey: privKey}
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (c *client) FillOrder(orderID uint, sendAddress, receiveAddress string) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(FillOrder{SendAddress: sendAddress, ReceiveAddress: receiveAddress}); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/orders/%d", c.url, orderID), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.JwtToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fill order: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		if resp.StatusCode == http.StatusUnauthorized {
			c.ReLogin()
			return c.FillOrder(orderID, sendAddress, receiveAddress)
		}
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return fmt.Errorf("failed to decode error response: %v", err)
		}
		return fmt.Errorf("failed to create order: %v", errorResponse.Error)
	}
	return nil
}

func (c *client) CreateOrder(sendAddress, receiveAddress, orderPair, sendAmount, receiveAmount, secretHash string) (uint, error) {
	var buf bytes.Buffer

	fromchain, _, _, _, err := model.ParseOrderPair(orderPair)
	if err != nil {
		return 0, err
	}

	var userBtcAddress string
	if fromchain.IsBTC() {
		userBtcAddress = sendAddress
	} else {
		userBtcAddress = receiveAddress
	}
	if err := json.NewEncoder(&buf).Encode(CreateOrder{SendAddress: sendAddress, ReceiveAddress: receiveAddress, OrderPair: orderPair, SendAmount: sendAmount, ReceiveAmount: receiveAmount, SecretHash: secretHash, UserWalletBTCAddress: userBtcAddress}); err != nil {
		return 0, err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/orders", c.url), &buf)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", c.JwtToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to create order: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return 0, fmt.Errorf("failed to decode error response: %v", err)
		}
		return 0, fmt.Errorf("failed to create order: %v", errorResponse.Error)
	}

	var orderResponse struct {
		ID uint `json:"orderId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&orderResponse); err != nil {
		return 0, fmt.Errorf("failed to decode order response: %v", err)
	}
	return orderResponse.ID, nil
}

func (c *client) GetOrder(id uint) (model.Order, error) {
	resp, err := http.Get(fmt.Sprintf("%s/orders/%d", c.url, id))
	if err != nil {
		return model.Order{}, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return model.Order{}, fmt.Errorf("failed to decode error response: %v", err)
		}
		return model.Order{}, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var order model.Order
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		if resp.ContentLength == 0 {
			return model.Order{}, fmt.Errorf("failed to get order: response body is empty")
		}
		return model.Order{}, fmt.Errorf("failed to decode order: %v", err)
	}
	return order, nil
}

type GetOrdersFilter struct {
	Maker      string
	Taker      string
	OrderPair  string
	SecretHash string
	OrderBy    string
	Verbose    bool
	Status     int
	MinPrice   float64
	MaxPrice   float64
	MinAmount  float64
	MaxAmount  float64
	Page       int
	PerPage    int
}

func appendFilterString(filterString, filterName, filterValue string) string {
	if filterString == "" {
		filterString += "?"
	} else {
		filterString += "&"
	}
	filterString += fmt.Sprintf("%s=%s", filterName, filterValue)
	return filterString
}

func (c *client) GetOrders(filter GetOrdersFilter) ([]model.Order, error) {
	filterString := ""
	if filter.Maker != "" {
		filterString = appendFilterString(filterString, "maker", filter.Maker)
	}

	if filter.Taker != "" {
		filterString = appendFilterString(filterString, "taker", filter.Taker)
	}

	if filter.OrderPair != "" {
		filterString = appendFilterString(filterString, "order_pair", filter.OrderPair)
	}

	if filter.SecretHash != "" {
		filterString = appendFilterString(filterString, "secret_hash", filter.SecretHash)
	}

	if filter.OrderBy != "" {
		filterString = appendFilterString(filterString, "sort", filter.OrderBy)
	}

	if filter.Verbose {
		filterString = appendFilterString(filterString, "verbose", "true")
	}

	if filter.Status != 0 {
		filterString = appendFilterString(filterString, "status", strconv.Itoa(filter.Status))
	}

	if filter.MinPrice != 0 {
		filterString = appendFilterString(filterString, "min_price", strconv.FormatFloat(filter.MinPrice, 'f', -1, 64))
	}

	if filter.MaxPrice != 0 {
		filterString = appendFilterString(filterString, "max_price", strconv.FormatFloat(filter.MaxPrice, 'f', -1, 64))
	}

	if filter.Page != 0 {
		filterString = appendFilterString(filterString, "page", strconv.Itoa(filter.Page))
	}

	if filter.PerPage != 0 {
		filterString = appendFilterString(filterString, "per_page", strconv.Itoa(filter.PerPage))
	}

	if filter.MinAmount != 0 {
		filterString = appendFilterString(filterString, "min_amount", strconv.FormatFloat(filter.MinAmount, 'f', -1, 64))
	}
	if filter.MaxAmount != 0 {
		filterString = appendFilterString(filterString, "max_amount", strconv.FormatFloat(filter.MaxAmount, 'f', -1, 64))
	}

	resp, err := http.Get(fmt.Sprintf("%s/orders%s", c.url, filterString))
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %v", err)
		}
		return nil, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var orders []model.Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %v", err)
	}
	return orders, nil
}

func (c *client) GetFollowerInitiateOrders() ([]model.Order, error) {
	resp, err := http.Get(fmt.Sprintf("%s/orders?taker=%s&status=3&verbose=true", c.url, c.id))
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %v", err)
		}
		return nil, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var orders []model.Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %v", err)
	}
	return orders, nil
}

func (c *client) GetFollowerRedeemOrders() ([]model.Order, error) {
	resp, err := http.Get(fmt.Sprintf("%s/orders?taker=%s&status=5&verbose=true", c.url, c.id))
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %v", err)
		}
		return nil, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var orders []model.Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %v", err)
	}
	return orders, nil
}

func (c *client) GetInitiatorInitiateOrders() ([]model.Order, error) {
	resp, err := http.Get(fmt.Sprintf("%s/orders?maker=%s&status=2&verbose=true", c.url, c.id))
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %v", err)
		}
		return nil, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var orders []model.Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %v", err)
	}
	return orders, nil
}
func (c *client) GetFollowerRefundedOrders() ([]model.Order, error) {
	resp, err := http.Get(fmt.Sprintf("%s/orders?maker=%s&status=8&verbose=true", c.url, c.id))
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %v", err)
		}
		return nil, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var orders []model.Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %v", err)
	}
	return orders, nil
}
func (c *client) FollowerWaitForRedeemOrders() ([]model.Order, error) {
	resp, err := http.Get(fmt.Sprintf("%s/orders?taker=%s&status=4&verbose=true", c.url, c.id))
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %v", err)
		}
		return nil, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var orders []model.Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %v", err)
	}
	return orders, nil
}
func (c *client) InitiatorWaitForInitiateOrders() ([]model.Order, error) {
	resp, err := http.Get(fmt.Sprintf("%s/orders?maker=%s&status=3&verbose=true", c.url, c.id))
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %v", err)
		}
		return nil, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var orders []model.Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %v", err)
	}
	return orders, nil

}

func (c *client) GetInitiatorRedeemOrders() ([]model.Order, error) {
	resp, err := http.Get(fmt.Sprintf("%s/orders?maker=%s&status=4&verbose=true", c.url, c.id))
	if err != nil {
		return nil, fmt.Errorf("failed to get orders: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return nil, fmt.Errorf("failed to decode error response: %v", err)
		}
		return nil, fmt.Errorf("failed to get orders: %v", errorResponse.Error)
	}

	var orders []model.Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %v", err)
	}
	return orders, nil
}
func (c *client) GetLockedValue(user string, chain string) (int64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/getValueLocked?userWallet=%s&chainSelector=%s", c.url, user, chain))
	if err != nil {
		return 0, fmt.Errorf("failed to get valueLocked: %v", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Value int64 `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("failed to decode value: %v", err)
	}
	return payload.Value, nil
}
func (c *client) Health() (string, error) {
	resp, err := http.Get(fmt.Sprintf("%s/health", c.url))
	if err != nil {
		return "", fmt.Errorf("failed to get health: %v", err)
	}
	defer resp.Body.Close()

	var HealthPayload struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&HealthPayload); err != nil {
		return "", fmt.Errorf("failed to decode health: %v", err)
	}
	return HealthPayload.Status, nil
}

func (c *client) SetJwt(jwt string) error {
	c.JwtToken = jwt
	id, err := GetUserWalletFromJWT(jwt)
	if err != nil {
		return fmt.Errorf("failed to get user wallet from jwt: %v", err)
	}
	c.id = id
	return nil
}

func (c *client) GetNonce() (string, error) {
	resp, err := http.Get(fmt.Sprintf("%s/nonce", c.url))
	if err != nil {
		return "", fmt.Errorf("failed to get nonce: %v", err)
	}
	defer resp.Body.Close()

	var NoncePayload struct {
		Nonce string `json:"nonce"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&NoncePayload); err != nil {
		return "", fmt.Errorf("failed to decode nonce: %v", err)
	}
	return NoncePayload.Nonce, nil
}

func (c *client) Login() (string, error) {
	nonce, err := c.GetNonce()
	if err != nil {
		return "", fmt.Errorf("failed to get nonce: %v", err)
	}

	key, err := crypto.HexToECDSA(c.privKey)
	if err != nil {
		return "", fmt.Errorf("failed to get private key: %v", err)
	}

	address := crypto.PubkeyToAddress(key.PublicKey)
	message, err := CreateEip4361TestMessage(address.Hex(), nonce)
	if err != nil {
		return "", fmt.Errorf("failed to create message: %v", err)
	}

	ethPrivKey, err := crypto.HexToECDSA(c.privKey)
	if err != nil {
		return "", fmt.Errorf("failed to get private key: %v", err)
	}

	signature, err := signMessage(message.String(), ethPrivKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign message: %v", err)
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(model.VerifySiwe{
		Message:   message.String(),
		Signature: hexutil.Encode(signature),
	}); err != nil {
		return "", err
	}

	resp, err := http.Post(fmt.Sprintf("%s/verify", c.url), "application/json", &buf)
	if err != nil {
		return "", fmt.Errorf("failed to verify: %v", err)
	}
	defer resp.Body.Close()

	var VerifyPayload struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&VerifyPayload); err != nil {
		return "", fmt.Errorf("failed to decode verify: %v", err)
	}

	return VerifyPayload.Token, nil
}

func (c *client) ReLogin() error {
	token, err := c.Login()
	if err != nil {
		return fmt.Errorf("failed to login: %v", err)
	}
	c.SetJwt(token)
	return nil
}

func GetUserWalletFromJWT(jwtString string) (string, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(jwtString, &Claims{})
	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(*Claims); ok {
		return claims.UserWallet, nil
	}

	return "", fmt.Errorf("unable to extract UserWallet from JWT")
}

func signHash(data []byte) common.Hash {
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	return crypto.Keccak256Hash([]byte(msg))
}

func signMessage(message string, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	sign := signHash([]byte(message))
	signature, err := crypto.Sign(sign.Bytes(), privateKey)

	if err != nil {
		return nil, err
	}

	signature[64] += 27
	return signature, nil
}

func CreateEip4361TestMessage(
	publicAddress string,
	nonce string,
) (*siwe.Message, error) {
	options := make(map[string]interface{})
	options["chainId"] = 11155111 // for now sepolia later will use config to swtich based on chains
	options["statement"] = "Sign into Catalog and experience limitless cross chain"
	message, err := siwe.InitMessage(
		"localhost:8080",
		publicAddress,
		"https://localhost:3000",
		nonce,
		options,
	)

	if err != nil {
		return nil, err
	}

	return message, nil
}
