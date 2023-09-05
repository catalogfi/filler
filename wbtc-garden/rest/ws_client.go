package rest

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type wsClient struct {
	url    string
	logger *zap.Logger

	msgs chan string
}

type WSClient interface {
	Listen() <-chan interface{}
	Subscribe(msg string)
}

func (w *wsClient) Listen() <-chan interface{} {
	responses := make(chan interface{})
	go func() {
		defer close(responses)

		conn, _, err := websocket.DefaultDialer.Dial(w.url, nil)
		if err != nil {
			w.logger.Error("failed to connect to websocket", zap.Error(err))
			return
		}

		go func() {
			for msg := range w.msgs {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
					w.logger.Error("failed to connect to websocket", zap.Error(err))
					conn.Close()
					return
				}
			}
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				w.logger.Error("failed to read message from", zap.Error(err))
				return
			}

			var v map[string]interface{}
			if err := json.Unmarshal(msg, &v); err != nil {
				w.logger.Error("failed to decode", zap.Error(err))
				return
			}

			value, err := typeCast(v["type"].(string), v["msg"])
			if err != nil {
				w.logger.Error("failed to decode", zap.Error(err))
				return
			}
			if value == nil {
				continue
			}

			wsErr, ok := value.(WebsocketError)
			if ok {
				if wsErr.Code > 100 {
					responses <- wsErr
					w.logger.Error("websocket error", zap.String("msg", wsErr.Error))
					return
				}

				responses <- wsErr
				w.logger.Warn("websocket warning", zap.String("msg", wsErr.Error))
			} else {
				responses <- value
			}
		}
	}()

	return responses
}

func (ws *wsClient) Subscribe(msg string) {
	ws.msgs <- msg
}

func NewWSClient(url string, logger *zap.Logger) WSClient {
	return &wsClient{
		url:    url,
		logger: logger.With(zap.String("service", "ws_client")),
		msgs:   make(chan string, 8),
	}
}

func typeCast(t string, from interface{}) (interface{}, error) {
	data, err := json.Marshal(from)
	if err != nil {
		return nil, err
	}
	switch t {
	case "rest.OpenOrder":
		obj := OpenOrder{}
		return obj, json.Unmarshal(data, &obj)
	case "rest.UpdatedOrders":
		obj := UpdatedOrders{}
		return obj, json.Unmarshal(data, &obj)
	case "rest.UpdatedOrder":
		obj := UpdatedOrder{}
		return obj, json.Unmarshal(data, &obj)
	case "rest.WebsocketError":
		obj := WebsocketError{}
		return obj, json.Unmarshal(data, &obj)
	case "ping":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported object of type %s", t)
	}
}
