package rpc

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/catalogfi/cobi/command"
	"github.com/catalogfi/cobi/handlers"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type RPC interface {
	AddCommand(cmd command.Command)
	HandleJSONRPC(ctx *gin.Context)
	Run()
}

type rpc struct {
	commands   map[string]command.Command
	coreConfig handlers.CoreConfig
}

// Request defines a JSON-RPC 2.0 request object.
type Request struct {
	Version string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response defines a JSON-RPC 2.0 response object.
type Response struct {
	Version string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error defines a JSON-RPC 2.0 error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// Error codes
const (
	ErrorCodeParseError        = -32700
	ErrorMessageParseError     = "Parse error"
	ErrorCodeInvalidRequest    = -32600
	ErrorMessageInvalidRequest = "Invalid Request"
	ErrorCodeMethodNotFound    = -32601
	ErrorMessageMethodNotFound = "Method not found"
	ErrorCodeInvalidParams     = -32602
	ErrorMessageInvalidParams  = "Invalid params"
	ErrorCodeInternalError     = -32603
	ErrorMessageInternalError  = "Internal error"
)

func NewResponse(id interface{}, result json.RawMessage, err *Error) Response {
	return Response{
		Version: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}
}

func NewError(code int, message string, data string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

func NewRpcServer(storage store.Store, envConfig utils.Config, keys *utils.Keys, logger *zap.Logger) RPC {
	return &rpc{
		commands: make(map[string]command.Command),
		coreConfig: handlers.CoreConfig{
			Storage:   storage,
			EnvConfig: envConfig,
			Keys:      keys,
			Logger:    logger,
		},
	}
}

func (r *rpc) AddCommand(cmd command.Command) {
	r.commands[cmd.Name()] = cmd
}

func (r *rpc) HandleJSONRPC(ctx *gin.Context) {
	req := Request{}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, NewResponse(req.ID, nil, NewError(ErrorCodeParseError, ErrorMessageParseError, err.Error())))
		return
	}

	cmd, ok := r.commands[req.Method]
	if !ok {
		ctx.JSON(http.StatusNotFound, NewResponse(req.ID, nil, NewError(ErrorCodeMethodNotFound, ErrorMessageMethodNotFound, "")))
		return
	}

	fmt.Println("params", string(req.Params))
	result, err := cmd.Query(r.coreConfig, req.Params)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, NewResponse(req.ID, nil, NewError(ErrorCodeInternalError, ErrorMessageInternalError, err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, NewResponse(req.ID, result, nil))

}

func (r *rpc) Run() {
	r.AddCommand(command.GetAccountInfo())
	r.AddCommand(command.CreateNewOrder())
	r.AddCommand(command.FillOrder())
	s := gin.Default()
	s.POST("/", r.HandleJSONRPC)
	s.Run(":8080")
}
