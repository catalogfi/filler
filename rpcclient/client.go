package rpcclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	jsonrpc "github.com/catalogfi/cobi/daemon/rpc"
	"github.com/catalogfi/cobi/daemon/rpc/handlers"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/utils"
)

type client struct {
	User      string
	Pass      string
	Protocol  string
	RPCServer string
}

type StartService struct {
	ServiceType     handlers.Service `json:"service" binding:"required"`
	Account         uint             `json:"userAccount"`
	IsInstantWallet bool             `json:"isInstantWallet"`
}

type Client interface {
	GetAccounts(data types.RequestAccount) (json.RawMessage, error)
	CreateOrder(data types.RequestCreate) (json.RawMessage, error)
	ListOrders(data types.RequestListOrders) (json.RawMessage, error)
	FillOrder(data types.RequestFill) (json.RawMessage, error)
	Transfer(data types.RequestTransfer) (json.RawMessage, error)
	Deposit(data types.RequestDeposit) (json.RawMessage, error)
	KillService(data handlers.KillSerivce) (json.RawMessage, error)
	StartService(data StartService) (json.RawMessage, error)
	SetConfig(data utils.Config) (json.RawMessage, error)
	RetryOrder(data types.RequestRetry) (json.RawMessage, error)
}

func NewClient(userName string, password string, protocol string, rpcServer string) Client {
	return &client{
		User:      userName,
		Pass:      password,
		Protocol:  protocol,
		RPCServer: rpcServer,
	}
}

// sendPostRequest sends the marshalled JSON-RPC command using HTTP-POST mode
// to the server described in the passed config struct.  It also attempts to
// unmarshal the response as a JSON-RPC response and returns either the result
// field or the error field depending on whether or not there is an error.
func (c *client) SendPostRequest(method string, jsonData []byte) (json.RawMessage, error) {
	payload := jsonrpc.Request{
		Version: "2.0",
		Method:  method,
		Params:  json.RawMessage(jsonData),
	}
	marshalledJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, (fmt.Errorf("failed to marshal payload: %w", err))
	}

	url := c.Protocol + "://" + c.RPCServer
	bodyReader := bytes.NewReader(marshalledJSON)
	httpRequest, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return nil, err
	}
	httpRequest.Close = true
	httpRequest.Header.Set("Content-Type", "application/json")

	// Configure basic access authorization.
	httpRequest.SetBasicAuth(c.User, c.Pass)

	// Create the new HTTP client that is configured according to the user-
	// specified options and submit the request.
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}

	// Read the raw bytes and close the response.
	respBytes, err := ioutil.ReadAll(httpResponse.Body)
	httpResponse.Body.Close()
	if err != nil {
		err = fmt.Errorf("error reading json reply: %v", err)
		return nil, err
	}

	// Handle unsuccessful HTTP responses
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		// Generate a standard error to return if the server body is
		// empty.  This should not happen very often, but it's better
		// than showing nothing in case the target server has a poor
		// implementation.
		if len(respBytes) == 0 {
			return nil, fmt.Errorf("%d %s", httpResponse.StatusCode,
				http.StatusText(httpResponse.StatusCode))
		}
		return nil, fmt.Errorf("%s", respBytes)
	}

	// Unmarshal the response.
	var resp jsonrpc.Response
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("error Occrured : %s with Data : %s", resp.Error.Message, resp.Error.Data)
	}
	return resp.Result, nil
}

func (c *client) GetAccounts(data types.RequestAccount) (json.RawMessage, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("getAccountInfo", jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	return resp, nil
}

func (c *client) CreateOrder(data types.RequestCreate) (json.RawMessage, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("createNewOrder", jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

func (c *client) ListOrders(data types.RequestListOrders) (json.RawMessage, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("listOrders", jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

func (c *client) FillOrder(data types.RequestFill) (json.RawMessage, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("fillOrder", jsonData)
	if err != nil {

		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

func (c *client) Transfer(data types.RequestTransfer) (json.RawMessage, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("transferFunds", jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

func (c *client) Deposit(data types.RequestDeposit) (json.RawMessage, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("depositFunds", jsonData)
	if err != nil {

		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

func (c *client) KillService(data handlers.KillSerivce) (json.RawMessage, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("killService", jsonData)
	if err != nil {

		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	return resp, nil
}

func (c *client) StartService(data StartService) (json.RawMessage, error) {
	if data.ServiceType == "" {
		return nil, fmt.Errorf("service type is required")
	}

	var procedure string
	if data.ServiceType == handlers.Executor {
		procedure = "startExecutor"
	} else {
		procedure = "startStrategy"
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest(procedure, jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil

}

func (c *client) SetConfig(data utils.Config) (json.RawMessage, error) {

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("setConfig", jsonData)

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

func (c *client) RetryOrder(data types.RequestRetry) (json.RawMessage, error) {

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := c.SendPostRequest("retryOrder", jsonData)

	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}
