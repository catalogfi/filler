package jsonrpc

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/catalogfi/cobi/daemon/rpc/methods"
	"github.com/catalogfi/cobi/daemon/types"
	"github.com/catalogfi/cobi/store"
	"github.com/catalogfi/cobi/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type RPC interface {
	AddCommand(cmd methods.Method)
	HandleJSONRPC(ctx *gin.Context)
	Run()
	authenticateUser(ctx *gin.Context)
	UpdateCredentials(ctx *gin.Context)
}

type rpc struct {
	commands   map[string]methods.Method
	coreConfig types.CoreConfig
	authsha    [sha256.Size]byte
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
	if envConfig.RpcUserName == "" && envConfig.RpcPassword == "" {
		panic("RPC username and password must be specified")
	}

	login := envConfig.RpcUserName + ":" + envConfig.RpcPassword
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))

	return &rpc{
		commands: make(map[string]methods.Method),
		authsha:  sha256.Sum256([]byte(auth)),
		coreConfig: types.CoreConfig{
			Storage:   storage,
			EnvConfig: envConfig,
			Keys:      keys,
			Logger:    logger,
		},
	}
}

func (r *rpc) AddCommand(cmd methods.Method) {
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

func (r *rpc) authenticateUser(ctx *gin.Context) {
	authhdr := ctx.GetHeader("Authorization")
	if len(authhdr) <= 0 {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized Invalid credentials"})
		return
	}
	authsha := sha256.Sum256([]byte(authhdr))
	cmp := subtle.ConstantTimeCompare(authsha[:], r.authsha[:])
	if cmp != 1 {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "Unauthorized Invalid credentials"})
		return
	}

}

func (r *rpc) UpdateCredentials(ctx *gin.Context) {

	req := types.UpdateCredentials{}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Invalid credentials"})
		return
	}

	if req.RpcUserName == "" && req.RpcPassword == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Invalid credentials"})
		return
	}

	rpcLogin := req.RpcUserName + ":" + req.RpcPassword
	rpcAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(rpcLogin))

	r.authsha = sha256.Sum256([]byte(rpcAuth))

	if err := utils.UpdateAuth(req.RpcUserName, req.RpcPassword); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "Invalid credentials"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Credentials updated"})
}

func (r *rpc) Run() {
	r.AddCommand(methods.GetAccountInfo())
	r.AddCommand(methods.CreateNewOrder())
	r.AddCommand(methods.FillOrder())
	r.AddCommand(methods.DepositFunds())
	r.AddCommand(methods.TransferFunds())
	r.AddCommand(methods.ListOrders())
	r.AddCommand(methods.Status())
	r.AddCommand(methods.SetConfig())
	r.AddCommand(methods.GetConfig())
	r.AddCommand(methods.Retry())
	r.AddCommand(methods.GetNetworks())
	r.AddCommand(methods.GetStrategy())
	r.AddCommand(methods.SetNetwork())
	r.AddCommand(methods.SetStrategy())

	s := gin.Default()

	authRoutes := s.Group("/")
	authRoutes.Use(r.authenticateUser)

	authRoutes.POST("/", r.HandleJSONRPC)
	authRoutes.POST("/update", r.UpdateCredentials)
	s.Run(":3000")
}
